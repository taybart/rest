package server_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/taybart/rest/server"
)

type Response struct {
	StatusCode int    `json:"status"`
	Body       string `json:"body"`
}

func checkResponse(t *testing.T, req *http.Request, expected Response) {
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Errorf("error reading response: %s", err)
	}
	if res.StatusCode != expected.StatusCode {
		t.Errorf("expected status code %d, got %d", expected.StatusCode, res.StatusCode)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Errorf("error reading response body: %s", err)
	}
	if string(body) != expected.Body {
		t.Errorf(`expected body %s, got %s`, expected.Body, body)
	}
}

func newServer(config server.Config) *httptest.Server {
	s := server.Server{
		Router: http.NewServeMux(),
		Config: config,
	}
	s.Routes(&http.Server{})
	ts := httptest.NewServer(s.Router)
	return ts
}

func TestRoot(t *testing.T) {
	ts := newServer(server.Config{
		Quiet: true,
	})
	defer ts.Close()
	req, err := http.NewRequest("GET", ts.URL+"/test/sinkhold/route", nil)
	if err != nil {
		t.Errorf("error creating request: %s", err)
	}
	checkResponse(t, req, Response{StatusCode: http.StatusOK, Body: `{"status": "ok"}`})
}

func TestResponse(t *testing.T) {
	f, err := os.ReadFile("../test/response.json")
	if err != nil {
		t.Errorf("error reading response.json: %s", err)
	}

	var res server.Response
	err = json.Unmarshal(f, &res)
	if err != nil {
		t.Errorf("error marshalling response.json: %s", err)
	}

	ts := newServer(server.Config{
		Quiet:    true,
		Response: &res,
	})
	defer ts.Close()
	req, err := http.NewRequest("GET", ts.URL+"/response/test", nil)
	if err != nil {
		t.Errorf("error creating request: %s", err)
	}
	checkResponse(t, req, Response{StatusCode: res.Status, Body: string(res.Body)})
}

func TestDir(t *testing.T) {
	f, err := os.ReadFile("../test/response.json")
	if err != nil {
		t.Errorf("error reading response.json: %s", err)
	}

	ts := newServer(server.Config{
		Quiet: true,
		Dir:   "../test",
	})
	defer ts.Close()
	req, err := http.NewRequest("GET", ts.URL+"/response.json", nil)
	if err != nil {
		t.Errorf("error creating request: %s", err)
	}
	checkResponse(t, req, Response{StatusCode: http.StatusOK, Body: string(f)})
}

func TestEcho(t *testing.T) {
	ts := newServer(server.Config{
		Quiet: true,
	})
	defer ts.Close()
	req, err := http.NewRequest("POST", ts.URL+"/__echo__", strings.NewReader(`{"data": "hello"}`))
	if err != nil {
		t.Errorf("error creating request: %s", err)
	}
	checkResponse(t, req, Response{StatusCode: http.StatusOK, Body: `{"data": "hello"}`})
}
