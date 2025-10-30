package request_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/taybart/rest"
	"github.com/taybart/rest/request"
)

func parse(t *testing.T, filename string, expectedReqs int) rest.Rest {
	rest, err := rest.NewRestFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if len(rest.HCLRequests) != expectedReqs {
		t.Fatalf("expected %d request(s), got %d", expectedReqs, len(rest.HCLRequests))
	}
	return rest
}
func build(t *testing.T, restfile rest.Rest, label string) *http.Request {
	toBuild, err := restfile.Request(label)
	if err != nil {
		t.Fatal("expected request to be found")
	}
	req, err := toBuild.Build()
	if err != nil {
		t.Fatal(err)
	}
	return req
}

func TestBasicRequest(t *testing.T) {
	rest := parse(t, "../doc/examples/client/basic.rest", 1)
	req := build(t, rest, "basic")
	if req.URL.String() != "http://localhost:18080/hello-world" {
		t.Fatal("expected url to be http://localhost:18080/hello-world got:", req.URL.String())
	}
	if req.Method != "POST" {
		t.Fatal("expected method to be POST got:", req.Method)
	}
	if req.Body == nil {
		t.Fatal("expected body to be non-nil")
	}
}
func DisabledTestClientRequest(t *testing.T) {
	rest := parse(t, "../doc/examples/client/basic.rest", 1)
	req := build(t, rest, "basic")
	if req.URL.String() != "http://localhost:18080/hello-world" {
		t.Fatal("expected url to be http://localhost:18080/hello-world got:", req.URL.String())
	}
	if req.Method != "POST" {
		t.Fatal("expected method to be POST got:", req.Method)
	}
	if req.Body == nil {
		t.Fatal("expected body to be non-nil")
	}

	serve := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "hello world")
	}))
	defer serve.Close()

	u, err := url.Parse(serve.URL)
	if err != nil {
		t.Fatal(err)
	}
	basic := rest.Requests["basic"]

	req.URL.Host = u.Host
	basic.Built = req

	client, err := request.NewClient(rest.Config)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := client.Do(basic); err != nil {
		t.Fatal(err)
	}

	resp := httptest.NewRecorder()
	if p, err := io.ReadAll(resp.Body); err != nil {
		t.Fail()
	} else {
		t.Fatal(string(p))
	}
}

func TestAuthdRequest(t *testing.T) {
	rest := parse(t, "../doc/examples/client/auth.rest", 3)
	req := build(t, rest, "basic auth")

	if req.Header.Get("Authorization") != "Basic dXNlcjpwYXNzd29yZA==" {
		t.Fatal("expected auth to be basic got:", req.Header.Get("Authorization"))
	}
	req = build(t, rest, "bearer token")
	if req.Header.Get("Authorization") != "Bearer ey..." {
		t.Fatal("expected auth to be token got:", req.Header.Get("Authorization"))
	}
}
