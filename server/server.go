// Package server provides a simple http server for handling/dumping requests
package server

import (
	"net/http"
	"slices"
	"time"

	"github.com/hashicorp/hcl/v2"
)

const (
	httpTimeout = 15 * time.Second
)

type Response struct {
	Status  int            `json:"status" hcl:"status"`
	Body    string         `json:"body"`
	BodyHCL hcl.Expression `hcl:"body,optional"`
}

type Server struct {
	router *http.ServeMux
	c      Config
}

type Config struct {
	Addr     string            `hcl:"address"`
	Dir      string            `hcl:"directory,optional"`
	Quiet    bool              `hcl:"quiet,optional"`
	Response *Response         `hcl:"response,block"`
	Headers  map[string]string `hcl:"headers,optional"`
	Origins  []string          `hcl:"origins,optional"`
	TLS      string            `hcl:"tls,optional"`
}

func New(c Config) *http.Server {

	s := Server{
		router: http.NewServeMux(),
		c:      c,
	}

	server := &http.Server{
		Addr:         c.Addr,
		WriteTimeout: httpTimeout,
		ReadTimeout:  httpTimeout,
	}
	// weird thing for shutdown route
	s.routes(server)
	server.Handler = s.router
	return server
}

// FIXME: this doesn't do the cors stuff
func (s *Server) cors(w http.ResponseWriter, r *http.Request) {
	if len(s.c.Origins) == 0 {
		return
	}

	if slices.Contains(s.c.Origins, "*") {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		return
	}
	origin := r.Header.Get("Origin")
	if slices.Contains(s.c.Origins, origin) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	}

	w.Header().Set(
		"Access-Control-Allow-Methods",
		"GET, POST, PUT, DELETE, OPTIONS",
	)
}
