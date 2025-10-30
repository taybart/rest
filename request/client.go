// Package request provides a client for making HTTP requests.
package request

import (
	"crypto/tls"
	"fmt"
	"io"
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
	client  *http.Client
	ws      *websocket.Conn
	Config  Config
	exports map[string]string
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

func (c *Client) Do(r Request) (string, map[string]any, error) {

	if r.Delay != "" {
		delay, err := time.ParseDuration(r.Delay)
		if err != nil {
			return "", nil, err
		}
		time.Sleep(delay)
	}
	r.UserAgent = c.Config.UserAgent

	req, err := r.Build()
	if err != nil {
		return "", nil, err
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
		return "", nil, err
	}
	// run lua code if it exists
	if r.PostHook != "" {
		res, exports, err := r.RunPostHook(res, c.client.Jar)
		// if len(exports) > 0 {
		// c.exports = exports
		// }
		return res, exports, err
	}

	dumped, err := c.CheckExpectation(r, res)
	if err != nil {
		return "", nil, err
	}
	// TODO
	exports, err := c.GetExports(r, res)
	if err != nil {
		return "", nil, err
	}

	return dumped, exports, err
}
func (c *Client) CheckExpectation(r Request, res *http.Response) (string, error) {
	dumped, err := httputil.DumpResponse(res, true)
	if err != nil {
		return "", err
	}
	if r.Expect != nil {
		if r.Expect.Status != 0 {
			if res.StatusCode != r.Expect.Status {
				return string(dumped), fmt.Errorf(
					`request "%s": unexpected response code %d != %d`,
					r.Label, r.Expect.Status, res.StatusCode)
			}
		}
		if len(r.Expect.Body) != 0 {
			body, err := io.ReadAll(res.Body)
			if err != nil {
				return "", err
			}
			defer res.Body.Close()

			if r.Expect.Body != string(body) {
				return string(dumped), fmt.Errorf(
					`request "%s": unexpected response body %s != %s`,
					r.Label, r.Expect.Body, string(body))
			}
		}
		if len(r.Expect.Headers) != 0 {
			for k, v := range r.Expect.Headers {
				values := res.Header.Values(k)
				if len(values) == 0 {
					return string(dumped), fmt.Errorf(
						`request "%s": required response header "%s" not present`,
						r.Label, k)
				}
				matches := false
				lastValue := ""
				for _, value := range values {
					lastValue = value
					if value == v {
						matches = true
					}
				}
				if !matches {
					// small assumption that header is standalone for usablilty
					return string(dumped), fmt.Errorf(
						`request "%s": unexpected response header [%s] %s != %s`,
						r.Label, k, v, lastValue)
				}
			}
		}
	} else if r.ExpectStatus != 0 {
		if res.StatusCode != r.ExpectStatus {
			return string(dumped), fmt.Errorf(
				`request "%s": unexpected response code %d != %d`,
				r.Label, r.ExpectStatus, res.StatusCode)
		}
	}
	return string(dumped), nil
}
func (c *Client) GetExports(r Request, res *http.Response) (map[string]any, error) {
	return nil, nil
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
