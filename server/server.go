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
}

type Handler struct {
	Method     string    `json:"method" hcl:"method,label"`
	Path       string    `json:"path" hcl:"path,label"`
	Middleware []string  `json:"middleware" hcl:"middleware,optional"`
	Fn         string    `json:"fn" hcl:"fn,optional"`
	Proxy      string    `json:"proxy" hcl:"proxy,optional"`
	WS         bool      `json:"ws" hcl:"ws,optional"`
	Response   *Response `hcl:"response,block"`
}

type Server struct {
	Server *http.Server
	Router *http.ServeMux
	Config Config
	kvMu   sync.RWMutex
	kvOnce sync.Once
	kv     map[string]any
}

type Config struct {
	Addr     string    `hcl:"address"`
	Dir      string    `hcl:"directory,optional"`
	Proxy    string    `hcl:"proxy,optional"`
	Quiet    bool      `hcl:"quiet,optional"`
	Response *Response `hcl:"response,block"`
	Origins  []string  `hcl:"origins,optional"`
	Cors     bool      `hcl:"cors,optional"`
	TLS      string    `hcl:"tls,optional"`
	// serve https with this machine's tailscale cert (needs the tailscale cli)
	Tailscale   bool           `hcl:"tailscale,optional"`
	Setup       string         `hcl:"setup,optional"`
	KV          map[string]any `json:"-"`
	KVHCL       hcl.Expression `hcl:"kv,optional"`
	Middlewares []*Middleware  `hcl:"middleware,block"`
	Handlers    []*Handler     `hcl:"handler,block"`
	SPA         bool           `hcl:"spa,optional"`
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
			log.Fatal("could not proxy url: %s, error: %v", in, err)
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
