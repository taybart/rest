package main

import (
	"encoding/json"
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

type server struct {
	router *http.ServeMux
	local  bool
	dir    bool
}

func newServer(addr string) http.Server {

	s := server{
		router: http.NewServeMux(),
	}
	s.routes()

	return http.Server{
		Handler:      s.router,
		Addr:         addr,
		WriteTimeout: httpTimeout,
		ReadTimeout:  httpTimeout,
	}
}

func (s *server) routes() {
	if servedir != "" {

		d, err := filepath.Abs(servedir)
		if err != nil {
			log.Fatal(err)
		}
		fs := http.FileServer(http.Dir(d))
		if servespa {
			s.router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				if _, err := os.Stat(fmt.Sprintf("%s%s", d, r.URL)); os.IsNotExist(err) {
					http.ServeFile(w, r, fmt.Sprintf("%s/index.html", d))
					return
				}
				fs.ServeHTTP(w, r)
			})
		} else {
			s.router.Handle("/", fs)
		}
		return
	}

	s.router.HandleFunc("/", log.Middleware(func(w http.ResponseWriter, r *http.Request) {
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
	}))
}

func serve(port string) {
	log.SetLevel(log.DEBUG)
	location := fmt.Sprintf(":%s", port)
	if local {
		location = fmt.Sprintf("localhost:%s", port)
	}
	srv := newServer(location)
	if servedir != "" {
		d, err := filepath.Abs(servedir)
		if err != nil {
			log.Fatal(err)
		}
		log.Infof("Serving %s at %s\n", d, location)
	} else {
		log.Infof("Running at %s\n", location)
	}
	log.Fatal(srv.ListenAndServe())
}
