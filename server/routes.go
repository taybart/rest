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

	if s.Config.Dir != "" {
		s.Router.HandleFunc("/", gzipHandler(s.HandleDir()))
		return
	}
	if !s.registerHandlerFns() {
		s.Router.HandleFunc("/__ws__", s.HandleWSEcho())
		s.Router.HandleFunc("/__echo__", log.Middleware(gzipHandler(s.HandleEcho())))
		s.Router.HandleFunc("/", log.Middleware(gzipHandler(s.HandleRoot())))
	}
}

func (s *Server) registerHandlerFns() bool {
	if len(s.Config.Handlers) > 0 {
		for _, handler := range s.Config.Handlers {
			log.Infof("registering handler %s%s%s\n", log.Blue, handler.Path, log.Reset)
			s.Router.HandleFunc(handler.Path, log.Middleware(s.CustomHandlerFn(handler)))
		}
		// catch all with s.Config.Response as default if specified
		s.Router.HandleFunc("/", log.Middleware(func(w http.ResponseWriter, r *http.Request) {
			s.WriteResponseWithDefault(w, Response{Status: http.StatusNotFound})
		}))
		return true
	}
	return false
}
func (s *Server) CustomHandlerFn(handler *Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if handler.Method == r.Method {
			if handler.Fn != "" {
				status, body, err := s.RunLuaHandler(handler.Fn, r, w)
				if err != nil {
					log.Error(err)
					w.WriteHeader(http.StatusBadRequest)
				} else {
					w.WriteHeader(status)
				}
				fmt.Fprint(w, body)
				return
			}
			if handler.Response != nil {
				s.WriteResponseWithDefault(w, *handler.Response)
				return
			}
		}
		s.WriteResponseWithDefault(w, Response{Status: http.StatusNotFound})
	}
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

func (s *Server) HandleDir() http.HandlerFunc {
	d, err := filepath.Abs(s.Config.Dir)
	if err != nil {
		log.Fatal(err)
	}
	fs := http.FileServer(http.Dir(d))
	return func(w http.ResponseWriter, r *http.Request) {
		if s.Config.Response != nil {
			for k, v := range s.Config.Response.Headers {
				w.Header().Add(k, v)
			}
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
		// FIXME, need to just bounce back client friendly headers
		// echo headers back
		// for k, v := range r.Header {
		// 	w.Header().Add(k, strings.Join(v, ","))
		// }

		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, string(body))
	}
}

func (s *Server) HandleRoot() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.Config.Quiet {
			dump, err := httputil.DumpRequest(r, true)
			if err != nil {
				http.Error(w, fmt.Sprint(err), http.StatusInternalServerError)
				return
			}
			fmt.Printf("%s%s%s\n", log.Yellow, string(dump), log.Rtd)
		}
		s.WriteConfigResponse(w)
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
