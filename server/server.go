package server

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"os"
	"path/filepath"
	"time"

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
	Addr string
	Dir  string
	Dump bool
}

func New(c Config) http.Server {

	s := Server{
		router: http.NewServeMux(),
		c:      c,
	}
	s.routes()

	return http.Server{
		Handler:      s.router,
		Addr:         c.Addr,
		WriteTimeout: httpTimeout,
		ReadTimeout:  httpTimeout,
	}
}

func (s *Server) routes() {
	if s.c.Dir != "" {
		d, err := filepath.Abs(s.c.Dir)
		if err != nil {
			log.Fatal(err)
		}
		fs := http.FileServer(http.Dir(d))
		s.router.HandleFunc("/", gzipHandler(func(w http.ResponseWriter, r *http.Request) {
			if _, err := os.Stat(fmt.Sprintf("%s%s", d, r.URL)); os.IsNotExist(err) {
				http.ServeFile(w, r, fmt.Sprintf("%s/index.html", d))
				return
			}
			fs.ServeHTTP(w, r)
		}))
		return
	}

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
