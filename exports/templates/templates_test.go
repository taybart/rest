package templates_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/taybart/rest/exports/templates"
)

type received struct {
	Method string
	URL    string
	Header http.Header
	Body   string
}

// recorder captures every request it receives
func recorder(t *testing.T) (*httptest.Server, func() []received) {
	var mu sync.Mutex
	var got []received
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		got = append(got, received{r.Method, r.URL.RequestURI(), r.Header.Clone(), string(body)})
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv, func() []received {
		mu.Lock()
		defer mu.Unlock()
		return got
	}
}

// awkward values that break naive quoting in every target language
const nastyHeader = `it's "quoted" $HOME ` + "`tick`" + ` \n ${x}`

// bodies can also carry non-ascii, header values can't portably (fetch rejects them)
const nasty = nastyHeader + " 💥"

func testRequests(base string) []templates.Request {
	return []templates.Request{
		{
			Label:          "post json",
			Method:         "POST",
			BaseURL:        base + "/json",
			Query:          url.Values{"a": {"1"}, "b": {"two words & more"}},
			RequestHeaders: map[string]string{"Content-Type": "application/json", "X-Nasty": nastyHeader},
			Auth:           templates.Auth{Type: "basic", User: "user", Password: "p:a'ss"},
			UserAgent:      "custom-agent/1.0",
			Cookies:        map[string]string{"session": "abc", "other": "def"},
			Body:           `{"msg":` + jsonString(nasty) + `,"n":1}`,
			Expect:         templates.Expect{Status: 200},
		},
		{
			Label:   "2-get thing",
			Method:  "GET",
			BaseURL: base + "/get",
			Auth:    templates.Auth{Type: "bearer", Token: "tok'en"},
		},
		{
			Label:          "put text",
			Method:         "PUT",
			BaseURL:        base + "/text",
			RequestHeaders: map[string]string{"Content-Type": "text/plain"},
			Body:           nasty,
		},
	}
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func jsonEqual(a, b string) bool {
	var x, y any
	if json.Unmarshal([]byte(a), &x) != nil || json.Unmarshal([]byte(b), &y) != nil {
		return false
	}
	return reflect.DeepEqual(x, y)
}

func render(t *testing.T, name string, reqs []templates.Request) string {
	t.Helper()
	tmpl, err := templates.Get(name)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := tmpl.Execute(&out, reqs); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

func checkReceived(t *testing.T, reqs []templates.Request, got []received, base string) {
	t.Helper()
	if len(got) != len(reqs) {
		t.Fatalf("expected %d requests, server got %d", len(reqs), len(got))
	}
	for i, want := range reqs {
		g := got[i]
		if g.Method != want.Method {
			t.Errorf("%s: method %q != %q", want.Label, g.Method, want.Method)
		}
		if wantURL := strings.TrimPrefix(want.URL(), base); g.URL != wantURL {
			t.Errorf("%s: url %q != %q", want.Label, g.URL, wantURL)
		}
		for k, v := range want.Headers() {
			if g.Header.Get(k) != v {
				t.Errorf("%s: header %s %q != %q", want.Label, k, g.Header.Get(k), v)
			}
		}
		for k, v := range want.Cookies {
			if !strings.Contains(g.Header.Get("Cookie"), k+"="+v) {
				t.Errorf("%s: cookie %s=%s missing from %q", want.Label, k, v, g.Header.Get("Cookie"))
			}
		}
		if want.IsJSON() {
			if !jsonEqual(g.Body, want.Body) {
				t.Errorf("%s: json body %q != %q", want.Label, g.Body, want.Body)
			}
		} else if g.Body != want.Body {
			t.Errorf("%s: body %q != %q", want.Label, g.Body, want.Body)
		}
	}
}

func lookPath(t *testing.T, bin string) string {
	p, err := exec.LookPath(bin)
	if err != nil {
		t.Skipf("%s not installed", bin)
	}
	return p
}

func run(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
}

func TestCurlSnippet(t *testing.T) {
	lookPath(t, "curl")
	srv, got := recorder(t)
	reqs := testRequests(srv.URL)
	snippet := render(t, "curl", reqs)

	dir := t.TempDir()
	script := filepath.Join(dir, "run.sh")
	// -sf so curl output doesn't clutter and http errors fail, set -e so a
	// broken command fails the script
	src := "set -e\n" + regexp.MustCompile(`(?m)^curl`).ReplaceAllString(snippet, "curl -sf")
	if err := os.WriteFile(script, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "sh", script)
	checkReceived(t, reqs, got(), srv.URL)
}

func TestGoSnippet(t *testing.T) {
	gobin := lookPath(t, "go")
	srv, got := recorder(t)
	reqs := testRequests(srv.URL)
	snippet := render(t, "go", reqs)

	program := `package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"
)

var _ = strings.NewReader

` + snippet + `
func main() {
	for _, fn := range []func() (*http.Response, error){postJson, request2GetThing, putText} {
		res, err := fn()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		res.Body.Close()
	}
}
`
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(program), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, dir, gobin, "run", "main.go")
	checkReceived(t, reqs, got(), srv.URL)
}

func TestJSSnippet(t *testing.T) {
	node := lookPath(t, "node")
	srv, got := recorder(t)
	reqs := testRequests(srv.URL)
	snippet := render(t, "js", reqs)

	program := snippet + `
(async () => {
  for (const fn of [postJson, request2GetThing, putText]) await fn();
})().catch((e) => { console.error(e); process.exit(1); });
`
	dir := t.TempDir()
	file := filepath.Join(dir, "main.mjs")
	if err := os.WriteFile(file, []byte(program), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, dir, node, file)
	checkReceived(t, reqs, got(), srv.URL)
}

func TestTemplateFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "http.tmpl")
	src := `{{.Method}} {{.URL}}{{range $k, $v := .Headers}}
{{$k}}: {{$v}}{{end}}`
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	out := render(t, file, []templates.Request{
		{Label: "a", Method: "GET", BaseURL: "http://x/a", RequestHeaders: map[string]string{"B": "2", "A": "1"}},
		{Label: "b", Method: "DELETE", BaseURL: "http://x/b"},
	})
	want := "GET http://x/a\nA: 1\nB: 2\n\nDELETE http://x/b\n"
	if out != want {
		t.Fatalf("got:\n%q\nwant:\n%q", out, want)
	}

	if _, err := templates.Get(filepath.Join(dir, "missing.tmpl")); err == nil {
		t.Fatal("expected error for missing template")
	}
}
