// Package server provides a simple http server for handling/dumping requests
package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"os"
	"path/filepath"
	"time"

	"github.com/gorilla/websocket"
	"github.com/taybart/log"
)

const (
	httpTimeout = 15 * time.Second
)

type Server struct {
	router *http.ServeMux
	c      Config
}

type Config struct {
	Addr    string
	Dir     string
	Dump    bool
	Headers map[string]string
}

func New(c Config) *http.Server {

	s := Server{
		router: http.NewServeMux(),
		c:      c,
	}
	goserver := &http.Server{
		Addr:         c.Addr,
		WriteTimeout: httpTimeout,
		ReadTimeout:  httpTimeout,
	}
	// weird thing for shutdown route
	s.routes(goserver)
	goserver.Handler = s.router
	return goserver
}

func (s *Server) routes(server *http.Server) {
	s.router.HandleFunc("/__quit__", gzipHandler(func(w http.ResponseWriter, _ *http.Request) {
		log.Warn("got signal on __quit__, stopping...")
		w.WriteHeader(http.StatusOK)

		go func() {
			time.Sleep(500 * time.Millisecond)
			server.Shutdown(context.Background())
		}()
	}))
	s.router.HandleFunc("/__ws__", func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Error(err)
			return
		}
		for {
			messageType, p, err := conn.ReadMessage()
			if err != nil {
				log.Error(err)
				return
			}
			if err := conn.WriteMessage(messageType, p); err != nil {
				log.Error(err)
				return
			}
		}
	})
	if s.c.Dir != "" {
		d, err := filepath.Abs(s.c.Dir)
		if err != nil {
			log.Fatal(err)
		}
		fs := http.FileServer(http.Dir(d))
		s.router.HandleFunc("/", gzipHandler(func(w http.ResponseWriter, r *http.Request) {
			for k, v := range s.c.Headers {
				w.Header().Add(k, v)
			}
			if _, err := os.Stat(fmt.Sprintf("%s%s", d, r.URL)); os.IsNotExist(err) {
				http.ServeFile(w, r, fmt.Sprintf("%s/index.html", d))
				return
			}
			fs.ServeHTTP(w, r)
		}))
		return
	}

	s.router.HandleFunc("/__echo__", log.Middleware(gzipHandler(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, string(body))
	})))
	s.router.HandleFunc("/", log.Middleware(gzipHandler(func(w http.ResponseWriter, r *http.Request) {
		if !s.c.Dump {
			dump, err := httputil.DumpRequest(r, true)
			if err != nil {
				http.Error(w, fmt.Sprint(err), http.StatusInternalServerError)
				return
			}
			fmt.Printf("%s%s%s\n", log.Yellow, string(dump), log.Rtd)
		}

		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status": "ok"}`)
	})))
}
