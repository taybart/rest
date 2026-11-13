package server_test

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/taybart/rest/server"
)

// precompressedDir makes a directory w/ app.js, app.js.br and app.js.gz (the
// "compressed" files are marked, not compressed: the server never reads them)
func precompressedDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"app.js":          "console.log('raw')",
		"app.js.br":       "BROTLI",
		"app.js.gz":       "GZIP",
		"index.html":      "<p>raw</p>",
		"index.html.br":   "BROTLI INDEX",
		"only.css.br":     "BROTLI W/O A RAW FILE",
		"page.unknownext": "raw",
	}
	now := time.Now()
	for name, body := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, now, now); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// fetch requests path w/ headers, w/o the client's own decompression
func fetch(t *testing.T, method, url string, headers map[string]string) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{Transport: &http.Transport{DisableCompression: true}}
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res, string(body)
}

func TestPrecompressed(t *testing.T) {
	dir := precompressedDir(t)
	ts := newServer(server.Config{Quiet: true, Dir: dir})
	defer ts.Close()

	cases := []struct {
		name, path, accept string
		wantBody, wantEnc  string
	}{
		{"brotli first", "/app.js", "gzip, deflate, br", "BROTLI", "br"},
		{"gzip", "/app.js", "gzip", "GZIP", "gzip"},
		{"br refused", "/app.js", "br;q=0, gzip", "GZIP", "gzip"},
		{"any", "/app.js", "*", "BROTLI", "br"},
		{"any but br", "/app.js", "br;q=0, *", "GZIP", "gzip"},
		{"neither", "/app.js", "identity", "console.log('raw')", ""},
		{"directory index", "/", "br", "BROTLI INDEX", "br"},
		{"no raw file", "/only.css", "br", "404 page not found\n", ""},
		{"asked for directly", "/app.js.br", "identity", "BROTLI", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res, body := fetch(t, "GET", ts.URL+c.path, map[string]string{"Accept-Encoding": c.accept})
			if body != c.wantBody {
				t.Errorf("body %q, want %q", body, c.wantBody)
			}
			if enc := res.Header.Get("Content-Encoding"); enc != c.wantEnc {
				t.Errorf("Content-Encoding %q, want %q", enc, c.wantEnc)
			}
			if c.wantEnc != "" {
				// the original's type, not the compressed file's (or a sniffed one)
				if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/javascript") && !strings.HasPrefix(ct, "text/html") {
					t.Errorf("Content-Type %q", ct)
				}
				if vary := res.Header.Values("Vary"); len(vary) != 1 || vary[0] != "Accept-Encoding" {
					t.Errorf("Vary %q, want it once", vary)
				}
			}
		})
	}

	t.Run("head", func(t *testing.T) {
		res, body := fetch(t, "HEAD", ts.URL+"/app.js", map[string]string{"Accept-Encoding": "br"})
		if body != "" || res.Header.Get("Content-Encoding") != "br" || res.Header.Get("Content-Length") != "6" {
			t.Errorf("body %q, headers %v", body, res.Header)
		}
	})

	t.Run("range gets the raw file", func(t *testing.T) {
		res, body := fetch(t, "GET", ts.URL+"/app.js", map[string]string{
			"Accept-Encoding": "br",
			"Range":           "bytes=0-6",
		})
		if res.StatusCode != http.StatusPartialContent || body != "console" || res.Header.Get("Content-Encoding") != "" {
			t.Errorf("status %d, body %q, headers %v", res.StatusCode, body, res.Header)
		}
	})
}

// A precompressed file much older than the original is ignored, so a missed
// rebuild doesn't serve old content. (A little older is fine: copying can
// write it first.)
func TestPrecompressedStale(t *testing.T) {
	for _, c := range []struct {
		name     string
		age      time.Duration
		wantBody string
	}{
		{"a little older", 5 * time.Second, "BROTLI"},
		{"stale", time.Hour, "GZIP"},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := precompressedDir(t)
			old := time.Now().Add(-c.age)
			if err := os.Chtimes(filepath.Join(dir, "app.js.br"), old, old); err != nil {
				t.Fatal(err)
			}
			ts := newServer(server.Config{Quiet: true, Dir: dir})
			defer ts.Close()

			_, body := fetch(t, "GET", ts.URL+"/app.js", map[string]string{"Accept-Encoding": "br, gzip"})
			if body != c.wantBody {
				t.Errorf("body %q, want %q", body, c.wantBody)
			}
		})
	}
}
