package rest_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/taybart/rest"
	"github.com/taybart/rest/server"
)

// an api for the server's request blocks to call
var cachedHits atomic.Int64

func upstream() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /cached/{id}", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"id": %q, "hit": %d}`, r.PathValue("id"), cachedHits.Add(1))
	})
	mux.HandleFunc("GET /item/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id": %q, "title": "item %s"}`, r.PathValue("id"), r.PathValue("id"))
	})
	mux.HandleFunc("POST /login", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"token": "tok"}`)
	})
	mux.HandleFunc("GET /fail", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	})
	return httptest.NewServer(mux)
}

const requestFile = `
locals { api = "%s" }

request "item" {
  skip   = true
  url    = "${locals.api}/item/${exports::try("id", 1)}"
  expect = 200
}

request "login" {
  url    = "${locals.api}/login"
  method = "POST"
  after  = <<LUA
    rest.exports.token = json.decode(rest.res.body).token
  LUA
}

# post processed: returns the title (and the id)
request "title" {
  skip  = true
  url   = "${locals.api}/item/${exports::try("id", 1)}"
  after = [lib.expected, <<-ERDE
    local item = rest.res:json()
    return item.title, { id = item.id, tags = { 'a', 'b' } }
  ERDE
  ]
  expect = 200
}

request "cached" {
  skip  = true
  url   = "${locals.api}/cached/${exports::try("id", 1)}"
  cache = "5m"
}

request "fail" {
  url    = "${locals.api}/fail"
  expect = 200
}

server {
  address = "localhost:0"
  setup   = <<LUA
    assert(rest.request('login'))
  LUA

  handler "GET" "/item/{id}" {
    fn = <<-ERDE
      local res = rest.request('item', { id = rest.req:path_value('id') })
      return { status = 200, body = res:json().title }
    ERDE
  }
  handler "GET" "/title/{id}" {
    fn = <<-ERDE
      local title, extra = rest.request('title', { id = rest.req:path_value('id') })
      return { status = 200, body = "{title} {extra.id} {table.concat(extra.tags, ',')}" }
    ERDE
  }
  handler "GET" "/first" {
    fn = <<LUA
      return { status = 200, body = rest.request(0):json().title }
    LUA
  }
  handler "GET" "/token" {
    fn = <<LUA
      local token = rest.exports.token
      rest.exports.token = 'changed' -- only for this request
      return { status = 200, body = token }
    LUA
  }
  handler "GET" "/cached/{id}" {
    fn = <<LUA
      local id = rest.req:path_value('id')
      local a = rest.request('cached', { id = id }):json()
      local b = rest.request('cached', { id = id }):json()
      return { status = 200, body = a.id .. ' ' .. a.hit .. ' ' .. b.hit }
    LUA
  }
  handler "GET" "/shared" {
    fn = <<LUA
      local a = rest.request('cached', { id = 's' }):json()
      local b = rest.request('cached', { id = 's' }):json()
      local ok, err = pcall(function() a.id = 'x' end)
      local fresh = rest.request('item', { id = 1 }):json() -- not cached: writable
      fresh.title = 'changed'
      return { status = 200, body = tostring(rawequal(a, b)) .. ' ' .. tostring(ok) .. ' ' .. tostring(err:find('read-only', 1, true) ~= nil) .. ' ' .. fresh.title }
    LUA
  }
  handler "GET" "/login" {
    fn = <<LUA
      local res = rest.request('login')
      return { status = 200, body = res.status .. ' ' .. rest.exports.token }
    LUA
  }
  handler "GET" "/fail" {
    fn = <<LUA
      local res, err = rest.request('fail')
      assert(res == nil)
      return { status = 502, body = err }
    LUA
  }
  handler "GET" "/count" {
    stateful = true
    fn = <<LUA
      rest.exports.n = (rest.exports.n or 0) + 1
      return { status = 200, headers = { ["X-Rest-State"] = rest.state }, body = tostring(rest.exports.n) }
    LUA
  }
}
`

func requestServer(t *testing.T) *httptest.Server {
	t.Helper()
	api := upstream()
	t.Cleanup(api.Close)
	filename := filepath.Join(t.TempDir(), "server.rest")
	if err := os.WriteFile(filename, []byte(fmt.Sprintf(requestFile, api.URL)), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := rest.NewFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	s, err := r.Server()
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.Router)
	t.Cleanup(ts.Close)
	return ts
}

func get(t *testing.T, url string, header ...string) (int, string, http.Header) {
	t.Helper()
	req, _ := http.NewRequest("GET", url, nil)
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(body), res.Header
}

func TestServerRequests(t *testing.T) {
	ts := requestServer(t)

	cases := []struct{ path, want string }{
		{"/item/7", "item 7"},
		{"/first", "item 1"}, // by index, w/ exports::try's default
		{"/token", "tok"},    // setup's exports
		{"/token", "tok"},    // not changed by the last request
		{"/item/8", "item 8"},
		{"/title/9", "item 9 9 a,b"}, // the after hook's returns
		{"/login", "200 tok"},        // the hook doesn't return: the response
	}
	for _, c := range cases {
		if status, body, _ := get(t, ts.URL+c.path); status != 200 || body != c.want {
			t.Errorf("%s: got %d %q, want %q", c.path, status, body, c.want)
		}
	}

	// cached responses are shared by requests (and keyed by url)
	cachedHits.Store(0)
	for _, c := range []struct{ path, want string }{
		{"/cached/1", "1 1 1"},
		{"/cached/1", "1 1 1"},
		{"/cached/2", "2 2 2"},
	} {
		if _, body, _ := get(t, ts.URL+c.path); body != c.want {
			t.Errorf("%s: got %q, want %q", c.path, body, c.want)
		}
	}

	// cached json is decoded once, read-only, and shared
	for range 2 {
		if _, body, _ := get(t, ts.URL+"/shared"); body != "true false true changed" {
			t.Errorf("shared: %q", body)
		}
	}

	status, body, _ := get(t, ts.URL+"/fail")
	if status != 502 || !strings.Contains(body, "rest.request(fail)") {
		t.Errorf("fail: got %d %q", status, body)
	}

	// a stateful handler keeps its exports
	_, n, h := get(t, ts.URL+"/count")
	_, n2, _ := get(t, ts.URL+"/count", server.StateHeader, h.Get(server.StateHeader))
	if n != "1" || n2 != "2" {
		t.Errorf("stateful exports: got %s, %s", n, n2)
	}
}

// Concurrent requests each get their own exports.
func TestServerRequestsConcurrent(t *testing.T) {
	ts := requestServer(t)
	var wg sync.WaitGroup
	for i := range 40 {
		wg.Go(func() {
			want := fmt.Sprintf("item %d", i)
			if _, body, _ := get(t, fmt.Sprintf("%s/item/%d", ts.URL, i)); body != want {
				t.Errorf("got %q, want %q", body, want)
			}
		})
	}
	wg.Wait()
}

func TestRequestCacheInvalid(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "bad.rest")
	if err := os.WriteFile(filename, []byte(`request "a" {
  url   = "http://localhost/"
  cache = "soon"
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := rest.NewFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Request("a"); err == nil || !strings.Contains(err.Error(), "cache must be a duration") {
		t.Errorf("got %v", err)
	}
}
