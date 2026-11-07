package server_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/taybart/rest/server"
)

type Response struct {
	StatusCode int               `json:"status"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
}

func checkResponse(t *testing.T, req *http.Request, expected Response) {
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Errorf("error reading response: %s", err)
	}
	if res.StatusCode != expected.StatusCode {
		t.Errorf("expected status code %d, got %d", expected.StatusCode, res.StatusCode)
	}
	for k, v := range expected.Headers {
		if got := res.Header.Get(k); got != v {
			t.Errorf("expected header %s=%q, got %q", k, v, got)
		}
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Errorf("error reading response body: %s", err)
	}
	if string(body) != expected.Body {
		t.Errorf(`expected body %s, got %s`, expected.Body, body)
	}
}

func newServer(config server.Config) *httptest.Server {
	s := server.Server{
		Router: http.NewServeMux(),
		Config: config,
	}
	s.Routes(&http.Server{})
	ts := httptest.NewServer(s.Router)
	return ts
}

func TestRoot(t *testing.T) {
	ts := newServer(server.Config{
		Quiet: true,
	})
	defer ts.Close()
	req, err := http.NewRequest("GET", ts.URL+"/test/sinkhold/route", nil)
	if err != nil {
		t.Errorf("error creating request: %s", err)
	}
	checkResponse(t, req, Response{StatusCode: http.StatusOK, Body: `{"status": "ok"}`})
}

func TestResponse(t *testing.T) {
	f, err := os.ReadFile("../test/response.json")
	if err != nil {
		t.Errorf("error reading response.json: %s", err)
	}

	var res server.Response
	err = json.Unmarshal(f, &res)
	if err != nil {
		t.Errorf("error marshalling response.json: %s", err)
	}

	ts := newServer(server.Config{
		Quiet:    true,
		Response: &res,
	})
	defer ts.Close()
	req, err := http.NewRequest("GET", ts.URL+"/response/test", nil)
	if err != nil {
		t.Errorf("error creating request: %s", err)
	}
	checkResponse(t, req, Response{StatusCode: res.Status, Body: string(res.Body)})
}

func TestDir(t *testing.T) {
	f, err := os.ReadFile("../test/response.json")
	if err != nil {
		t.Errorf("error reading response.json: %s", err)
	}

	ts := newServer(server.Config{
		Quiet: true,
		Dir:   "../test",
	})
	defer ts.Close()
	req, err := http.NewRequest("GET", ts.URL+"/response.json", nil)
	if err != nil {
		t.Errorf("error creating request: %s", err)
	}
	checkResponse(t, req, Response{StatusCode: http.StatusOK, Body: string(f)})
}

// A directory serves what the handlers don't match.
func TestDirWithHandlers(t *testing.T) {
	ts := newServer(server.Config{
		Quiet: true,
		Dir:   "../test",
		Handlers: []*server.Handler{
			{Method: "GET", Path: "/{$}", Response: &server.Response{Status: 200, Body: []byte(`"root"`)}},
			{Method: "GET", Path: "/response.json", Response: &server.Response{Status: 200, Body: []byte(`"handler"`)}},
		},
	})
	defer ts.Close()
	get := func(path string) Response {
		res, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		body, _ := io.ReadAll(res.Body)
		return Response{StatusCode: res.StatusCode, Body: string(body)}
	}
	cases := []struct {
		path string
		want Response
	}{
		{"/", Response{StatusCode: 200, Body: `"root"`}},
		{"/response.json", Response{StatusCode: 200, Body: `"handler"`}}, // handlers win
		{"/jsx/../response.json", Response{StatusCode: 200, Body: `"handler"`}},
		{"/cli-tool/client.rest", Response{StatusCode: 200}},
		{"/nope.txt", Response{StatusCode: 404}},
	}
	for _, c := range cases {
		got := get(c.path)
		if got.StatusCode != c.want.StatusCode || (c.want.Body != "" && got.Body != c.want.Body) {
			t.Errorf("%s: got %d %q, want %d %q", c.path, got.StatusCode, got.Body, c.want.StatusCode, c.want.Body)
		}
	}
}

func TestEcho(t *testing.T) {
	ts := newServer(server.Config{
		Quiet: true,
	})
	defer ts.Close()
	req, err := http.NewRequest("POST", ts.URL+"/__echo__", strings.NewReader(`{"data": "hello"}`))
	if err != nil {
		t.Errorf("error creating request: %s", err)
	}
	checkResponse(t, req, Response{StatusCode: http.StatusOK, Body: `{"data": "hello"}`})
}

// Handlers on "/" w/ a method (ex. "GET /") used to conflict w/ the built in
// "/__quit__" route, which panicked on startup.
func TestRootHandlerWithMethod(t *testing.T) {
	for _, method := range []string{"GET", "POST", "*"} {
		ts := newServer(server.Config{
			Quiet: true,
			Handlers: []*server.Handler{
				{Method: method, Path: "/", Fn: `return { status = 200, body = 'root' }`},
			},
		})
		req, err := http.NewRequest("POST", ts.URL+"/", nil)
		if err != nil {
			t.Fatalf("error creating request: %s", err)
		}
		if method == "GET" {
			checkResponse(t, req, Response{StatusCode: http.StatusNotFound, Body: ""})
		} else {
			checkResponse(t, req, Response{StatusCode: http.StatusOK, Body: "root"})
		}
		ts.Close()
	}
}

func TestHandlerFnPathValue(t *testing.T) {
	ts := newServer(server.Config{
		Quiet: true,
		Handlers: []*server.Handler{
			{
				Method: "POST",
				Path:   "/user/{id}",
				Fn: `
					local id = rest.req:path_value("id")
					local token = rest.req:bearer_token()
					local custom = rest.req:header("x-custom")
					local q = rest.req:query("filter")
					local session = rest.req:cookie("session")
					local body = form.decode(rest.req.body)
					return {
						status = 200,
						cookies = {
							token = {
								value = token,
								path = "/",
								http_only = true,
								same_site = "strict",
								secure = true,
							},
						},
						body = form.encode({
							id = id,
							token = token,
							custom = custom,
							filter = q,
							session = session,
							name = body.name,
						}),
					}
				`,
			},
		},
	})
	defer ts.Close()
	req, err := http.NewRequest("POST", ts.URL+"/user/42?filter=active", strings.NewReader("name=alice+smith"))
	if err != nil {
		t.Fatalf("error creating request: %s", err)
	}
	req.Header.Set("Authorization", "Bearer secret-token")
	req.Header.Set("X-Custom", "hello")
	req.AddCookie(&http.Cookie{Name: "session", Value: "abc123"})
	checkResponse(t, req, Response{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Set-Cookie": "token=secret-token; Path=/; HttpOnly; Secure; SameSite=Strict",
		},
		Body: `custom=hello&filter=active&id=42&name=alice+smith&session=abc123&token=secret-token`,
	})
}

func TestMiddleware(t *testing.T) {
	ts := newServer(server.Config{
		Quiet: true,
		Middlewares: []*server.Middleware{
			{
				Name: "auth",
				Fn: `
					local token = rest.req:cookie("token") or rest.req:bearer_token()
					if token ~= "valid-token" then
						return {
							status = 401,
							body = "unauthorized",
						}
					end
					rest.ctx.user = "frankie"
				`,
			},
		},
		Handlers: []*server.Handler{
			{
				Method:     "GET",
				Path:       "/user/{id}",
				Middleware: []string{"auth"},
				Fn: `
					return {
						status = 200,
						body = rest.req:path_value("id") .. ":" .. rest.ctx.user,
					}
				`,
			},
		},
	})
	defer ts.Close()

	// 1. Unauthorized request short-circuits in middleware
	unauthReq, err := http.NewRequest("GET", ts.URL+"/user/7", nil)
	if err != nil {
		t.Fatalf("error creating request: %s", err)
	}
	checkResponse(t, unauthReq, Response{
		StatusCode: http.StatusUnauthorized,
		Body:       "unauthorized",
	})

	// 2. Authorized request via cookie passes rest.ctx to handler
	authReq, err := http.NewRequest("GET", ts.URL+"/user/7", nil)
	if err != nil {
		t.Fatalf("error creating request: %s", err)
	}
	authReq.AddCookie(&http.Cookie{Name: "token", Value: "valid-token"})
	checkResponse(t, authReq, Response{
		StatusCode: http.StatusOK,
		Body:       "7:frankie",
	})
}

func TestErdeMiddleware(t *testing.T) {
	ts := newServer(server.Config{
		Quiet: true,
		Middlewares: []*server.Middleware{
			{
				Name: "auth",
				Lang: "erde",
				Fn: `
					local token = rest.req:cookie("token") || rest.req:bearer_token()
					if token != "valid-token" {
						return { status = 401, body = "unauthorized" }
					}
					rest.ctx.user = "frankie"
				`,
			},
		},
		Handlers: []*server.Handler{
			{
				Method:     "GET",
				Path:       "/user/{id}",
				Middleware: []string{"auth"},
				Lang:       "erde",
				Fn: `
					local id = rest.req:path_value("id")
					return { status = 200, body = "{id}:{rest.ctx.user}" }
				`,
			},
		},
	})
	defer ts.Close()

	unauthReq, err := http.NewRequest("GET", ts.URL+"/user/7", nil)
	if err != nil {
		t.Fatalf("error creating request: %s", err)
	}
	checkResponse(t, unauthReq, Response{
		StatusCode: http.StatusUnauthorized,
		Body:       "unauthorized",
	})

	authReq, err := http.NewRequest("GET", ts.URL+"/user/7", nil)
	if err != nil {
		t.Fatalf("error creating request: %s", err)
	}
	authReq.AddCookie(&http.Cookie{Name: "token", Value: "valid-token"})
	checkResponse(t, authReq, Response{
		StatusCode: http.StatusOK,
		Body:       "7:frankie",
	})
}

func TestServerKVAndSetup(t *testing.T) {
	ts := newServer(server.Config{
		Quiet: true,
		KV: map[string]any{
			"db": map[string]any{
				"users": map[string]any{
					"1": map[string]any{"name": "Bryan Cranston", "age": float64(30)},
					"4": map[string]any{"name": "Frankie Muniz", "age": float64(15)},
				},
			},
		},
		Setup: `
			kv.set("greeting", "hello")
		`,
		Handlers: []*server.Handler{
			{
				Method: "GET",
				Path:   "/user/{id}",
				Fn: `
					local id = rest.req:path_value("id")
					local db = kv.get("db")
					local user = db.users[id]
					if not user then
						return { status = 404, body = "not found" }
					end
					return {
						status = 200,
						body = kv.get("greeting") .. ":" .. user.name .. ":" .. tostring(user.age),
					}
				`,
			},
		},
	})
	defer ts.Close()

	req, err := http.NewRequest("GET", ts.URL+"/user/4", nil)
	if err != nil {
		t.Fatalf("error creating request: %s", err)
	}
	checkResponse(t, req, Response{
		StatusCode: http.StatusOK,
		Body:       "hello:Frankie Muniz:15",
	})
}

func TestJWTServerAndMiddleware(t *testing.T) {
	ts := newServer(server.Config{
		Quiet: true,
		Setup: `
			kv.set("jwt_secret", "super-secret-key")
		`,
		Middlewares: []*server.Middleware{
			{
				Name: "jwt_auth",
				Fn: `
					local token = rest.req:bearer_token()
					if not token then
						return { status = 401, body = "missing token" }
					end
					local claims, err = jwt.verify(token, kv.get("jwt_secret"))
					if not claims then
						return { status = 401, body = err }
					end
					rest.ctx.claims = claims
				`,
			},
		},
		Handlers: []*server.Handler{
			{
				Method: "POST",
				Path:   "/login",
				Fn: `
					local body = json.decode(rest.req.body)
					local token = jwt.sign({ sub = body.user, role = "admin" }, kv.get("jwt_secret"), { expires_in = 300 })
					return {
						status = 200,
						body = json.encode({ token = token }),
					}
				`,
			},
			{
				Method:     "GET",
				Path:       "/me",
				Middleware: []string{"jwt_auth"},
				Fn: `
					return {
						status = 200,
						body = rest.ctx.claims.sub .. ":" .. rest.ctx.claims.role,
					}
				`,
			},
		},
	})
	defer ts.Close()

	// 1. Login to get a signed JWT
	loginReq, err := http.NewRequest("POST", ts.URL+"/login", strings.NewReader(`{"user":"malcolm"}`))
	if err != nil {
		t.Fatal(err)
	}
	loginRes, err := http.DefaultClient.Do(loginReq)
	if err != nil {
		t.Fatal(err)
	}
	defer loginRes.Body.Close()
	var loginPayload struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(loginRes.Body).Decode(&loginPayload); err != nil {
		t.Fatal(err)
	}
	if loginPayload.Token == "" {
		t.Fatal("expected non-empty jwt token")
	}

	// 2. Call /me with valid JWT
	meReq, err := http.NewRequest("GET", ts.URL+"/me", nil)
	if err != nil {
		t.Fatal(err)
	}
	meReq.Header.Set("Authorization", "Bearer "+loginPayload.Token)
	checkResponse(t, meReq, Response{
		StatusCode: http.StatusOK,
		Body:       "malcolm:admin",
	})

	// 3. Call /me with tampered JWT
	badReq, err := http.NewRequest("GET", ts.URL+"/me", nil)
	if err != nil {
		t.Fatal(err)
	}
	badReq.Header.Set("Authorization", "Bearer "+loginPayload.Token+"tampered")
	checkResponse(t, badReq, Response{
		StatusCode: http.StatusUnauthorized,
		Body:       `invalid jwt signature`,
	})
}

// Handlers (and middleware) can return markup elements, which are rendered as
// HTML documents.
func TestErdeMarkupHandlers(t *testing.T) {
	const page = `local App = props -> <main><h1>hi {props.name}</h1></main>`
	ts := newServer(server.Config{
		Quiet: true,
		Middlewares: []*server.Middleware{
			{
				Name: "gate",
				Lang: "erde",
				Fn:   `if rest.req:query('blocked') { return <p>blocked</p> }`,
			},
		},
		Handlers: []*server.Handler{
			{
				Method: "GET", Path: "/element", Lang: "erde", Middleware: []string{"gate"},
				Fn: page + `
					return <App name={rest.req:query('name')}/>`,
			},
			{
				Method: "GET", Path: "/body", Lang: "erde",
				Fn: page + `
					return { status = 404, body = <App name="nobody"/> }`,
			},
			{
				Method: "GET", Path: "/content-type", Lang: "erde",
				Fn: `return { headers = { ['content-type'] = 'text/plain' }, body = <p/> }`,
			},
			{
				Method: "GET", Path: "/plain", Lang: "erde",
				Fn: `return { status = 200, body = { ok = true } }`,
			},
		},
	})
	defer ts.Close()

	const html = "text/html; charset=utf-8"
	cases := []struct {
		path string
		want Response
	}{
		{"/element?name=frankie", Response{StatusCode: 200, Headers: map[string]string{"Content-Type": html}, Body: "<!DOCTYPE html><main><h1>hi frankie</h1></main>"}},
		{"/element?name=frankie&blocked=1", Response{StatusCode: 200, Headers: map[string]string{"Content-Type": html}, Body: "<!DOCTYPE html><p>blocked</p>"}},
		{"/body", Response{StatusCode: 404, Headers: map[string]string{"Content-Type": html}, Body: "<!DOCTYPE html><main><h1>hi nobody</h1></main>"}},
		{"/content-type", Response{StatusCode: 200, Headers: map[string]string{"Content-Type": "text/plain"}, Body: "<!DOCTYPE html><p></p>"}},
		{"/plain", Response{StatusCode: 200, Body: `{"ok":true}`}},
	}
	for _, c := range cases {
		req, err := http.NewRequest("GET", ts.URL+c.path, nil)
		if err != nil {
			t.Fatalf("error creating request: %s", err)
		}
		checkResponse(t, req, c.want)
	}
}

// Stateful handlers keep their lua state between requests that send back
// rest.state (in the X-Rest-State header).
func TestStatefulHandlers(t *testing.T) {
	ts := newServer(server.Config{
		Quiet: true,
		Handlers: []*server.Handler{
			{
				Method: "GET", Path: "/new", Stateful: true,
				Fn: `count = 0; return { body = rest.state }`,
			},
			{
				Method: "POST", Path: "/inc", Stateful: true,
				Fn: `count = count + 1; return { body = tostring(count) .. ' ' .. rest.req.body }`,
			},
			{
				Method: "GET", Path: "/plain",
				Fn: `return { body = tostring(count) .. ' ' .. tostring(rest.state) }`,
			},
		},
	})
	defer ts.Close()

	newState := func() string {
		res, err := http.Get(ts.URL + "/new")
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		id, _ := io.ReadAll(res.Body)
		return string(id)
	}
	inc := func(state, body string, want Response) {
		req, err := http.NewRequest("POST", ts.URL+"/inc", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set(server.StateHeader, state)
		checkResponse(t, req, want)
	}

	a, b := newState(), newState()
	if a == "" || a == b {
		t.Fatalf("expected distinct state ids, got %q and %q", a, b)
	}
	inc(a, "x", Response{StatusCode: 200, Body: "1 x"})
	inc(a, "y", Response{StatusCode: 200, Body: "2 y"}) // rest.req is per request
	inc(b, "z", Response{StatusCode: 200, Body: "1 z"})
	inc("bogus", "", Response{StatusCode: http.StatusGone, Body: `{"error":"state bogus is gone"}`})

	// other handlers get a fresh state
	req, _ := http.NewRequest("GET", ts.URL+"/plain", nil)
	req.Header.Set(server.StateHeader, a)
	checkResponse(t, req, Response{StatusCode: 200, Body: "nil nil"})
}

// Page handlers serve an interactive markup page, w/ its actions as POSTs to the
// same path.
func TestPageHandlers(t *testing.T) {
	ts := newServer(server.Config{
		Quiet: true,
		Middlewares: []*server.Middleware{
			{Name: "gate", Lang: "erde", Fn: `if rest.req:query('blocked') { return { status = 403 } }`},
		},
		Handlers: []*server.Handler{
			{
				Method: "GET", Path: "/counter/{start}", Lang: "erde", Middleware: []string{"gate"},
				Page: `
					local { state } = require('erde.markup')
					local Counter = (props) -> {
						local s = state({ count = tonumber(props.start) })
						return <button onclick={() -> { s.count += 1 }}>{s.count}</button>
					}
					return <main><Counter start={rest.req:path_value('start')}/></main>`,
			},
			{
				Method: "GET", Path: "/redirect", Lang: "erde",
				Page: `return { status = 302, headers = { Location = '/counter/1' } }`,
			},
		},
	})
	defer ts.Close()

	res, err := http.Get(ts.URL + "/counter/5")
	if err != nil {
		t.Fatal(err)
	}
	page, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || res.Header.Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatalf("page: %d %s\n%s", res.StatusCode, res.Header.Get("Content-Type"), page)
	}
	// the client posts to the page's own url (rpc: null), w/ its state
	cfg := regexp.MustCompile(`rpc: null, headers: \{ 'X-Rest-State': '([^']*)' \}`).FindStringSubmatch(string(page))
	action := regexp.MustCompile(`erde\.rpc\('([^']+)'`).FindStringSubmatch(string(page))
	if cfg == nil || action == nil || !strings.Contains(string(page), ">5</button>") {
		t.Fatalf("page\n%s", page)
	}
	state := cfg[1]

	post := func(state, query, body string, want Response) {
		req, err := http.NewRequest("POST", ts.URL+"/counter/5"+query, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		if state != "" {
			req.Header.Set(server.StateHeader, state)
		}
		checkResponse(t, req, want)
	}
	click := `{"id": "` + action[1] + `", "data": {"type": "click"}}`
	button := func(n string) string {
		id := strings.Split(action[1], "#")[0]
		return `<template data-erde-target="` + id + `"><button data-erde="` + id + `" onclick="erde.rpc('` + action[1] + `', event)">` + n + `</button></template>`
	}
	post(state, "", click, Response{StatusCode: 200, Body: button("6")})
	post(state, "", click, Response{StatusCode: 200, Body: button("7")})
	post("", "", click, Response{StatusCode: 400, Body: `{"error": "missing X-Rest-State header"}`})
	post("bogus", "", click, Response{StatusCode: 410, Body: `{"error":"state bogus is gone"}`})
	post(state, "", `nope`, Response{StatusCode: 400, Body: `{"error": "expected a json body w/ an action id"}`})
	post(state, "?blocked=1", click, Response{StatusCode: 403}) // page middleware runs for actions too

	// a page can also return a response table
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err = client.Get(ts.URL + "/redirect")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 302 || res.Header.Get("Location") != "/counter/1" {
		t.Errorf("redirect: %d %s", res.StatusCode, res.Header.Get("Location"))
	}
}

// A script that doesn't return a response is a 500, w/ a hint in the log.
func TestBadHandlerReturns(t *testing.T) {
	cases := []struct{ script, hint string }{
		{"return function() end", "render it w/ `return <App/>`"},
		{"local x = 1", "is the return missing?"},
		{"return 'hi'", "got a string"},
	}
	for _, c := range cases {
		err := func() error {
			s := server.Server{Config: server.Config{}}
			_, _, err := s.RunLuaPipeline(&server.Handler{Page: c.script}, httptest.NewRequest("GET", "/", nil), httptest.NewRecorder())
			return err
		}()
		if err == nil || !strings.Contains(err.Error(), c.hint) {
			t.Errorf("%q: got %v, want %q", c.script, err, c.hint)
		}
	}

	ts := newServer(server.Config{Handlers: []*server.Handler{{Method: "GET", Path: "/", Fn: "return function() end"}}})
	defer ts.Close()
	res, err := http.Get(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 500 {
		t.Errorf("got %d, want 500", res.StatusCode)
	}
}

func TestPageHandlerConflicts(t *testing.T) {
	for name, handlers := range map[string][]*server.Handler{
		"fn and page": {{Method: "GET", Path: "/", Page: "return 1", Fn: "return 1"}},
		"post on the same path": {
			{Method: "GET", Path: "/x", Page: "return 1"},
			{Method: "POST", Path: "/x", Fn: "return {}"},
		},
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: expected a panic", name)
				}
			}()
			newServer(server.Config{Quiet: true, Handlers: handlers}).Close()
		}()
	}
}

// Pages w/o actions are static: no client script, and no state is kept.
func TestStaticPage(t *testing.T) {
	ts := newServer(server.Config{
		Quiet: true,
		Handlers: []*server.Handler{
			{Method: "GET", Path: "/static", Lang: "erde", Page: `return <p>hi {rest.req:query('n')}</p>`},
		},
	})
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/static?n=1", nil)
	checkResponse(t, req, Response{StatusCode: 200, Body: "<!DOCTYPE html><p>hi 1</p>"})
}

// Handlers w/ cache keep their responses (see cache.go).
func TestHandlerCache(t *testing.T) {
	counter := `local n = (kv.get('n') or 0) + 1; kv.set('n', n)`
	erdeCounter := `local n = (kv.get('n') || 0) + 1; kv.set('n', n)`
	ts := newServer(server.Config{Quiet: true, Middlewares: []*server.Middleware{
		{Name: "auth", Fn: `if not rest.req:header('Authorization') then return { status = 401 } end`},
	}, Handlers: []*server.Handler{
		{Method: "GET", Path: "/fn/{id}", CacheTTL: time.Minute, Vary: []string{"Accept-Language"},
			Fn: counter + `; return { status = 200, body = rest.req:path_value('id') .. ' ' .. n }`},
		{Method: "GET", Path: "/short", CacheTTL: 50 * time.Millisecond, Fn: counter + `; return { status = 200, body = tostring(n) }`},
		{Method: "GET", Path: "/missing", CacheTTL: time.Minute, Fn: counter + `; return { status = 404, body = tostring(n) }`},
		{Method: "GET", Path: "/cookie", CacheTTL: time.Minute, Fn: counter + `; return { status = 200, cookies = { a = 'b' }, body = tostring(n) }`},
		{Method: "GET", Path: "/auth", CacheTTL: time.Minute, Middleware: []string{"auth"}, Fn: counter + `; return { status = 200, body = tostring(n) }`},
		{Method: "GET", Path: "/static", CacheTTL: time.Minute, Lang: "erde", Page: erdeCounter + "\nreturn <p>{n}</p>"},
		{Method: "GET", Path: "/interactive", CacheTTL: time.Minute, Lang: "erde", Page: erdeCounter + "\nreturn <button onclick={() -> {}}>{n}</button>"},
	}})
	defer ts.Close()

	get := func(path string, header ...string) (string, http.Header) {
		t.Helper()
		req, _ := http.NewRequest("GET", ts.URL+path, nil)
		for i := 0; i+1 < len(header); i += 2 {
			req.Header.Set(header[i], header[i+1])
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		body, _ := io.ReadAll(res.Body)
		return fmt.Sprintf("%d %s", res.StatusCode, body), res.Header
	}

	steps := []struct {
		path   string
		header []string
		want   string
	}{
		{"/fn/a", nil, "200 a 1"},
		{"/fn/a", nil, "200 a 1"},                               // cached
		{"/fn/b", nil, "200 b 2"},                               // another path
		{"/fn/a?x=1&y=2", nil, "200 a 3"},                       // another query
		{"/fn/a?y=2&x=1", nil, "200 a 3"},                       // (sorted)
		{"/fn/a", []string{"Accept-Language", "fr"}, "200 a 4"}, // a vary header
		{"/missing", nil, "404 5"},
		{"/missing", nil, "404 6"}, // not a 200
		{"/cookie", nil, "200 7"},
		{"/cookie", nil, "200 8"}, // sets cookies
		{"/auth", nil, "401 "},    // middleware runs first
		{"/auth", []string{"Authorization", "x"}, "200 9"},
		{"/auth", []string{"Authorization", "y"}, "200 9"}, // (not part of the key)
		{"/auth", nil, "401 "},                             // even for cached responses
	}
	for _, s := range steps {
		if got, _ := get(s.path, s.header...); got != s.want {
			t.Errorf("%s %v: got %q, want %q", s.path, s.header, got, s.want)
		}
	}

	if _, h := get("/fn/a"); h.Get("Cache-Control") != "public, max-age=60" {
		t.Errorf("Cache-Control: %q", h.Get("Cache-Control"))
	}

	// a static page is cached, an interactive one isn't
	static, _ := get("/static")
	if again, _ := get("/static"); again != static {
		t.Errorf("static page: %q then %q", static, again)
	}
	interactive, _ := get("/interactive")
	if again, _ := get("/interactive"); again == interactive {
		t.Errorf("interactive page was cached: %q", again)
	}

	// entries expire
	first, _ := get("/short")
	second, _ := get("/short")
	time.Sleep(80 * time.Millisecond)
	if third, _ := get("/short"); first != second || third == first {
		t.Errorf("expiry: %q %q %q", first, second, third)
	}
}
