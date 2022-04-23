package server

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/taybart/log"
)

const (
	httpTimeout = 15 * time.Second
)

type gzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
}

func (w gzipResponseWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}
func gzipHandler(fn http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			fn(w, r)
			return
		}
		w.Header().Set("Content-Encoding", "gzip")
		gz := gzip.NewWriter(w)
		defer gz.Close()
		gzr := gzipResponseWriter{Writer: gz, ResponseWriter: w}
		fn(gzr, r)
	}
}

type Server struct {
	router *http.ServeMux
	c      Config
}

type Config struct {
	Addr string
	Dir  string
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
		dump, err := httputil.DumpRequest(r, true)
		if err != nil {
			http.Error(w, fmt.Sprint(err), http.StatusInternalServerError)
			return
		}
		fmt.Println(log.Yellow)
		fmt.Println(string(dump))
		fmt.Println(log.Rtd)

		res := struct {
			Status string `json:"status"`
		}{
			Status: "ok",
		}
		w.WriteHeader(http.StatusOK)

		err = json.NewEncoder(w).Encode(res)
		if err != nil {
			panic(err)
		}
	})))
}
