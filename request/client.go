package request

import (
	"bufio"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httputil"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/taybart/log"
)

var (
	client = &http.Client{}
)

type RequestClient struct {
	client *http.Client
	ws     *websocket.Conn
	Config Config
}

func NewRequestClient(config Config) (*RequestClient, error) {

	client := http.Client{}
	if !config.NoCookies {
		jar, err := cookiejar.New(nil)
		if err != nil {
			return nil, err
		}
		client.Jar = jar
	}
	return &RequestClient{
		client: &client,
		Config: config,
	}, nil
}

func (c *RequestClient) Do(r Request) (string, error) {

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
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
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
func (c *RequestClient) DoSocket(s Socket) error {

	s.UserAgent = c.Config.UserAgent

	// req, err := s.Build()
	// if err != nil {
	// 	return "", err
	// }

	if c.Config.NoFollowRedirect {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	dialer := &websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: 45 * time.Second,
	}

	// Set multiple custom headers
	headers := http.Header{
		"Origin":     []string{s.Origin},
		"User-Agent": []string{c.Config.UserAgent},
	}

	// Establish WebSocket connection
	conn, _, err := dialer.Dial(s.URL, headers)
	if err != nil {
		log.Fatal("Failed to connect:", err)
	}
	defer conn.Close()

	// Channel for incoming messages
	done := make(chan struct{})

	// Goroutine to read messages from server
	go func() {
		defer close(done)
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				log.Println("Read error:", err)
				return
			}
			fmt.Printf("%s\r< %s%s\n%s> %s",
				log.Green, log.Yellow, message, log.Green, log.Reset)
		}
	}()

	scanner := bufio.NewScanner(os.Stdin)
	fmt.Printf("%s> %s", log.Green, log.Reset)

	go func() {
		for scanner.Scan() {
			text := strings.TrimSpace(scanner.Text())

			if text == "quit" || text == "exit" {
				close(done)
				return
			}
			if text == "" {
				fmt.Printf("%s> %s", log.Green, log.Reset)
				continue
			}

			pb, ok := s.PlaybookParsed[text]
			if !ok {
				fmt.Printf("no such playbook entry: %s\n> ", text)
				continue
			}

			// Send message to server
			err := conn.WriteMessage(websocket.TextMessage, []byte(pb))
			if err != nil {
				log.Println("Write error:", err)
				return
			}

			fmt.Printf("%s> %s", log.Green, log.Reset)
		}
	}()

	// Wait for interrupt or done signal
	select {
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
