package request

import (
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httputil"
	"time"
)

var (
	client = &http.Client{}
)

type RequestClient struct {
	client *http.Client
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
