// Package client provides a client for making HTTP requests.
package client

import (
	"bytes"
	"crypto/tls"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httputil"
	"time"

	"github.com/gorilla/websocket"
	restlua "github.com/taybart/rest/lua"
	"github.com/taybart/rest/request"
)

type Client struct {
	client *http.Client
	ws     *websocket.Conn
	Config request.Config
	// Out is where after hooks print, defaults to stdout
	Out io.Writer
	// Cache keeps responses of requests w/ `cache` (in Run), nil disables it
	Cache *Cache
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
	if config.NoFollowRedirect {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}
	if config.InsecureNoVerifyTLS {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}
	return &Client{
		client: &client,
		Config: config,
		Cache:  NewCache(DefaultCacheEntries),
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
	if c.Out != nil {
		r.Out = c.Out
	}

	req, err := r.Build()
	if err != nil {
		return "", nil, err
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

// Run sends a request like Do (w/ its expect and after hook), but instead of
// a dump, returns the response, and what the after hook exported and
// returned. It's safe to use concurrently.
func (c *Client) Run(r request.Request) (*restlua.RequestResult, error) {
	r.UserAgent = c.Config.UserAgent
	if c.Out != nil {
		r.Out = c.Out
	}
	req, err := r.Build()
	if err != nil {
		return nil, err
	}
	send := func() (*http.Response, []byte, error) {
		if r.Delay != "" {
			delay, err := time.ParseDuration(r.Delay)
			if err != nil {
				return nil, nil, err
			}
			time.Sleep(delay)
		}
		res, err := c.client.Do(req)
		if err != nil {
			return nil, nil, err
		}
		body, err := io.ReadAll(res.Body)
		res.Body.Close()
		return res, body, err
	}
	var res *http.Response
	var body []byte
	var entry *cacheEntry
	if r.CacheTTL > 0 && c.Cache != nil {
		res, body, entry, err = c.Cache.get(cacheKey(req, r.Body, c.client.Jar), r.CacheTTL, req, send)
	} else {
		res, body, err = send()
	}
	if err != nil {
		return nil, err
	}
	reset := func() { res.Body = io.NopCloser(bytes.NewReader(body)) }

	reset()
	result := &restlua.RequestResult{Res: res, Body: body}
	if entry != nil {
		result.SharedJSON = entry.sharedJSON
	}
	if r.After != "" {
		result.Exports, result.Returns, err = r.RunAfterHookReturns(res, c.client.Jar)
	} else {
		_, err = c.CheckExpectation(r, res)
	}
	reset()
	if err != nil {
		return nil, err
	}
	return result, nil
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
