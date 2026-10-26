package client_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/taybart/rest"
	"github.com/taybart/rest/client"
)

func parse(t *testing.T, filename string, expectedReqs int) *rest.Rest {
	rest, err := rest.NewFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if len(rest.Requests) != expectedReqs {
		t.Fatalf("expected %d request(s), got %d", expectedReqs, len(rest.Requests))
	}
	return rest
}
func build(t *testing.T, restfile *rest.Rest, label string) *http.Request {
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
	if req.URL.String() != "http://localhost:18080/__echo__" {
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
	basic, err := rest.Request("basic")
	if err != nil {
		t.Fatal(err)
	}

	req.URL.Host = u.Host
	// basic.Built = req

	client, err := client.New(rest.Parser.Config)
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

func TestLibHookExecution(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":42,"name":"alice","user":{"token":"secret-abc"}}`)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "hooks.rest")
	content := fmt.Sprintf(`request "test_hooks" {
  url    = %q
  expect = 200
  after  = [
    lib.expected,
    inspect_res("id", "name"),
    export_res({ token = "user.token", user_id = "id" }),
  ]
}
`, srv.URL)
	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	restFile, err := rest.NewFile(filename)
	if err != nil {
		t.Fatal(err)
	}

	req, err := restFile.Request("test_hooks")
	if err != nil {
		t.Fatal(err)
	}

	c, err := client.New(restFile.Parser.Config)
	if err != nil {
		t.Fatal(err)
	}

	_, exports, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	if exports["token"] != "secret-abc" {
		t.Fatalf("expected exported token 'secret-abc', got %v", exports["token"])
	}
	if exports["user_id"] != float64(42) {
		t.Fatalf("expected exported user_id 42, got %v", exports["user_id"])
	}
}

func TestExpectShapeMatchers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-Id", "550e8400-e29b-41d4-a716-446655440000")
		fmt.Fprint(w, `{"token":"550e8400-e29b-41d4-a716-446655440000","id":4,"name":"Frankie Muniz","code":"USR-42"}`)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "expect_is.rest")
	content := fmt.Sprintf(`request "no_lua" {
  url = %q
  expect {
    status = 200
    headers = {
      "X-Request-Id" = is.uuid
    }
    body = {
      id    = 4
      name  = "Frankie Muniz"
      token = is.uuidv4
      code  = is("regex:^USR-[0-9]+$")
    }
  }
}

request "with_lua" {
  url = %q
  expect {
    status = 200
    body = {
      id    = is.int
      name  = is.string
      token = is.uuidv4
      code  = is("regex", "^USR-[0-9]+$")
    }
  }
  after = lib.expected
}

request "missing_field_fails" {
  url = %q
  expect {
    status = 200
    body = {
      id   = 4
      name = "Frankie Muniz"
    }
  }
}
`, srv.URL, srv.URL, srv.URL)
	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	restFile, err := rest.NewFile(filename)
	if err != nil {
		t.Fatal(err)
	}

	c, err := client.New(restFile.Parser.Config)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Go-only expectation check (no after hook)
	reqNoLua, err := restFile.Request("no_lua")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.Do(reqNoLua); err != nil {
		t.Fatalf("expected no_lua to pass, got: %v", err)
	}

	// 2. Lua lib.expected check
	reqWithLua, err := restFile.Request("with_lua")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.Do(reqWithLua); err != nil {
		t.Fatalf("expected with_lua to pass, got: %v", err)
	}

	// 3. Extra field in response fails exact shape match
	reqFail, err := restFile.Request("missing_field_fails")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.Do(reqFail); err == nil {
		t.Fatal("expected missing_field_fails to fail due to extra fields in response")
	}
}
