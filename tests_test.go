package rest_test

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/taybart/rest"
)

// testServer records the paths it was sent, /login returns a token that
// /me requires, /status/<code> responds with code
func testServer(t *testing.T) (*httptest.Server, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var hits []string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits = append(hits, r.URL.Path)
		mu.Unlock()
		switch {
		case r.URL.Path == "/login":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"token":"tok"}`)
		case r.URL.Path == "/me":
			if r.Header.Get("Authorization") != "Bearer tok" {
				w.WriteHeader(401)
				return
			}
			fmt.Fprint(w, `{}`)
		case strings.HasPrefix(r.URL.Path, "/status/"):
			var code int
			fmt.Sscanf(strings.TrimPrefix(r.URL.Path, "/status/"), "%d", &code)
			w.WriteHeader(code)
		}
	}))
	t.Cleanup(s.Close)
	return s, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string{}, hits...)
	}
}

func writeRest(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(dir, "main.rest")
}

func runTests(t *testing.T, files map[string]string, name string) (string, error) {
	t.Helper()
	r, err := rest.NewFile(writeRest(t, files))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err = r.RunTestsTo(&out, name)
	return out.String(), err
}

const requests = `
locals { url = "%s" }
request "login" {
  url = "${locals.url}/login"
  after = "rest.exports.token = json.decode(rest.res.body).token"
}
request "me" {
  url = "${locals.url}/me"
  bearer_token = exports::try("token", "none")
  expect = 200
}
request "costly" {
  skip = true
  url = "${locals.url}/status/200"
}
request "teapot" {
  url = "${locals.url}/status/418"
  expect = 200
}
request "teapot quiet hook" {
  url = "${locals.url}/status/418"
  expect = 200
  after = "print('hook ran')"
}
request "cleanup" {
  url = "${locals.url}/status/204"
}
`

func TestTestRunsOrderWithExports(t *testing.T) {
	s, hits := testServer(t)
	out, err := runTests(t, map[string]string{"main.rest": fmt.Sprintf(requests, s.URL) + `
test "flow" {
  order = ["login", "me", "costly", "me"]
}
`}, "")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	want := []string{"/login", "/me", "/status/200", "/me"}
	if got := hits(); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("hits %v, want %v", got, want)
	}
	if !strings.Contains(out, "PASS flow") {
		t.Fatalf("missing PASS:\n%s", out)
	}
}

func TestTestsAreIsolated(t *testing.T) {
	s, _ := testServer(t)
	out, err := runTests(t, map[string]string{"main.rest": fmt.Sprintf(requests, s.URL) + `
test "logs in" {
  order = ["login", "me"]
}
test "reuses login" {
  order = ["me"]
}
`}, "")
	if err == nil {
		t.Fatalf("second test should fail without the first's exports:\n%s", out)
	}
	if !strings.Contains(out, "PASS logs in") || !strings.Contains(out, "FAIL reuses login") {
		t.Fatalf("unexpected output:\n%s", out)
	}
	if !strings.Contains(out, "1 passed, 1 failed") {
		t.Fatalf("missing summary:\n%s", out)
	}
}

func TestTestStopsOnFailureAndCleansUp(t *testing.T) {
	s, hits := testServer(t)
	out, err := runTests(t, map[string]string{"main.rest": fmt.Sprintf(requests, s.URL) + `
test "fails" {
  order   = ["login", "teapot", "me"]
  cleanup = ["cleanup"]
}
`}, "")
	if err == nil {
		t.Fatalf("expected failure:\n%s", out)
	}
	want := []string{"/login", "/status/418", "/status/204"}
	if got := hits(); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("hits %v, want %v", got, want)
	}
	if !strings.Contains(out, "FAIL fails (1/3)") {
		t.Fatalf("unexpected output:\n%s", out)
	}
}

func TestTestChecksExpectWithAfterHook(t *testing.T) {
	s, _ := testServer(t)
	out, err := runTests(t, map[string]string{"main.rest": fmt.Sprintf(requests, s.URL) + `
test "hook" {
  order = ["teapot quiet hook"]
}
`}, "")
	if err == nil {
		t.Fatalf("expect should fail even though the hook doesn't check it:\n%s", out)
	}
	if !strings.Contains(out, "hook ran") {
		t.Fatalf("hook should still run:\n%s", out)
	}
}

func TestTestCallsImported(t *testing.T) {
	s, hits := testServer(t)
	_, err := runTests(t, map[string]string{
		"lib.rest": fmt.Sprintf(`
config { skip_imported = true }
request "ping" { url = "%s/status/200" }
test "ignored" { order = ["nope"] }
`, s.URL),
		"main.rest": `
imports = ["./lib.rest"]
config {
  skip_imported = true
  namespace_imports = true
}
test "imported" { order = ["lib::ping"] }
`}, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := hits(); len(got) != 1 {
		t.Fatalf("hits %v", got)
	}
}

func TestTestUnknownLabelOnlyFailsWhenRun(t *testing.T) {
	s, hits := testServer(t)
	files := map[string]string{"main.rest": fmt.Sprintf(requests, s.URL) + `
test "stale" {
  order = ["login", "gone"]
}
test "fine" {
  order = ["login"]
}
`}
	// running the file's requests isn't affected
	r, err := rest.NewFile(writeRest(t, files))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.RunLabelTo(&bytes.Buffer{}, "login"); err != nil {
		t.Fatal(err)
	}

	out, err := runTests(t, files, "stale")
	if err == nil || !strings.Contains(out, `request "gone" not found`) {
		t.Fatalf("err %v:\n%s", err, out)
	}
	// nothing from the stale test was sent
	if got := hits(); len(got) != 1 {
		t.Fatalf("hits %v", got)
	}
	if out, err := runTests(t, files, "fine"); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}

func TestTestNotFound(t *testing.T) {
	if _, err := runTests(t, map[string]string{"main.rest": `test "a" { order = ["x"] }`}, "b"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := runTests(t, map[string]string{"main.rest": `request "x" { url = "http://x" }`}, ""); err == nil {
		t.Fatal("expected error for a file without tests")
	}
}

func TestTestSeedsExports(t *testing.T) {
	var body string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := new(bytes.Buffer)
		b.ReadFrom(r.Body)
		body = r.URL.Path + " " + b.String()
	}))
	t.Cleanup(s.Close)
	out, err := runTests(t, map[string]string{"main.rest": fmt.Sprintf(`
locals { url = "%s" }
request "send" {
  method = "POST"
  url    = "${locals.url}/${exports.id}"
  body   = exports::try("payload", "{}")
  after  = "if rest.exports.payload.args.who ~= 'Frank' then fail('not seeded') end"
}
test "seeded" {
  exports = {
    id      = "abc"
    payload = { method = "admin", args = { who = "Frank" } }
  }
  order = ["send"]
}
`, s.URL)}, "seeded")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if want := `/abc {"args":{"who":"Frank"},"method":"admin"}`; body != want {
		t.Fatalf("got %q, want %q", body, want)
	}
}
