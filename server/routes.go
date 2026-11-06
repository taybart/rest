package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/taybart/log"
)

func (s *Server) Routes(server *http.Server) {
	if err := s.initKV(); err != nil {
		log.Error(err)
	}
	// w/ methods, so it's more specific than any handler pattern (a method-less
	// "/__quit__" conflicts w/ ex. "GET /" in ServeMux, since neither is more
	// specific than the other)
	var ttl time.Duration
	if s.Config.StateTTL != "" {
		var err error
		if ttl, err = time.ParseDuration(s.Config.StateTTL); err != nil {
			panic(fmt.Errorf("invalid state_ttl %q: %w", s.Config.StateTTL, err))
		}
	}
	s.states = newStateStore(s.Config.MaxStates, ttl)
	quit := s.HandleQuit(server)
	s.Router.HandleFunc("GET /__quit__", quit)
	s.Router.HandleFunc("POST /__quit__", quit)

	// a directory (or else a server-level proxy) serves anything the handlers
	// don't match
	var fallback http.HandlerFunc
	if s.Config.Dir != "" {
		fallback = gzipHandler(s.HandleDir())
	} else if s.Config.Proxy != nil {
		fallback = log.Middleware(s.ProxyHandler(s.Config.Proxy))
	}
	if len(s.Config.Handlers) > 0 {
		s.registerHandlerFns(fallback)
		return
	}
	if fallback != nil {
		s.Router.HandleFunc("/", fallback)
		return
	}
	s.Router.HandleFunc("/__ws__", s.HandleWSEcho("*"))
	s.Router.HandleFunc("/__echo__", log.Middleware(gzipHandler(s.HandleEcho())))
	s.Router.HandleFunc("/", log.Middleware(gzipHandler(s.HandleRoot())))
}

// handlerPattern builds a ServeMux pattern. Including the method lets
// handlers share a path ("GET /x" and "POST /x"); "*" matches any method.
func handlerPattern(h *Handler) string {
	if h.Method == "*" || h.Method == "" {
		return h.Path
	}
	return strings.ToUpper(h.Method) + " " + h.Path
}

func (s *Server) registerHandlerFns(fallback http.HandlerFunc) {
	if err := checkPages(s.Config.Handlers); err != nil {
		panic(err) // like ServeMux's panics for conflicting patterns
	}
	rootRegistered := false
	for _, handler := range s.Config.Handlers {
		if handler.CacheTTL > 0 {
			handler.cache = newResponseCache(handler.CacheTTL)
		}
		pattern := handlerPattern(handler)
		if pattern == "/" {
			rootRegistered = true
		}
		log.Infof("registering handler %s%s%s\n", log.Blue, pattern, log.Reset)
		switch {
		case handler.WS:
			s.Router.HandleFunc(pattern, s.HandleWSEcho("*"))
		default:
			s.Router.HandleFunc(pattern, log.Middleware(s.CustomHandlerFn(handler)))
		}
		if handler.Page != "" {
			actions := pageActionsHandler(handler)
			log.Infof("registering page actions %s%s%s\n", log.Blue, handlerPattern(actions), log.Reset)
			s.Router.HandleFunc(handlerPattern(actions), log.Middleware(s.CustomHandlerFn(actions)))
		}
	}
	if rootRegistered {
		return
	}
	if fallback != nil {
		s.Router.HandleFunc("/", fallback)
		return
	}
	// catch all with s.Config.Response as default if specified
	s.Router.HandleFunc("/", log.Middleware(func(w http.ResponseWriter, r *http.Request) {
		s.WriteResponseWithDefault(w, Response{Status: http.StatusNotFound})
	}))
}

// CustomHandlerFn serves a handler block. Method matching is done by the
// router pattern (see handlerPattern).
func (s *Server) CustomHandlerFn(handler *Handler) http.HandlerFunc {
	var proxy http.HandlerFunc
	if handler.Proxy != nil {
		proxy = s.ProxyHandler(handler.Proxy)
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if len(handler.Middleware) > 0 || handler.Fn != "" || handler.Page != "" {
			// validation runs inside the pipeline, after middleware
			res, handled, err := s.RunLuaPipeline(handler, r, w)
			if err != nil {
				// a script error is the server's (the error is only logged, it
				// can have script source in it)
				log.Error(err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			if handled {
				writeHandlerResponse(w, res)
				return
			}
		} else if err := ValidateRequest(handler.Expect, r); err != nil {
			log.Warnf("%s %s: %v\n", r.Method, r.URL.Path, err)
			writeHandlerResponse(w, validationResponse(err))
			return
		}
		if proxy != nil {
			proxy(w, r)
			return
		}
		if handler.Response != nil {
			s.WriteResponseWithDefault(w, *handler.Response)
			return
		}
		s.WriteResponseWithDefault(w, Response{Status: http.StatusNotFound})
	}
}

func writeHandlerResponse(w http.ResponseWriter, res Response) {
	for k, v := range res.Headers {
		w.Header().Add(k, v)
	}
	for _, c := range res.Cookies {
		http.SetCookie(w, c)
	}
	w.WriteHeader(res.Status)
	fmt.Fprint(w, string(res.Body))
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
		// file downloads can take arbitrarily long, drop the server-wide write deadline
		_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
		if s.Config.Response != nil {
			for k, v := range s.Config.Response.Headers {
				w.Header().Add(k, v)
			}
		}
		if s.Config.SPA {
			if _, err := os.Stat(fmt.Sprintf("%s%s", d, r.URL)); os.IsNotExist(err) {
				http.ServeFile(w, r, fmt.Sprintf("%s/index.html", d))
				return
			}
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
		// echo x- headers back
		for k, v := range r.Header {
			if strings.HasPrefix(k, "x-") || strings.HasPrefix(k, "X-") {
				w.Header().Add(k, strings.Join(v, ","))
			}
		}

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

func (s *Server) HandleWSEcho(method string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if method == r.Method || method == "*" {

			log.Infof("%s %s\n",
				r.Method,
				r.URL.Path,
			)
			upgrader := websocket.Upgrader{
				ReadBufferSize:  1024,
				WriteBufferSize: 1024,
			}
			if s.Config.Cors {
				upgrader.CheckOrigin = func(r *http.Request) bool { return true }
			}
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				log.Error(err)
				return
			}
			for {
				messageType, p, err := conn.ReadMessage()
				if err != nil {
					if !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
						log.Error(err)
					}
					return
				}
				log.Infof("[ws] echo %s\n", p)
				if err := conn.WriteMessage(messageType, p); err != nil {
					log.Error(err)
					return
				}
			}
		}
	}
}
