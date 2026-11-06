package server

import (
	"fmt"
	"maps"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/taybart/log"
)

// Handlers w/ `cache = "<ttl>"` (fn or page handlers for GET) keep their
// responses for that long, keyed by path, query, and the `vary` headers. It's
// the handler's promise that its response only depends on those: rest can't
// tell (a script can read cookies, kv, the time, ...). So only plain 200s are
// kept: not ones that set cookies, and not interactive pages (whose html has
// the id of the visitor's own lua state).

// cacheEntries is how many responses a handler keeps (the oldest are dropped
// first).
const cacheEntries = 1000

type responseCache struct {
	mu      sync.Mutex
	ttl     time.Duration
	entries map[string]cachedResponse
	// warned about an interactive page (once)
	warned sync.Once
}

type cachedResponse struct {
	res     Response
	expires time.Time
}

func newResponseCache(ttl time.Duration) *responseCache {
	return &responseCache{ttl: ttl, entries: map[string]cachedResponse{}}
}

// responseCacheKey is the path, query (sorted), and the handler's vary
// headers.
func responseCacheKey(h *Handler, req *http.Request) string {
	var b strings.Builder
	b.WriteString(req.URL.Path)
	b.WriteByte('?')
	b.WriteString(req.URL.Query().Encode())
	for _, name := range h.Vary {
		b.WriteString("\n" + strings.ToLower(name) + ": " + strings.Join(req.Header.Values(name), ", "))
	}
	return b.String()
}

func (c *responseCache) get(key string) (Response, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok {
		return Response{}, false
	}
	if !time.Now().Before(e.expires) {
		delete(c.entries, key)
		return Response{}, false
	}
	res := e.res
	res.Headers = maps.Clone(e.res.Headers)
	return res, true
}

// store keeps a handler's response if it can be (and adds Cache-Control to
// it, so a CDN / proxy in front of rest can cache it too).
func (c *responseCache) store(h *Handler, key string, res *Response, interactive bool) {
	if interactive {
		c.warned.Do(func() {
			log.Warnf("%s %s: not cached, it's an interactive page (its html is for one visitor)\n", h.Method, h.Path)
		})
		return
	}
	if res.Status != http.StatusOK || len(res.Cookies) > 0 {
		return
	}
	if _, ok := headerKey(res.Headers, "Cache-Control"); !ok {
		if res.Headers == nil {
			res.Headers = map[string]string{}
		}
		res.Headers["Cache-Control"] = fmt.Sprintf("public, max-age=%d", int(c.ttl.Seconds()))
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= cacheEntries {
		now := time.Now()
		for k, e := range c.entries {
			if !now.Before(e.expires) {
				delete(c.entries, k)
			}
		}
		for len(c.entries) >= cacheEntries {
			var oldest string
			for k, e := range c.entries {
				if oldest == "" || e.expires.Before(c.entries[oldest].expires) {
					oldest = k
				}
			}
			delete(c.entries, oldest)
		}
	}
	stored := *res
	stored.Headers = maps.Clone(res.Headers)
	c.entries[key] = cachedResponse{res: stored, expires: time.Now().Add(c.ttl)}
}

func headerKey(headers map[string]string, key string) (string, bool) {
	for k := range headers {
		if strings.EqualFold(k, key) {
			return k, true
		}
	}
	return "", false
}
