// Package request provides a client for making HTTP requests.
package request

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httputil"
	"os"
	"os/signal"
	"time"

	"github.com/gorilla/websocket"
	"github.com/taybart/log"
)

type Client struct {
	client *http.Client
	ws     *websocket.Conn
	Config Config
	// Requests map[string]Request
}

func NewClient(config Config) (*Client, error) {

	client := http.Client{}
	if !config.NoCookies {
		jar, err := cookiejar.New(nil)
		if err != nil {
			return nil, err
		}
		client.Jar = jar
	}
	return &Client{
		client: &client,
		Config: config,
	}, nil
}

func (c *Client) Do(r Request) (string, error) {

	if r.Delay != "" {
		delay, err := time.ParseDuration(r.Delay)
		if err != nil {
			return "", err
		}
		time.Sleep(delay)
	}
	r.UserAgent = c.Config.UserAgent

	req, err := r.Build()
	if err != nil {
		return "", err
	}

	if c.Config.NoFollowRedirect {
		c.client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}
	if c.Config.InsecureNoVerifyTLS {
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		c.client.Transport = tr
	}

	res, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	// run lua code if it exists
	if r.PostHook != "" {
		return r.RunPostHook(res, c.client.Jar)
	}

	dumped, err := httputil.DumpResponse(res, true)
	if err != nil {
		return "", err
	}
	if r.Expect != 0 {
		if res.StatusCode != r.Expect {
			return string(dumped), fmt.Errorf("unexpected response code %d != %d", r.Expect, res.StatusCode)
		}
	}
	return string(dumped), nil
}
func (c *Client) DoSocket(socketArg string, s Socket) error {

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

	conn, _, err := dialer.Dial(s.u.String(), headers)
	if err != nil {
		log.Fatal("Failed to connect:", err)
	}
	defer conn.Close()

	// signals
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	quit := make(chan struct{}) // internal quit
	done := make(chan struct{}) // cleanup channel

	// recieve goroutine
	go func() {
		defer close(done)
		for {
			select {
			case <-quit:
				return
			default:
				_, message, err := conn.ReadMessage()
				if err != nil {
					log.Println("Read error:", err)
					return
				}
				fmt.Printf("%s\r< %s\r\n%s> %s",
					log.Yellow, message, log.Green, log.Reset)
			}
		}
	}()

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
	case SocketRunPlaybook:
		fmt.Printf("Runing playbook order: %+v\n", s.Run.Order)

		go func() {
			defer close(quit)
			for _, next := range s.Run.Order {
				if next == "noop" {
					continue
				}
				payload := []byte(s.Playbook[next])
				err := conn.WriteMessage(websocket.TextMessage, payload)
				if err != nil {
					log.Error("Write:", err)
					return
				}
				time.Sleep(delay)
			}
		}()

	case SocketRunEntry:
		if pb, ok := s.Playbook[socketArg]; ok {
			err := conn.WriteMessage(websocket.TextMessage, []byte(pb))
			if err != nil {
				return err
			}
			return nil
		}
		return fmt.Errorf("no such playbook entry: %s", socketArg)

	case SocketREPL:
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
				err := conn.WriteMessage(websocket.TextMessage, []byte(pb))
				if err != nil {
					return fmt.Errorf("ws write: %w", err)
				}
				fmt.Printf("\n\r%ssent(%s)%s", log.BoldGreen, cmd, log.Reset)
			}
			return nil
		}, done)
	}

	// Wait for interrupt, quit or done signal
	select {
	case <-quit:
	case <-done:
	case <-interrupt:
		err := conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		if err != nil {
			log.Println("Write close error:", err)
		}
		return err
	}
	return nil
}
