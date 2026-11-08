package pages_test

import (
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	lua "github.com/taybart/midnight"
	restlua "github.com/taybart/rest/lua"
	"github.com/taybart/rest/pages"
)

var actionID = regexp.MustCompile(`midnight\.rpc\('([^']+)'`)

func get(t *testing.T, url string) (*http.Response, string) {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	return res, string(body)
}

func post(t *testing.T, url, state, body string) (*http.Response, string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if state != "" {
		req.Header.Set(pages.StateHeader, state)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res, string(b)
}

// A file page w/ props, a required module, and an action, on a std mux.
func TestFilePage(t *testing.T) {
	eng := pages.New(pages.Options{OnError: func(_ *http.Request, err error) { t.Error(err) }})
	defer eng.Close()
	mux := http.NewServeMux()
	mux.Handle("/app", eng.File("testdata/app.mn", pages.Props(func(r *http.Request) (any, error) {
		return map[string]any{"name": r.URL.Query().Get("name")}, nil
	})))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	res, html := get(t, ts.URL+"/app?name=gopher")
	if res.StatusCode != 200 || !strings.Contains(html, "<h1>hello gopher</h1>") || !strings.Contains(html, "count 0") {
		t.Fatalf("render: %d\n%s", res.StatusCode, html)
	}
	if ct := res.Header.Get("Content-Type"); ct != pages.HTMLContentType {
		t.Errorf("content type %q", ct)
	}
	// the client config has the state in its headers: { 'X-Rest-State': '<id>' }
	state := regexp.MustCompile(`["']` + pages.StateHeader + `["']:\s*["']([^"']+)`).FindStringSubmatch(html)
	ids := actionID.FindStringSubmatch(html)
	if state == nil || ids == nil {
		t.Fatalf("no state / action in\n%s", html)
	}
	stateID := state[1]

	res, body := post(t, ts.URL+"/app", stateID, `{"id":"`+ids[1]+`"}`)
	if res.StatusCode != 200 || !strings.Contains(body, "count 1") {
		t.Fatalf("action: %d %s", res.StatusCode, body)
	}

	if res, _ := post(t, ts.URL+"/app", "", `{}`); res.StatusCode != http.StatusBadRequest {
		t.Errorf("w/o a state: %d", res.StatusCode)
	}
	if res, _ := post(t, ts.URL+"/app", "nope", `{"id":"x"}`); res.StatusCode != http.StatusGone {
		t.Errorf("unknown state: %d", res.StatusCode)
	}
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/app", nil)
	if res, _ := http.DefaultClient.Do(req); res.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("delete: %d", res.StatusCode)
	}
}

// Source pages, static pages (whose state isn't kept), redirects, and errors.
func TestSourcePage(t *testing.T) {
	var errs []error
	eng := pages.New(pages.Options{OnError: func(_ *http.Request, err error) { errs = append(errs, err) }})
	defer eng.Close()
	mux := http.NewServeMux()
	mux.Handle("/static", eng.Source(`return <p>hi {rest.req:query('x')}</p>`))
	mux.Handle("/redirect", eng.Source(`return { status = 302, headers = { Location = '/static' } }`))
	mux.Handle("/broken", eng.Source(`return 42`))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	if res, html := get(t, ts.URL+"/static?x=there"); res.StatusCode != 200 || !strings.Contains(html, "<p>hi there</p>") || strings.Contains(html, "midnight.rpc") {
		t.Errorf("static: %d\n%s", res.StatusCode, html)
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	if res, err := client.Get(ts.URL + "/redirect"); err != nil || res.StatusCode != 302 || res.Header.Get("Location") != "/static" {
		t.Errorf("redirect: %v %v", res, err)
	}
	if res, _ := get(t, ts.URL+"/broken"); res.StatusCode != 500 || len(errs) != 1 {
		t.Errorf("broken: %d, errors %v", res.StatusCode, errs)
	}
}

// rest.request runs the engine's Requester w/ rest.exports, which start from
// the engine's and page's Exports, and are kept (w/ what requests export) for
// the page's actions.
func TestExports(t *testing.T) {
	var seen []map[string]any
	requester := func(target any, exports map[string]any) (*restlua.RequestResult, error) {
		if target != "whoami" {
			return nil, fmt.Errorf("no request %v", target)
		}
		seen = append(seen, maps.Clone(exports))
		calls, _ := exports["calls"].(float64)
		return &restlua.RequestResult{
			Exports: map[string]any{"calls": calls + 1},
			Returns: []any{fmt.Sprintf("%s@%s", exports["token"], exports["base"])},
		}, nil
	}
	eng := pages.New(pages.Options{
		Requester: requester,
		Exports:   map[string]any{"base": "api", "token": "engine"},
		OnError:   func(_ *http.Request, err error) { t.Error(err) },
	})
	defer eng.Close()
	src := `
local who = rest.request('whoami')
local App = () -> <button onclick={() -> { who = rest.request('whoami') }}>{who} calls={rest.exports.calls}</button>
return <App/>`
	mux := http.NewServeMux()
	mux.Handle("/", eng.Source(src, pages.Exports(func(r *http.Request) (map[string]any, error) {
		return map[string]any{"token": r.URL.Query().Get("token")}, nil
	})))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	_, html := get(t, ts.URL+"/?token=visitor")
	if !strings.Contains(html, "visitor@api calls=1") {
		t.Fatalf("render:\n%s", html)
	}
	state := regexp.MustCompile(`["']` + pages.StateHeader + `["']:\s*["']([^"']+)`).FindStringSubmatch(html)
	ids := actionID.FindStringSubmatch(html)
	if state == nil || ids == nil {
		t.Fatalf("no state / action in\n%s", html)
	}
	// the action has no token in its url: the exports are the page's
	_, body := post(t, ts.URL+"/", state[1], `{"id":"`+ids[1]+`"}`)
	if !strings.Contains(body, "visitor@api calls=2") {
		t.Fatalf("action: %s", body)
	}
	if len(seen) != 2 || seen[1]["calls"] != float64(1) {
		t.Errorf("requests saw %v", seen)
	}
}

// W/o a Requester, rest.request is an error.
func TestNoRequester(t *testing.T) {
	var errs []error
	eng := pages.New(pages.Options{OnError: func(_ *http.Request, err error) { errs = append(errs, err) }})
	defer eng.Close()
	ts := httptest.NewServer(eng.Source(`return <p>{rest.request('x')}</p>`))
	defer ts.Close()
	if res, _ := get(t, ts.URL); res.StatusCode != 500 || len(errs) != 1 || !strings.Contains(errs[0].Error(), "no request blocks") {
		t.Errorf("%d %v", res.StatusCode, errs)
	}
}

// Actions POST to the page's own url as the browser sees it (rpc: null in the
// page), so a page mounted under a stripped prefix works. ActionURL sets it.
func TestActionURL(t *testing.T) {
	eng := pages.New(pages.Options{OnError: func(_ *http.Request, err error) { t.Error(err) }})
	defer eng.Close()
	counter := `
local n = 0
local App = () -> <button onclick={() -> { n += 1 }}>n={n}</button>
return <App/>`
	mux := http.NewServeMux()
	mux.Handle("/app/", http.StripPrefix("/app", eng.Source(counter)))
	mux.Handle("/other", eng.Source(counter, pages.ActionURL("/actions")))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	_, html := get(t, ts.URL+"/app/counter")
	cfg := regexp.MustCompile(`rpc: null, headers: \{ '` + pages.StateHeader + `': '([^']+)'`).FindStringSubmatch(html)
	ids := actionID.FindStringSubmatch(html)
	if cfg == nil || ids == nil {
		t.Fatalf("page:\n%s", html)
	}
	// what the client does: POST to location.pathname + location.search
	if res, body := post(t, ts.URL+"/app/counter", cfg[1], `{"id":"`+ids[1]+`"}`); res.StatusCode != 200 || !strings.Contains(body, "n=1") {
		t.Errorf("action under a prefix: %d %s", res.StatusCode, body)
	}

	if _, html := get(t, ts.URL+"/other"); !strings.Contains(html, "rpc: '/actions',") {
		t.Errorf("ActionURL:\n%s", html)
	}
}

// LuaFuncs are a module of the page; Request is the request being handled
// (the action's for actions, not the page load's), in coroutines too.
func TestLuaFuncs(t *testing.T) {
	eng := pages.New(pages.Options{OnError: func(_ *http.Request, err error) { t.Error(err) }})
	defer eng.Close()
	auth := map[string]lua.LGFunction{
		"user": func(l *lua.LState) int {
			l.Push(lua.LString(pages.Request(l).Header.Get("X-User")))
			return 1
		},
		"must": func(l *lua.LState) int {
			if !l.ToBool(1) {
				l.RaiseError("not allowed")
			}
			return 0
		},
	}
	src := `
local auth = require('app.auth')
local loaded, clicked = auth.user(), ''
local ok, err = pcall(auth.must, false)
local App = () -> <button onclick={() -> { clicked = coroutine.wrap(() -> auth.user())() }}>
  loaded={loaded} clicked={clicked} err={err}
</button>
return <App/>`
	ts := httptest.NewServer(eng.Source(src, pages.LuaFuncs("app.auth", auth)))
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL, nil)
	req.Header.Set("X-User", "alice")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	html := string(b)
	if !strings.Contains(html, "loaded=alice clicked= err=") || !strings.Contains(html, "not allowed") {
		t.Fatalf("render:\n%s", html)
	}
	state := regexp.MustCompile(`["']` + pages.StateHeader + `["']:\s*["']([^"']+)`).FindStringSubmatch(html)
	ids := actionID.FindStringSubmatch(html)
	if state == nil || ids == nil {
		t.Fatalf("no state / action in\n%s", html)
	}

	req, _ = http.NewRequest(http.MethodPost, ts.URL, strings.NewReader(`{"id":"`+ids[1]+`"}`))
	req.Header.Set(pages.StateHeader, state[1])
	req.Header.Set("X-User", "bob")
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ = io.ReadAll(res.Body)
	res.Body.Close()
	if !strings.Contains(string(b), "loaded=alice clicked=bob") {
		t.Errorf("action: %s", b)
	}
}
