package client_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/taybart/rest/client"
	"github.com/taybart/rest/request"
)

// an api that counts its requests
func countingServer(t *testing.T, delay time.Duration) (*httptest.Server, *atomic.Int64) {
	var hits atomic.Int64
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		time.Sleep(delay)
		if r.URL.Path == "/fail" {
			w.WriteHeader(500)
			return
		}
		w.Header().Set("X-Hit", fmt.Sprint(n))
		fmt.Fprintf(w, `{"path": %q, "auth": %q}`, r.URL.RequestURI(), r.Header.Get("Authorization"))
	}))
	t.Cleanup(ts.Close)
	return ts, &hits
}

func cachedRequest(url string, ttl time.Duration) request.Request {
	return request.Request{Label: "r", Method: "GET", URL: url, Cache: ttl.String(), CacheTTL: ttl}
}

func TestCache(t *testing.T) {
	ts, hits := countingServer(t, 0)
	c, err := client.New(request.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	run := func(r request.Request) string {
		t.Helper()
		res, err := c.Run(r)
		if err != nil {
			t.Fatal(err)
		}
		return string(res.Body) + " hit " + res.Res.Header.Get("X-Hit")
	}

	first := run(cachedRequest(ts.URL+"/a", time.Minute))
	if again := run(cachedRequest(ts.URL+"/a", time.Minute)); again != first || hits.Load() != 1 {
		t.Errorf("not cached: %q, %q (%d requests)", first, again, hits.Load())
	}

	// anything that's sent is part of the key
	run(cachedRequest(ts.URL+"/a?page=2", time.Minute))
	authed := cachedRequest(ts.URL+"/a", time.Minute)
	authed.BearerToken = "tok"
	run(authed)
	if hits.Load() != 3 {
		t.Errorf("query / headers not in the key: %d requests", hits.Load())
	}

	// w/o cache, every request is sent
	run(request.Request{Label: "r", Method: "GET", URL: ts.URL + "/a"})
	if hits.Load() != 4 {
		t.Errorf("uncached request: %d requests", hits.Load())
	}

	// errors aren't cached
	for range 2 {
		res, err := c.Run(cachedRequest(ts.URL+"/fail", time.Minute))
		if err != nil || res.Res.StatusCode != 500 {
			t.Fatalf("fail: %v %v", res, err)
		}
	}
	if hits.Load() != 6 {
		t.Errorf("a 500 was cached: %d requests", hits.Load())
	}

	// entries expire
	short := cachedRequest(ts.URL+"/short", 50*time.Millisecond)
	run(short)
	run(short)
	time.Sleep(80 * time.Millisecond)
	run(short)
	if hits.Load() != 8 {
		t.Errorf("expiry: %d requests, want 8", hits.Load())
	}
}

// after hooks (and expect) run on cached responses too.
func TestCacheAfterHook(t *testing.T) {
	ts, hits := countingServer(t, 0)
	c, err := client.New(request.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	r := cachedRequest(ts.URL+"/a", time.Minute)
	r.After = `return rest.res:json().path .. ' ' .. rest.res.status`
	for range 3 {
		res, err := c.Run(r)
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprint(res.Returns) != "[/a 200]" {
			t.Errorf("hook returned %v", res.Returns)
		}
	}
	if hits.Load() != 1 {
		t.Errorf("%d requests", hits.Load())
	}
}

// Concurrent requests for the same missing entry share one request.
func TestCacheConcurrentMisses(t *testing.T) {
	ts, hits := countingServer(t, 100*time.Millisecond)
	c, err := client.New(request.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			res, err := c.Run(cachedRequest(ts.URL+"/a", time.Minute))
			if err != nil || res.Res.StatusCode != 200 || string(res.Body) != `{"path": "/a", "auth": ""}` {
				t.Errorf("%v %v", res, err)
			}
		})
	}
	wg.Wait()
	if hits.Load() != 1 {
		t.Errorf("%d requests, want 1", hits.Load())
	}
}

func TestCacheEviction(t *testing.T) {
	ts, hits := countingServer(t, 0)
	c, err := client.New(request.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	c.Cache = client.NewCache(2)
	for _, path := range []string{"/a", "/b", "/c", "/c", "/a"} {
		ttl := time.Minute
		if path == "/a" {
			ttl = time.Second // expires first, so it's evicted first
		}
		if _, err := c.Run(cachedRequest(ts.URL+path, ttl)); err != nil {
			t.Fatal(err)
		}
	}
	// /a was evicted to make room for /c, so it's requested again
	if hits.Load() != 4 || c.Cache.Len() != 2 {
		t.Errorf("%d requests, %d cached", hits.Load(), c.Cache.Len())
	}
}
