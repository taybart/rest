package server

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// a dev server with a page handler (with a cache, which dev mode ignores), a
// json one, and a directory
func newDevServer(t *testing.T, sources ...string) (*Server, *httptest.Server) {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "css"), 0o755)
	os.WriteFile(filepath.Join(dir, "css", "site.css"), []byte("body{}"), 0o644)
	s := New(Config{
		Quiet:      true,
		Dir:        dir,
		Dev:        true,
		DevSources: sources,
		Handlers: []*Handler{
			{Method: "GET", Path: "/page", Lang: "midnight", CacheTTL: time.Hour,
				Page: "local n = (kv.get('n') || 0) + 1; kv.set('n', n)\nreturn <html><body><p>{n}</p></body></html>"},
			{Method: "GET", Path: "/json", Fn: `return { status = 200, headers = { ['Content-Type'] = 'application/json' }, body = '{"a":"</body>"}' }`},
		},
	})
	ts := httptest.NewServer(s.Server.Handler)
	t.Cleanup(ts.Close)
	return s, ts
}

func get(t *testing.T, url string) (string, http.Header) {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return string(b), res.Header
}

func TestDevInjectAndNoCache(t *testing.T) {
	_, ts := newDevServer(t)
	first, h := get(t, ts.URL+"/page")
	second, _ := get(t, ts.URL+"/page")
	if !strings.Contains(first, "<p>1</p>") || !strings.Contains(second, "<p>2</p>") {
		t.Errorf("cached in dev mode:\n%s\n%s", first, second)
	}
	if h.Get("Cache-Control") != "" {
		t.Errorf("Cache-Control in dev mode: %q", h.Get("Cache-Control"))
	}
	// the script goes before </body>, once
	if strings.Count(first, "data-rest-dev") != 1 || !strings.Contains(first, "</script></body>") {
		t.Errorf("no dev script before </body>:\n%s", first)
	}
	if body, _ := get(t, ts.URL+"/json"); body != `{"a":"</body>"}` {
		t.Errorf("injected into json: %s", body)
	}
}

func TestDevNotOnByDefault(t *testing.T) {
	s := New(Config{Quiet: true, Handlers: []*Handler{
		{Method: "GET", Path: "/page", Lang: "midnight", Page: "return <html><body>hi</body></html>"},
	}})
	ts := httptest.NewServer(s.Server.Handler)
	defer ts.Close()
	if body, _ := get(t, ts.URL+"/page"); strings.Contains(body, "data-rest-dev") {
		t.Error("dev script without dev mode")
	}
	if res, _ := http.Get(ts.URL + DevEventsPath); res.StatusCode == http.StatusOK {
		t.Error("dev events without dev mode")
	}
}

func TestDevIdiomorph(t *testing.T) {
	_, ts := newDevServer(t)
	body, h := get(t, ts.URL+devMorphPath)
	if !strings.HasPrefix(body, "var Idiomorph=") || !strings.Contains(h.Get("Content-Type"), "javascript") {
		t.Errorf("%s: %.40s", h.Get("Content-Type"), body)
	}
}

// what a batch of changes does: sources restart, css under the directory is
// swapped, anything else reloads
func TestDevHandle(t *testing.T) {
	wd, _ := os.Getwd()
	src := filepath.Join(wd, "site.rest")
	s, ts := newDevServer(t, src)
	dirRel, _ := filepath.Rel(wd, s.dev.dir)

	res, err := http.Get(ts.URL + DevEventsPath)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	events := bufio.NewReader(res.Body)
	next := func() string {
		t.Helper()
		done := make(chan string, 1)
		go func() {
			var event, data string
			for {
				line, err := events.ReadString('\n')
				if err != nil {
					done <- "error: " + err.Error()
					return
				}
				line = strings.TrimSpace(line)
				if v, ok := strings.CutPrefix(line, "event: "); ok {
					event = v
				} else if v, ok := strings.CutPrefix(line, "data: "); ok {
					data = v
				} else if line == "" && event != "" {
					done <- event + " " + data
					return
				}
			}
		}()
		select {
		case e := <-done:
			return e
		case <-time.After(time.Second):
			t.Fatal("no event")
			return ""
		}
	}
	for range 100 { // (wait for the page to be connected)
		s.dev.mu.Lock()
		n := len(s.dev.clients)
		s.dev.mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	if !s.dev.handle([]string{"site.rest"}) {
		t.Error("a source didn't restart")
	}
	if s.dev.handle([]string{filepath.Join(dirRel, "css", "site.css")}) {
		t.Error("css restarted")
	}
	if e := next(); e != "css /css/site.css" {
		t.Errorf("css: %q", e)
	}
	if s.dev.handle([]string{"src/docs/intro.md", filepath.Join(dirRel, "css", "site.css")}) {
		t.Error("markdown restarted")
	}
	if e := next(); e != "reload " {
		t.Errorf("reload: %q", e)
	}
	// css outside the directory isn't served, so it reloads
	s.dev.handle([]string{"other/x.css"})
	if e := next(); e != "reload " {
		t.Errorf("css outside: %q", e)
	}
}

// under run -w: hello, then a restart for a source change
func TestDevListen(t *testing.T) {
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	runSide := os.NewFile(uintptr(fds[0]), "run")
	defer runSide.Close()
	t.Setenv("RUN_EVENTS_FD", fmt.Sprint(fds[1]))

	wd, _ := os.Getwd()
	s, _ := newDevServer(t, filepath.Join(wd, "site.rest"))
	go s.dev.listen()

	r := bufio.NewReader(runSide)
	read := func() string {
		t.Helper()
		runSide.SetReadDeadline(time.Now().Add(time.Second))
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(line)
	}
	if got := read(); got != "hello" {
		t.Fatalf("got %q", got)
	}
	fmt.Fprint(runSide, "change modified src/a.md\ndone\nchange modified site.rest\ndone\n")
	if got := read(); got != "restart" {
		t.Fatalf("got %q", got)
	}
}

// script errors show in the browser in dev mode: page loads get an html page
// with the error (and the dev script), fetches (ex. actions) get it as text
func TestDevErrors(t *testing.T) {
	handlers := func() []*Handler {
		return []*Handler{
			{Method: "GET", Path: "/broken", Lang: "midnight",
				Page: "local t = nil\nreturn <html><body>{t.x}<b>&</b></body></html>"},
			{Method: "POST", Path: "/act", Fn: "error('boom <b>')"},
		}
	}
	do := func(t *testing.T, url, method, accept string) (int, string, http.Header) {
		t.Helper()
		req, _ := http.NewRequest(method, url, nil)
		if accept != "" {
			req.Header.Set("Accept", accept)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(b), res.Header
	}

	s := New(Config{Quiet: true, Dev: true, Handlers: handlers()})
	ts := httptest.NewServer(s.Server.Handler)
	defer ts.Close()

	// a page load
	status, body, h := do(t, ts.URL+"/broken", "GET", "text/html,application/xhtml+xml,*/*;q=0.8")
	if status != 500 || h.Get(DevErrorHeader) == "" || !strings.HasPrefix(h.Get("Content-Type"), "text/html") {
		t.Errorf("page load: %d %v", status, h)
	}
	if !strings.Contains(body, "data-rest-error") || !strings.Contains(body, "attempt to index") ||
		strings.Count(body, "data-rest-dev>") != 1 {
		t.Errorf("page load, no error or dev script:\n%s", body)
	}
	// the dev script's re-fetch
	status, body, h = do(t, ts.URL+"/broken", "GET", "*/*")
	if status != 500 || h.Get(DevErrorHeader) == "" || !strings.HasPrefix(h.Get("Content-Type"), "text/plain") ||
		!strings.Contains(body, "attempt to index") || strings.Contains(body, "<script") {
		t.Errorf("fetch: %d %v\n%s", status, h, body)
	}
	// an action (as text, not escaped)
	status, body, h = do(t, ts.URL+"/act", "POST", "text/html")
	if status != 500 || h.Get(DevErrorHeader) == "" || !strings.Contains(body, "boom <b>") {
		t.Errorf("action: %d %v\n%s", status, h, body)
	}

	// without dev mode, only the status
	s = New(Config{Quiet: true, Handlers: handlers()})
	prod := httptest.NewServer(s.Server.Handler)
	defer prod.Close()
	status, body, h = do(t, prod.URL+"/broken", "GET", "text/html")
	if status != 500 || body != "" || h.Get(DevErrorHeader) != "" {
		t.Errorf("not dev: %d %v %q", status, h, body)
	}
}

// the html error page escapes the error
func TestDevErrorPageEscapes(t *testing.T) {
	d := newDevMode(Config{})
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Accept", "text/html")
	res := d.errorResponse(req, fmt.Errorf("<script>alert(1)</script>"))
	if strings.Contains(string(res.Body), "<script>alert") || !strings.Contains(string(res.Body), "&lt;script&gt;alert") {
		t.Errorf("not escaped:\n%s", res.Body)
	}
}
