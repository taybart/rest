// Package client provides a client for making HTTP requests.
package client

import (
	"crypto/tls"
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
	var body string
	if r.Expect != nil && len(r.Expect.Body) != 0 {
		b, err := io.ReadAll(res.Body)
		if err != nil {
			return "", err
		}
		defer res.Body.Close()
		body = string(b)
	}
	if err := request.CheckExpectation(r.Label, r.Expect, r.ExpectStatus, res.StatusCode, res.Header, body); err != nil {
		return string(dumped), err
	}
	return string(dumped), nil
}
