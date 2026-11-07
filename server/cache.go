package server

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
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

// A handler keeps at most cacheEntries responses, and cacheBytes of them
// (the oldest are dropped first). Bodies worth compressing are kept gzipped:
// served as is to clients that accept gzip (most), and decompressed for the
// rest.
const (
	cacheEntries = 1000
	cacheBytes   = 64 << 20
)

type responseCache struct {
	mu      sync.Mutex
	ttl     time.Duration
	entries map[string]cachedResponse
	bytes   int // of the entries' bodies
	// warned about an interactive page (once)
	warned sync.Once
}

type cachedResponse struct {
	res     Response
	gzipped bool // res.Body is gzipped
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

// get returns a cached response: gzipped (w/ Content-Encoding) if it's
// stored that way and the client accepts it.
func (c *responseCache) get(key string, acceptGzip bool) (Response, bool) {
	c.mu.Lock()
	e, ok := c.entries[key]
	if ok && !time.Now().Before(e.expires) {
		c.remove(key)
		ok = false
	}
	c.mu.Unlock()
	if !ok {
		return Response{}, false
	}
	res := e.res
	res.Headers = maps.Clone(e.res.Headers)
	if e.gzipped {
		if acceptGzip {
			res.Headers["Content-Encoding"] = "gzip"
		} else {
			body, err := gunzip(res.Body)
			if err != nil {
				return Response{}, false
			}
			res.Body = body
		}
	}
	return res, true
}

func (c *responseCache) remove(key string) {
	c.bytes -= len(c.entries[key].res.Body)
	delete(c.entries, key)
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

	stored := cachedResponse{res: *res}
	stored.res.Headers = maps.Clone(res.Headers)
	if worthGzipping(res) {
		if gz, err := gzipBytes(res.Body); err == nil {
			stored.res.Body, stored.gzipped = gz, true
		}
	}
	size := len(stored.res.Body)
	if size > cacheBytes/4 {
		return // (one response shouldn't push out most of the others)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.entries[key]; ok {
		c.remove(key)
	}
	if len(c.entries) >= cacheEntries || c.bytes+size > cacheBytes {
		now := time.Now()
		for k, e := range c.entries {
			if !now.Before(e.expires) {
				c.remove(k)
			}
		}
		for len(c.entries) > 0 && (len(c.entries) >= cacheEntries || c.bytes+size > cacheBytes) {
			var oldest string
			for k, e := range c.entries {
				if oldest == "" || e.expires.Before(c.entries[oldest].expires) {
					oldest = k
				}
			}
			c.remove(oldest)
		}
	}
	stored.expires = time.Now().Add(c.ttl)
	c.entries[key] = stored
	c.bytes += size
}

// worthGzipping is whether a response would be gzipped by gzipHandler.
func worthGzipping(res *Response) bool {
	if len(res.Body) < gzipMinSize {
		return false
	}
	if _, ok := headerKey(res.Headers, "Content-Encoding"); ok {
		return false
	}
	contentType := http.DetectContentType(res.Body)
	if k, ok := headerKey(res.Headers, "Content-Type"); ok {
		contentType = res.Headers[k]
	}
	return isCompressible(contentType)
}

func gzipBytes(b []byte) ([]byte, error) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(b); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func gunzip(b []byte) ([]byte, error) {
	zr, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	return io.ReadAll(zr)
}

func headerKey(headers map[string]string, key string) (string, bool) {
	for k := range headers {
		if strings.EqualFold(k, key) {
			return k, true
		}
	}
	return "", false
}
