package client

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	lua "github.com/taybart/go-erde"
	restlua "github.com/taybart/rest/lua"
)

// DefaultCacheEntries is how many responses a Cache keeps (the ones closest
// to expiring are dropped first).
const DefaultCacheEntries = 1000

// Cache keeps the responses of requests w/ `cache = "<ttl>"` (when run w/
// Client.Run, ex. by rest.request). Entries are keyed by everything that's
// sent (method, url, headers, cookies, and body), only successful (2xx)
// responses are kept, and concurrent requests for the same missing entry
// share one request. It's safe for concurrent use.
type Cache struct {
	mu       sync.Mutex
	max      int
	entries  map[string]*cacheEntry
	inflight map[string]*flight
}

type cacheEntry struct {
	status  int
	proto   string
	header  http.Header
	body    []byte
	expires time.Time

	// the body decoded from json, once, as read-only tables shared by every
	// state that uses the response (see sharedJSON)
	jsonOnce sync.Once
	json     lua.LValue
	jsonErr  error
}

// sharedJSON decodes the body (once): tables are frozen, so states in other
// goroutines can share them (and pages don't each keep their own copy).
func (e *cacheEntry) sharedJSON() (lua.LValue, error) {
	e.jsonOnce.Do(func() {
		var v any
		if e.jsonErr = json.Unmarshal(e.body, &v); e.jsonErr != nil {
			return
		}
		l := lua.NewState(lua.Options{SkipOpenLibs: true})
		e.json = restlua.AnyToLValue(l, v)
		l.Close() // (the tables don't belong to the state)
		if t, ok := e.json.(*lua.LTable); ok {
			t.FreezeAll()
		}
	})
	return e.json, e.jsonErr
}

type flight struct {
	done  chan struct{}
	entry *cacheEntry // nil if the request failed or wasn't cacheable
	res   *http.Response
	body  []byte
	err   error
}

// NewCache makes a cache that keeps up to max responses (DefaultCacheEntries
// if max <= 0).
func NewCache(max int) *Cache {
	if max <= 0 {
		max = DefaultCacheEntries
	}
	return &Cache{max: max, entries: map[string]*cacheEntry{}, inflight: map[string]*flight{}}
}

// cacheKey is everything about a request that can change its response.
func cacheKey(req *http.Request, body string, jar http.CookieJar) string {
	var b strings.Builder
	b.WriteString(req.Method)
	b.WriteByte(' ')
	b.WriteString(req.URL.String())
	keys := make([]string, 0, len(req.Header))
	for k := range req.Header {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		for _, v := range req.Header[k] {
			b.WriteString("\n" + k + ": " + v)
		}
	}
	if jar != nil {
		for _, c := range jar.Cookies(req.URL) {
			b.WriteString("\ncookie " + c.Name + "=" + c.Value)
		}
	}
	b.WriteString("\n\n")
	b.WriteString(body)
	return b.String()
}

// response makes a response from an entry, for one request.
func (e *cacheEntry) response(req *http.Request) *http.Response {
	return &http.Response{
		Status:        http.StatusText(e.status),
		StatusCode:    e.status,
		Proto:         e.proto,
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        e.header.Clone(),
		Body:          io.NopCloser(bytes.NewReader(e.body)),
		ContentLength: int64(len(e.body)),
		Request:       req,
	}
}

// get returns the response for key, from the cache or from send (which is
// only called once for concurrent gets of the same key).
// The entry is nil if the response isn't cached.
func (c *Cache) get(key string, ttl time.Duration, req *http.Request, send func() (*http.Response, []byte, error)) (*http.Response, []byte, *cacheEntry, error) {
	c.mu.Lock()
	if e, ok := c.entries[key]; ok {
		if time.Now().Before(e.expires) {
			c.mu.Unlock()
			return e.response(req), e.body, e, nil
		}
		delete(c.entries, key)
	}
	if f, ok := c.inflight[key]; ok {
		c.mu.Unlock()
		<-f.done
		if f.entry != nil {
			return f.entry.response(req), f.entry.body, f.entry, nil
		}
		// not cacheable (ex. a 500): the other request's response, as is
		if f.err != nil {
			return nil, nil, nil, f.err
		}
		return (&cacheEntry{status: f.res.StatusCode, proto: f.res.Proto, header: f.res.Header, body: f.body}).response(req), f.body, nil, nil
	}
	f := &flight{done: make(chan struct{})}
	c.inflight[key] = f
	c.mu.Unlock()

	f.res, f.body, f.err = send()
	c.mu.Lock()
	delete(c.inflight, key)
	if f.err == nil && f.res.StatusCode >= 200 && f.res.StatusCode < 300 {
		f.entry = &cacheEntry{
			status:  f.res.StatusCode,
			proto:   f.res.Proto,
			header:  f.res.Header.Clone(),
			body:    f.body,
			expires: time.Now().Add(ttl),
		}
		c.store(key, f.entry)
	}
	c.mu.Unlock()
	close(f.done)
	return f.res, f.body, f.entry, f.err
}

// store adds an entry (the cache is locked), making room if needed: expired
// entries first, then the ones closest to expiring.
func (c *Cache) store(key string, e *cacheEntry) {
	if len(c.entries) >= c.max {
		now := time.Now()
		for k, old := range c.entries {
			if !now.Before(old.expires) {
				delete(c.entries, k)
			}
		}
		for len(c.entries) >= c.max {
			var soonest string
			for k, old := range c.entries {
				if soonest == "" || old.expires.Before(c.entries[soonest].expires) {
					soonest = k
				}
			}
			delete(c.entries, soonest)
		}
	}
	c.entries[key] = e
}

// Len is the number of cached responses (including expired ones not yet
// dropped).
func (c *Cache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}
