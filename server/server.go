// Package server provides a simple http server for handling/dumping requests
package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/hashicorp/hcl/v2"
	"github.com/rs/cors"
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
	Router *http.ServeMux
	C      Config
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

func New(c Config) *http.Server {

	s := Server{
		Router: http.NewServeMux(),
		C:      c,
	}

	server := &http.Server{
		Addr:         c.Addr,
		WriteTimeout: httpTimeout,
		ReadTimeout:  httpTimeout,
	}
	// weird thing for shutdown route
	s.Routes(server)
	server.Handler = s.Router
	if c.Cors {
		server.Handler = cors.AllowAll().Handler(s.Router)
	}
	return server
}
