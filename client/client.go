// Package client provides a client for making HTTP requests.
package client

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httputil"
	"time"

	"github.com/gorilla/websocket"
	"github.com/taybart/rest/request"
)

type Client struct {
	client *http.Client
	ws     *websocket.Conn
	Config request.Config
}

func New(config request.Config) (*Client, error) {
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

func (c *Client) Do(r request.Request) (string, map[string]any, error) {

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
	if r.After != "" {
		exports, err := r.RunAfterHook(res, c.client.Jar)
		return "", exports, err
	}

	dumped, err := c.CheckExpectation(r, res)
	if err != nil {
		return "", nil, err
	}

	return dumped, nil, err
}
func (c *Client) CheckExpectation(r request.Request, res *http.Response) (string, error) {
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
