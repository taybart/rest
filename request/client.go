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
func (c *RequestClient) DoSocket(socketArg string, s *Socket) error {

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

	// TODO: probably only set if provided
	headers := http.Header{
		"Origin":     []string{s.Origin},
		"User-Agent": []string{c.Config.UserAgent},
	}

	conn, _, err := dialer.Dial(s.URL, headers)
	if err != nil {
		log.Fatal("Failed to connect:", err)
	}
	defer conn.Close()

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
				fmt.Printf("%s\r< %s%s\n%s> %s",
					log.Green, log.Yellow, message, log.Green, log.Reset)
			}
		}
	}()

	if s.Run != nil && socketArg == "run" {
		fmt.Printf("Runing playbook order: %+v\n", s.Run.Order)
		var delay time.Duration
		if s.Run.Delay != "" {
			var err error
			delay, err = time.ParseDuration(s.Run.Delay)
			if err != nil {
				log.Error(err)
				return nil
			}
		}

		go func() {
			defer close(quit)

			for _, next := range s.Run.Order {

				if next == "noop" {
					continue
				}
				// TODO: is this even feasible
				// requireResponse := false
				// if next[len(next)-1] == '!' {
				// 	next = next[:len(next)-1]
				// 	requireResponse = true
				// }
				// if requireResponse {
				// 	// fmt.Println("response required")
				// }
				payload := []byte(s.Playbook[next])
				err := conn.WriteMessage(websocket.TextMessage, payload)
				if err != nil {
					log.Error("Write:", err)
					return
				}
				time.Sleep(delay)
			}
		}()

	} else {
		// TODO: this is kind of ambiguous maybe it could be in a switch with run above
		if socketArg != "" {
			if pb, ok := s.Playbook[socketArg]; ok {
				return conn.WriteMessage(websocket.TextMessage, []byte(pb))
			}
			return fmt.Errorf("no such playbook entry: %s", socketArg)
		}

		// REPL
		scanner := bufio.NewScanner(os.Stdin)
		fmt.Printf("%s> %s", log.Green, log.Reset)

		go func() {
			for scanner.Scan() {
				text := strings.TrimSpace(scanner.Text())

				switch text {
				case "quit", "exit":
					close(done)
					return
				case "":
					fmt.Printf("%s> %s", log.Green, log.Reset)
					continue

				default:
					pb, ok := s.Playbook[text]
					if !ok {
						fmt.Printf("no such playbook entry: %s\n> ", text)
						continue
					}
					err := conn.WriteMessage(websocket.TextMessage, []byte(pb))
					if err != nil {
						log.Println("Write error:", err)
						return
					}
				}

				fmt.Printf("%s> %s", log.Green, log.Reset)
			}
		}()
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
