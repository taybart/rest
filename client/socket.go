package client

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/taybart/log"
	"github.com/taybart/rest/request"
)

func (c *Client) DoSocket(socketArg string, s request.Socket) error {

	dialer, action, err := s.Build(socketArg, c.Config)
	if err != nil {
		return err
	}
	headers := http.Header{
		"User-Agent": []string{c.Config.UserAgent},
	}
	if s.Origin != "" {
		headers.Set("Origin", s.Origin)
	}

	c.ws, _, err = dialer.Dial(s.U.String(), headers)
	if err != nil {
		log.Fatal("Failed to connect:", err)
	}
	defer c.ws.Close()

	// signals
	// interrupt := make(chan os.Signal, 1)
	// signal.Notify(interrupt, os.Interrupt)
	done := make(chan struct{}) // cleanup channel

	// recieve goroutine
	go c.Listen(s, done)

	// populate delay if available
	var delay time.Duration
	if s.Run.Delay != "" {
		var err error
		delay, err = time.ParseDuration(s.Run.Delay)
		if err != nil {
			log.Error(err)
			return nil
		}
	}
	switch action {
	case request.SocketRunPlaybook:
		fmt.Printf("Runing playbook order: %+v\n", s.Run.Order)
		go c.RunSocketPlaybook(s, delay, done)
	case request.SocketRunEntry:
		return c.RunSocketEntry(s, socketArg)
	case request.SocketREPL:
		c.DoREPL(s, done)
	}

	// Wait done signal
	<-done
	err = c.ws.WriteMessage(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
	)
	if err != nil {
		log.Println("Write close error:", err)
		return err
	}
	return nil
}

func (c *Client) RunSocketPlaybook(s request.Socket, delay time.Duration, done chan struct{}) error {
	defer close(done)
	for _, next := range s.Run.Order {
		if next == "noop" {
			continue
		}
		payload := []byte(s.Playbook[next])
		err := c.ws.WriteMessage(websocket.TextMessage, payload)
		if err != nil {
			log.Error("Write:", err)
			return err
		}
		time.Sleep(delay)
	}
	return nil
}

func (c *Client) RunSocketEntry(s request.Socket, entry string) error {
	if pb, ok := s.Playbook[entry]; ok {
		err := c.ws.WriteMessage(websocket.TextMessage, []byte(pb))
		if err != nil {
			return err
		}
		return nil
	}
	return fmt.Errorf("no such playbook entry: %s", entry)
}

func (c *Client) DoREPL(s request.Socket, done chan struct{}) error {
	r := NewREPL(s.NoSpecialCmds)
	go r.Loop(func(cmd string) error {
		switch cmd {
		case "ls":
			if !r.NoSpecialCmds {
				fmt.Print("\n\r")
				for k := range s.Playbook {
					fmt.Printf("%s ", k)
				}
				return nil
			}
			fallthrough
		default:
			pb, ok := s.Playbook[cmd]
			if !ok {
				return fmt.Errorf("no such playbook entry: %s", cmd)
			}
			err := c.ws.WriteMessage(websocket.TextMessage, []byte(pb))
			if err != nil {
				return fmt.Errorf("ws write: %w", err)
			}
			fmt.Printf("\n\r%ssent(%s)%s", log.BoldGreen, cmd, log.Reset)
		}
		return nil
	}, done)
	return nil
}

func (c *Client) Listen(s request.Socket, done chan struct{}) error {
	for {
		select {
		case <-done:
			return nil
		default:
			_, message, err := c.ws.ReadMessage()
			if err != nil {
				log.Println("Read error:", err)
				return err
			}
			fmt.Printf("%s\r< %s\r\n%s> %s",
				log.Yellow, message, log.Green, log.Reset)
		}
	}
}
