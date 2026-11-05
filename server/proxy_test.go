package server_test

import (
	"bufio"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/taybart/rest/server"
)

func get(t *testing.T, method, url string) (int, string, http.Header) {
	t.Helper()
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b), res.Header
}

func static(status int, body string) *server.Response {
	return &server.Response{Status: status, Body: []byte(body)}
}

func TestSamePathDifferentMethods(t *testing.T) {
	ts := newServer(server.Config{Quiet: true, Handlers: []*server.Handler{
		{Method: "GET", Path: "/item", Response: static(200, `"get"`)},
		{Method: "POST", Path: "/item", Response: static(201, `"post"`)},
		{Method: "delete", Path: "/item", Response: static(204, ``)},
	}})
	defer ts.Close()

	for _, c := range []struct {
		method string
		status int
		body   string
	}{
		{"GET", 200, `"get"`},
		{"POST", 201, `"post"`},
		{"DELETE", 204, ``},
		{"PUT", 404, ``},
	} {
		status, body, _ := get(t, c.method, ts.URL+"/item")
		if status != c.status || (c.body != "" && body != c.body) {
			t.Errorf("%s /item: expected %d %s, got %d %s", c.method, c.status, c.body, status, body)
		}
	}
}

func newBackend(t *testing.T) *httptest.Server {
	t.Helper()
	b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Seen-Host", r.Host)
		w.Header().Set("X-Seen-Forwarded-Host", r.Header.Get("X-Forwarded-Host"))
		io.WriteString(w, "backend "+r.Method+" "+r.URL.Path)
	}))
	t.Cleanup(b.Close)
	return b
}

func TestServerProxyFallsBackAfterHandlers(t *testing.T) {
	backend := newBackend(t)
	ts := newServer(server.Config{Quiet: true, Proxy: &server.Proxy{Target: backend.URL}, Handlers: []*server.Handler{
		{Method: "GET", Path: "/mocked", Response: static(200, `"mock"`)},
	}})
	defer ts.Close()

	if status, body, _ := get(t, "GET", ts.URL+"/mocked"); status != 200 || body != `"mock"` {
		t.Errorf("mocked route: got %d %s", status, body)
	}
	if _, body, _ := get(t, "POST", ts.URL+"/mocked"); body != "backend POST /mocked" {
		t.Errorf("unmatched method should proxy, got %s", body)
	}
	if _, body, _ := get(t, "GET", ts.URL+"/other"); body != "backend GET /other" {
		t.Errorf("unmatched path should proxy, got %s", body)
	}
}

func TestProxyRewritesHost(t *testing.T) {
	backend := newBackend(t)
	ts := newServer(server.Config{Quiet: true, Proxy: &server.Proxy{Target: backend.URL}})
	defer ts.Close()

	_, _, h := get(t, "GET", ts.URL+"/x")
	backendHost := strings.TrimPrefix(backend.URL, "http://")
	frontHost := strings.TrimPrefix(ts.URL, "http://")
	if got := h.Get("X-Seen-Host"); got != backendHost {
		t.Errorf("expected Host %s, got %s", backendHost, got)
	}
	if got := h.Get("X-Seen-Forwarded-Host"); got != frontHost {
		t.Errorf("expected X-Forwarded-Host %s, got %s", frontHost, got)
	}
}

// a handler-level proxy (with middleware) must stream, and must outlive the
// server's write timeout
func TestHandlerProxyStreamsPastWriteTimeout(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for i := range 3 {
			io.WriteString(w, "data: "+string(rune('a'+i))+"\n\n")
			w.(http.Flusher).Flush()
			time.Sleep(150 * time.Millisecond)
		}
	}))
	defer backend.Close()

	s := server.Server{Router: http.NewServeMux(), Config: server.Config{
		Quiet:       true,
		Middlewares: []*server.Middleware{{Name: "noop", Fn: "-- pass"}},
		Handlers: []*server.Handler{
			{Method: "GET", Path: "/events", Middleware: []string{"noop"}, Proxy: &server.Proxy{Target: backend.URL}},
		},
	}}
	ts := httptest.NewUnstartedServer(nil)
	s.Routes(ts.Config)
	ts.Config.Handler = s.Router
	ts.Config.WriteTimeout = 200 * time.Millisecond
	ts.Start()
	defer ts.Close()

	start := time.Now()
	res, err := http.Get(ts.URL + "/events")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	r := bufio.NewReader(res.Body)
	first, err := r.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if first != "data: a\n" || time.Since(start) > 120*time.Millisecond {
		t.Fatalf("first event not streamed promptly: %q after %s", first, time.Since(start))
	}
	rest, _ := io.ReadAll(r)
	if !strings.Contains(string(rest), "data: c") {
		t.Fatalf("stream cut off before last event (write timeout?): %q", rest)
	}
}

func TestHandlerProxyWebsocket(t *testing.T) {
	upgrader := websocket.Upgrader{}
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		mt, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		conn.WriteMessage(mt, append([]byte("echo "), msg...))
	}))
	defer backend.Close()

	ts := newServer(server.Config{Quiet: true, Handlers: []*server.Handler{
		{Method: "GET", Path: "/ws", Proxy: &server.Proxy{Target: backend.URL}},
	}})
	defer ts.Close()

	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(ts.URL, "http")+"/ws", nil)
	if err != nil {
		t.Fatalf("dial through proxy: %v", err)
	}
	defer conn.Close()
	if err := conn.WriteMessage(websocket.TextMessage, []byte("hi")); err != nil {
		t.Fatal(err)
	}
	_, msg, err := conn.ReadMessage()
	if err != nil || string(msg) != "echo hi" {
		t.Fatalf("expected echo hi, got %q (%v)", msg, err)
	}
}

func TestProxyStripAndHeaders(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, strings.Join([]string{
			"path=" + r.URL.EscapedPath(),
			"host=" + r.Host,
			"auth=" + r.Header.Get("Authorization"),
			"cookie=" + r.Header.Get("Cookie"),
			"keep=" + r.Header.Get("X-Keep"),
		}, " "))
	}))
	defer backend.Close()

	ts := newServer(server.Config{Quiet: true, Handlers: []*server.Handler{
		{Method: "*", Path: "/api/", Proxy: &server.Proxy{
			Target: backend.URL + "/v2",
			Strip:  "/api/", // trailing slash is ignored
			Headers: map[string]string{
				"Authorization": "Bearer secret",
				"Cookie":        "",
				"Host":          "api.internal",
			},
		}},
		{Method: "*", Path: "/apiary/", Proxy: &server.Proxy{Target: backend.URL, Strip: "/api"}},
	}})
	defer ts.Close()

	do := func(path string) string {
		req, _ := http.NewRequest("GET", ts.URL+path, nil)
		req.Header.Set("Authorization", "Bearer client")
		req.Header.Set("Cookie", "session=1")
		req.Header.Set("X-Keep", "yes")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return string(b)
	}

	got := do("/api/users/a%2Fb")
	for _, want := range []string{"path=/v2/users/a%2Fb", "host=api.internal", "auth=Bearer secret", "cookie= ", "keep=yes"} {
		if !strings.Contains(got+" ", want) {
			t.Errorf("expected %q in %q", want, got)
		}
	}
	if got := do("/api/"); !strings.Contains(got, "path=/v2/ ") {
		t.Errorf("stripping to root: got %q", got)
	}
	if got := do("/apiary/x"); !strings.Contains(got, "path=/apiary/x ") {
		t.Errorf("strip must respect segment boundaries: got %q", got)
	}
}

func TestProxyInsecure(t *testing.T) {
	backend := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "tls backend")
	}))
	defer backend.Close()

	ts := newServer(server.Config{Quiet: true, Handlers: []*server.Handler{
		{Method: "GET", Path: "/verified", Proxy: &server.Proxy{Target: backend.URL}},
		{Method: "GET", Path: "/insecure", Proxy: &server.Proxy{Target: backend.URL, Insecure: true}},
	}})
	defer ts.Close()

	// self-signed cert: verification fails by default -> 502 from the proxy
	if status, _, _ := get(t, "GET", ts.URL+"/verified"); status != http.StatusBadGateway {
		t.Errorf("expected 502 for unverified self-signed backend, got %d", status)
	}
	if status, body, _ := get(t, "GET", ts.URL+"/insecure"); status != 200 || body != "tls backend" {
		t.Errorf("insecure: got %d %q", status, body)
	}
}
