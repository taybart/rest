package server_test

import (
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/taybart/rest/server"
)

// rawGet requests without the client's transparent gzip handling so headers can be inspected
func rawGet(t *testing.T, url string, headers map[string]string) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatalf("error creating request: %s", err)
	}
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{Transport: &http.Transport{DisableCompression: true}}
	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("error doing request: %s", err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("error reading response body: %s", err)
	}
	return res, body
}

func TestDirGzip(t *testing.T) {
	dir := t.TempDir()
	page := []byte("<!doctype html><p>" + strings.Repeat("hello world ", 200) + "</p>")
	small := []byte("<!doctype html><p>hi</p>")
	game := make([]byte, 64*1024)
	rand.Read(game)
	for name, b := range map[string][]byte{"page.html": page, "small.html": small, "game.nsp": game} {
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	ts := newServer(server.Config{Quiet: true, Dir: dir})
	defer ts.Close()

	t.Run("compresses text", func(t *testing.T) {
		res, body := rawGet(t, ts.URL+"/page.html", nil)
		if res.Header.Get("Content-Encoding") != "gzip" {
			t.Fatalf("expected gzip, got headers %v", res.Header)
		}
		if res.Header.Get("Vary") != "Accept-Encoding" {
			t.Errorf("expected Vary: Accept-Encoding, got %q", res.Header.Get("Vary"))
		}
		zr, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		got, err := io.ReadAll(zr)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, page) {
			t.Errorf("decompressed body does not match file")
		}
	})

	t.Run("binary passes through with length", func(t *testing.T) {
		res, body := rawGet(t, ts.URL+"/game.nsp", nil)
		if enc := res.Header.Get("Content-Encoding"); enc != "" {
			t.Errorf("expected no Content-Encoding, got %q", enc)
		}
		if cl := res.Header.Get("Content-Length"); cl != strconv.Itoa(len(game)) {
			t.Errorf("expected Content-Length %d, got %q", len(game), cl)
		}
		if res.Header.Get("Accept-Ranges") != "bytes" {
			t.Errorf("expected Accept-Ranges: bytes, got %q", res.Header.Get("Accept-Ranges"))
		}
		if !bytes.Equal(body, game) {
			t.Errorf("body does not match file")
		}
	})

	t.Run("range requests are never compressed", func(t *testing.T) {
		res, body := rawGet(t, ts.URL+"/page.html", map[string]string{"Range": "bytes=10-19"})
		if res.StatusCode != http.StatusPartialContent {
			t.Fatalf("expected 206, got %d", res.StatusCode)
		}
		if enc := res.Header.Get("Content-Encoding"); enc != "" {
			t.Errorf("expected no Content-Encoding, got %q", enc)
		}
		if !bytes.Equal(body, page[10:20]) {
			t.Errorf("expected %q, got %q", page[10:20], body)
		}
	})

	t.Run("small bodies are not compressed", func(t *testing.T) {
		res, body := rawGet(t, ts.URL+"/small.html", nil)
		if enc := res.Header.Get("Content-Encoding"); enc != "" {
			t.Errorf("expected no Content-Encoding, got %q", enc)
		}
		if !bytes.Equal(body, small) {
			t.Errorf("body does not match file")
		}
	})

	t.Run("not modified", func(t *testing.T) {
		res, _ := rawGet(t, ts.URL+"/page.html", nil)
		res, body := rawGet(t, ts.URL+"/page.html", map[string]string{"If-Modified-Since": res.Header.Get("Last-Modified")})
		if res.StatusCode != http.StatusNotModified {
			t.Fatalf("expected 304, got %d", res.StatusCode)
		}
		if len(body) != 0 || res.Header.Get("Content-Encoding") != "" {
			t.Errorf("expected empty uncompressed 304, got %d bytes, headers %v", len(body), res.Header)
		}
	})
}

// Handler responses are gzipped too, and cached ones are kept gzipped (and
// decompressed for clients that don't accept it).
func TestHandlerGzip(t *testing.T) {
	counter := `local n = (kv.get('n') or 0) + 1; kv.set('n', n)`
	long := strings.Repeat("hello world ", 200)
	ts := newServer(server.Config{Quiet: true, Handlers: []*server.Handler{
		{Method: "GET", Path: "/page", CacheTTL: time.Minute, Lang: "erde",
			Page: "local n = (kv.get('n') || 0) + 1; kv.set('n', n)\nreturn <p>{n} " + long + "</p>"},
		{Method: "GET", Path: "/fn", Fn: counter + `; return { status = 200, body = '` + long + `' }`},
		{Method: "GET", Path: "/small", Fn: `return { status = 200, body = 'hi' }`},
	}})
	defer ts.Close()

	gunzip := func(t *testing.T, b []byte) string {
		t.Helper()
		zr, err := gzip.NewReader(bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		out, _ := io.ReadAll(zr)
		return string(out)
	}
	plain := func(path string) (*http.Response, string) {
		res, body := rawGet(t, ts.URL+path, map[string]string{"Accept-Encoding": "identity"})
		return res, string(body)
	}

	// a page: rendered (gzipped on the fly), then from the cache (stored gzipped)
	for i, from := range []string{"render", "cache"} {
		res, body := rawGet(t, ts.URL+"/page", nil)
		if res.Header.Get("Content-Encoding") != "gzip" || res.Header.Get("Vary") != "Accept-Encoding" {
			t.Fatalf("%s: headers %v", from, res.Header)
		}
		if got := gunzip(t, body); !strings.Contains(got, "<p>1 hello world") {
			t.Fatalf("%s %d: body %.60q", from, i, got)
		}
	}
	// the cached one for a client w/o gzip
	res, body := plain("/page")
	if res.Header.Get("Content-Encoding") != "" || !strings.Contains(body, "<p>1 hello world") {
		t.Fatalf("plain cached page: headers %v, body %.60q", res.Header, body)
	}

	// fn handlers
	if res, body := rawGet(t, ts.URL+"/fn", nil); res.Header.Get("Content-Encoding") != "gzip" || gunzip(t, body) != long {
		t.Fatalf("fn: headers %v", res.Header)
	}
	if res, body := plain("/fn"); res.Header.Get("Content-Encoding") != "" || body != long {
		t.Fatalf("plain fn: headers %v", res.Header)
	}
	// small bodies aren't worth it
	if res, body := rawGet(t, ts.URL+"/small", nil); res.Header.Get("Content-Encoding") != "" || string(body) != "hi" {
		t.Fatalf("small: headers %v, body %q", res.Header, body)
	}
}
