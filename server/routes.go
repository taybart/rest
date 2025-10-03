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

func (s *Server) Routes(server *http.Server) {
	s.Router.HandleFunc("/__quit__", s.HandleQuit(server))

	if s.C.Dir != "" {
		s.Router.HandleFunc("/", gzipHandler(s.HandleDir()))
		return
	}

	s.Router.HandleFunc("/__ws__", s.HandleWSEcho())
	s.Router.HandleFunc("/__echo__", log.Middleware(gzipHandler(s.HandleEcho())))
	s.Router.HandleFunc("/", log.Middleware(gzipHandler(s.HandleRoot())))
}

func (s *Server) HandleQuit(server *http.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		log.Warn("got signal on __quit__, stopping...")
		w.WriteHeader(http.StatusOK)

		go func() {
			time.Sleep(500 * time.Millisecond)
			server.Shutdown(context.Background())
		}()
	}
}

func (s *Server) HandleWSEcho() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
	}
}

func (s *Server) HandleDir() http.HandlerFunc {
	d, err := filepath.Abs(s.C.Dir)
	if err != nil {
		log.Fatal(err)
	}
	fs := http.FileServer(http.Dir(d))
	return func(w http.ResponseWriter, r *http.Request) {
		s.cors(w, r)
		for k, v := range s.C.Headers {
			w.Header().Add(k, v)
		}
		if _, err := os.Stat(fmt.Sprintf("%s%s", d, r.URL)); os.IsNotExist(err) {
			http.ServeFile(w, r, fmt.Sprintf("%s/index.html", d))
			return
		}
		fs.ServeHTTP(w, r)
	}
}

func (s *Server) HandleEcho() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, string(body))
	}
}

func (s *Server) HandleRoot() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.C.Quiet {
			dump, err := httputil.DumpRequest(r, true)
			if err != nil {
				http.Error(w, fmt.Sprint(err), http.StatusInternalServerError)
				return
			}
			fmt.Printf("%s%s%s\n", log.Yellow, string(dump), log.Rtd)
		}

		s.cors(w, r)
		for k, v := range s.C.Headers {
			w.Header().Add(k, v)
		}

		// default
		status := http.StatusOK
		body := `{"status": "ok"}`
		// overrides
		if res := s.C.Response; res != nil {
			if res.Status != 0 {
				status = res.Status
			}
			if len(res.Body) != 0 {
				body = string(res.Body)
			}
		}
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}
}
