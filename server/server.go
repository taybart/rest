// Package server provides a simple http server for handling/dumping requests
package server

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/hashicorp/hcl/v2"
	"github.com/rs/cors"
	"github.com/taybart/log"
	restlua "github.com/taybart/rest/lua"
)

const (
	httpTimeout = 15 * time.Second
)

type Response struct {
	Status int `json:"status" hcl:"status"`
	// Path    string          `json:"path" hcl:"path,optional"`
	// Method  string          `json:"method" hcl:"method,optional"`
	Headers map[string]string `json:"headers" hcl:"headers,optional"`
	Cookies []*http.Cookie    `json:"-"`
	Body    json.RawMessage   `json:"body"`
	BodyHCL hcl.Expression    `hcl:"body,optional"`
}
type Middleware struct {
	Name string `json:"name" hcl:"name,label"`
	Fn   string `json:"fn" hcl:"fn"`
	Lang string `json:"-"`
}

type Handler struct {
	Method     string   `json:"method" hcl:"method,label"`
	Path       string   `json:"path" hcl:"path,label"`
	Middleware []string `json:"middleware" hcl:"middleware,optional"`
	Fn         string   `json:"fn" hcl:"fn,optional"`
	// proxy = "target" lands in ProxyHCL and is moved into Proxy by the
	// parser; a proxy { } block decodes straight into Proxy
	ProxyHCL hcl.Expression `json:"-" hcl:"proxy,optional"`
	Proxy    *Proxy         `json:"proxy,omitempty" hcl:"proxy,block"`
	WS       bool           `json:"ws" hcl:"ws,optional"`
	// Stateful keeps the lua state between requests (see state.go).
	Stateful bool `json:"stateful" hcl:"stateful,optional"`
	// Page is like Fn, but returns a markup element that's served as an
	// interactive page (see page.go).
	Page string `json:"page,omitempty" hcl:"page,optional"`
	// Cache keeps the handler's responses for this long (ex. "1m"), see
	// cache.go. Vary lists request headers that are part of the cache key
	// (besides the path and query).
	Cache    string        `json:"cache,omitempty" hcl:"cache,optional"`
	Vary     []string      `json:"vary,omitempty" hcl:"vary,optional"`
	CacheTTL time.Duration `json:"-"`
	cache    *responseCache
	// set on the POST handler rest adds for a page's actions
	pageActions bool
	Response    *Response `hcl:"response,block"`
	Expect      *Expect   `json:"expect,omitempty" hcl:"expect,block"`
	Lang        string    `json:"-"`
}

// Expect declares what a handler accepts. Requests that don't match get a
// 400 with {"error": "<path>: <reason>"} before the handler's fn, proxy or
// response runs (but after its middleware).
type Expect struct {
	Headers map[string]string `json:"headers,omitempty" hcl:"headers,optional"`
	Query   map[string]string `json:"query,omitempty" hcl:"query,optional"`
	Path    map[string]string `json:"path,omitempty" hcl:"path,optional"`
	// Additional allows body fields (at any depth) that aren't listed in body.
	Additional bool           `json:"additional,omitempty" hcl:"additional,optional"`
	Body       string         `json:"body,omitempty"`
	BodyHCL    hcl.Expression `json:"-" hcl:"body,optional"`
}

type Server struct {
	Server *http.Server
	Router *http.ServeMux
	Config Config
	kvMu   sync.RWMutex
	kvOnce sync.Once
	kv     map[string]any
	// rest.exports set by setup (read only after kvOnce)
	setupExports map[string]any
	// lua states of stateful handlers (set up by Routes)
	states *stateStore
}

type Config struct {
	Addr     string         `hcl:"address"`
	Dir      string         `hcl:"directory,optional"`
	ProxyHCL hcl.Expression `json:"-" hcl:"proxy,optional"`
	Proxy    *Proxy         `json:"proxy,omitempty" hcl:"proxy,block"`
	Quiet    bool           `hcl:"quiet,optional"`
	// limits for stateful handlers (see state.go): the max number of states
	// (default 1000) and how long unused ones are kept (ex. "10m", default
	// 30m)
	MaxStates int       `json:"max_states,omitempty" hcl:"max_states,optional"`
	StateTTL  string    `json:"state_ttl,omitempty" hcl:"state_ttl,optional"`
	Response  *Response `hcl:"response,block"`
	Origins   []string  `hcl:"origins,optional"`
	Cors      bool      `hcl:"cors,optional"`
	TLS       string    `hcl:"tls,optional"`
	// serve https with this machine's tailscale cert (needs the tailscale cli)
	Tailscale   bool           `hcl:"tailscale,optional"`
	Setup       string         `hcl:"setup,optional"`
	KV          map[string]any `json:"-"`
	KVHCL       hcl.Expression `hcl:"kv,optional"`
	Middlewares []*Middleware  `hcl:"middleware,block"`
	Handlers    []*Handler     `hcl:"handler,block"`
	SPA         bool           `hcl:"spa,optional"`
	// set by the parser
	SetupLang string
	// runs the file's request blocks (rest.request in lua)
	Requester restlua.Requester `json:"-"`
}

func New(c Config) Server {

	s := Server{
		Router: http.NewServeMux(),
		Config: c,
	}

	server := &http.Server{
		Addr:         c.Addr,
		WriteTimeout: httpTimeout,
		ReadTimeout:  httpTimeout,
	}
	// weird thing: pass server in for shutdown route
	s.Routes(server)
	server.Handler = s.Router
	if c.Cors {
		server.Handler = cors.AllowAll().Handler(s.Router)
	}
	s.Server = server
	return s
}
func (s *Server) Serve() error {

	if s.Config.Tailscale {
		return s.serveTailscale()
	}
	if !s.Config.Quiet {
		log.Infof("listening to %s...\n", s.Config.Addr)
	}
	if s.Config.TLS != "" {
		crt := fmt.Sprintf("%s.crt", s.Config.TLS)
		key := fmt.Sprintf("%s.key", s.Config.TLS)
		if err := s.Server.ListenAndServeTLS(crt, key); err != nil {
			if !errors.Is(err, http.ErrServerClosed) {
				return err
			}
		}
		return nil
	}
	if err := s.Server.ListenAndServe(); err != nil {
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}
	return nil
}

func (s *Server) serveTailscale() error {
	if s.Config.TLS != "" {
		return errors.New("tls and tailscale can't be used together")
	}
	ts, err := newTailscaleCert()
	if err != nil {
		return err
	}
	host, port, err := net.SplitHostPort(s.Config.Addr)
	if err != nil {
		return err
	}
	if ip := net.ParseIP(host); host == "localhost" || (ip != nil && ip.IsLoopback()) {
		log.Warnf("listening on %s, other tailnet devices can't reach it (use -a :%s)\n", host, port)
	}
	if !s.Config.Quiet {
		log.Infof("listening to https://%s:%s...\n", ts.Domain, port)
	}
	s.Server.TLSConfig = &tls.Config{GetCertificate: ts.GetCertificate}
	if err := s.Server.ListenAndServeTLS("", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *Server) MustParseURL(in string) *url.URL {
	u, err := url.Parse(in)
	if err != nil || u.Host == "" {
		u, err = url.Parse("http://" + in)
		if err != nil {
			log.Fatalf("could not proxy url: %s, error: %v", in, err)
		}
	}
	return u
}

func (s *Server) WriteConfigResponse(w http.ResponseWriter) error {

	if s.Config.Response != nil {
		for k, v := range s.Config.Response.Headers {
			w.Header().Add(k, v)
		}
	}

	status := http.StatusOK
	body := `{"status": "ok"}`
	// overrides
	if res := s.Config.Response; res != nil {
		if res.Status != 0 {
			status = res.Status
		}
		if len(res.Body) != 0 {
			body = string(res.Body)
		}
	}
	w.WriteHeader(status)
	fmt.Fprint(w, body)
	return nil
}
func (s *Server) WriteResponseWithDefault(w http.ResponseWriter, res Response) error {

	if s.Config.Response != nil {
		for k, v := range s.Config.Response.Headers {
			w.Header().Add(k, v)
		}
	} else {
		for k, v := range res.Headers {
			w.Header().Add(k, v)
		}
	}

	status := res.Status
	body := res.Body
	if cres := s.Config.Response; cres != nil {
		if cres.Status != 0 {
			status = cres.Status
		}
		if len(cres.Body) != 0 {
			body = cres.Body
		}
	}
	w.WriteHeader(status)
	fmt.Fprint(w, string(body))
	return nil
}
