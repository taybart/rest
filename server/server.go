// Package server provides a simple http server for handling/dumping requests
package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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
	Body    json.RawMessage   `json:"body"`
	BodyHCL hcl.Expression    `hcl:"body,optional"`
}

type Server struct {
	Server *http.Server
	Router *http.ServeMux
	Config Config
}

type Config struct {
	Addr     string    `hcl:"address"`
	Dir      string    `hcl:"directory,optional"`
	Quiet    bool      `hcl:"quiet,optional"`
	Response *Response `hcl:"response,block"`
	Origins  []string  `hcl:"origins,optional"`
	Cors     bool      `hcl:"cors,optional"`
	TLS      string    `hcl:"tls,optional"`
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

	log.Infof("listening to %s...\n", s.Config.Addr)
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
