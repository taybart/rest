package file_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/taybart/rest"
	"github.com/taybart/rest/server"
)

const expectServer = `
server {
  address = ":0"
  middleware "auth" {
    fn = <<LUA
      if rest.req:header('Authorization') ~= 'Bearer ok' then
        return { status = 401, body = '{"error":"unauthorized"}' }
      end
    LUA
  }
  handler "POST" "/login" {
    expect {
      headers = { "Content-Type" = is("regex:^application/json") }
      body = {
        username = is.not_empty
        password = is.string
        remember = is::optional(is.bool)
        scopes   = is::optional(is::array_of(is.string))
      }
    }
    fn = <<LUA
      local b = json.decode(rest.req.body)
      return { status = 200, body = json.encode({ user = b.username }) }
    LUA
  }
  handler "POST" "/events" {
    expect {
      additional = true
      body = { type = is.string }
    }
    response { status = 202 }
  }
  handler "GET" "/users/{id}" {
    middleware = ["auth"]
    expect {
      path  = { id = is.int }
      query = {
        fields = is::optional(is.string)
        tag    = is::optional(is::array_of(is("regex:^[a-z]+$")))
      }
    }
    response {
      status = 200
      body   = { ok = true }
    }
  }
}
`

func newExpectServer(t *testing.T, content string) *httptest.Server {
	t.Helper()
	filename := filepath.Join(t.TempDir(), "server.rest")
	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	r, err := rest.NewFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	config, err := r.Parser.Server()
	if err != nil {
		t.Fatal(err)
	}
	config.Quiet = true
	s := server.Server{Router: http.NewServeMux(), Config: config}
	s.Routes(&http.Server{})
	ts := httptest.NewServer(s.Router)
	t.Cleanup(ts.Close)
	return ts
}

func TestHandlerExpect(t *testing.T) {
	ts := newExpectServer(t, expectServer)

	cases := []struct {
		name    string
		method  string
		path    string
		headers map[string]string
		body    string
		status  int
		bodySub string
	}{
		{"login ok", "POST", "/login", map[string]string{"Content-Type": "application/json"},
			`{"username":"root","password":"toor","scopes":["a"]}`, 200, `"user":"root"`},
		{"login missing content type", "POST", "/login", nil,
			`{"username":"root","password":"toor"}`, 400, `headers.Content-Type: missing required value`},
		{"login missing field", "POST", "/login", map[string]string{"Content-Type": "application/json"},
			`{"username":"root"}`, 400, `body.password: missing required field`},
		{"login bad optional", "POST", "/login", map[string]string{"Content-Type": "application/json"},
			`{"username":"root","password":"x","remember":"yes"}`, 400, `body.remember: expected bool`},
		{"login not json", "POST", "/login", map[string]string{"Content-Type": "application/json"},
			`username=root`, 400, `body: expected JSON`},
		{"login empty body", "POST", "/login", map[string]string{"Content-Type": "application/json"},
			``, 400, `body: missing`},
		{"additional allows extra fields", "POST", "/events", nil,
			`{"type":"click","x":1,"meta":{"a":1}}`, 202, ``},
		{"additional still requires listed", "POST", "/events", nil,
			`{"x":1}`, 400, `body.type: missing required field`},
		{"user ok", "GET", "/users/4?fields=name&tag=a&tag=b", map[string]string{"Authorization": "Bearer ok"},
			``, 200, `"ok":true`},
		{"middleware runs before validation", "GET", "/users/abc", nil, ``, 401, `unauthorized`},
		{"user bad path", "GET", "/users/abc", map[string]string{"Authorization": "Bearer ok"},
			``, 400, `path.id: expected integer`},
		{"user bad tag", "GET", "/users/4?tag=a&tag=B", map[string]string{"Authorization": "Bearer ok"},
			``, 400, `query.tag[1]: value \"B\" does not match regex`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req, err := http.NewRequest(c.method, ts.URL+c.path, strings.NewReader(c.body))
			if err != nil {
				t.Fatal(err)
			}
			for k, v := range c.headers {
				req.Header.Set(k, v)
			}
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			b, _ := io.ReadAll(res.Body)
			if res.StatusCode != c.status {
				t.Fatalf("expected status %d, got %d (%s)", c.status, res.StatusCode, b)
			}
			if !strings.Contains(string(b), c.bodySub) {
				t.Fatalf("expected body containing %q, got %s", c.bodySub, b)
			}
			if c.status == 400 && res.Header.Get("Content-Type") != "application/json" {
				t.Fatalf("expected json content type on 400, got %q", res.Header.Get("Content-Type"))
			}
		})
	}
}

func TestHandlerExpectUnknownPathWildcard(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "server.rest")
	content := `
server {
  address = ":0"
  handler "GET" "/users/{id}" {
    expect { path = { user_id = is.int } }
    response { status = 200 }
  }
}
`
	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	r, err := rest.NewFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.Parser.Server()
	if err == nil || !strings.Contains(err.Error(), "no {user_id} wildcard") {
		t.Fatalf("expected wildcard error, got %v", err)
	}
}
