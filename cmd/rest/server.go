package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httputil"
	"os"
	"time"

	"github.com/taybart/log"
)

type server struct {
	router *http.ServeMux
}

func newServer(dir bool, addr string) http.Server {

	s := server{
		router: http.NewServeMux(),
	}
	s.routes(dir)

	return http.Server{
		Handler:      s.router,
		Addr:         addr,
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}
}

func (s *server) routes(dir bool) {
	if dir {
		s.router.Handle("/", http.FileServer(http.Dir(".")))
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

		json.NewEncoder(w).Encode(res)
	}))
}

func serve(dir, local bool, port string) {
	log.SetLevel(log.DEBUG)
	var location string
	if local {
		location = "localhost"
	}
	location = fmt.Sprintf("%s:%s", location, port)
	srv := newServer(dir, location)
	if dir {
		d, err := os.Getwd()
		if err != nil {
			log.Warn(err)
		}
		log.Infof("Serving %s at %s\n", d, location)
	} else {
		log.Infof("Running at %s\n", location)
	}
	log.Fatal(srv.ListenAndServe())
}
