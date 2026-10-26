package server_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

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

