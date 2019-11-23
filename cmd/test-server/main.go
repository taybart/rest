package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httputil"
	"time"

	"github.com/taybart/log"
)

type server struct {
	router *http.ServeMux
}

func newServer() http.Server {

	s := server{
		router: http.NewServeMux(),
	}
	s.routes()

	return http.Server{
		Handler:      s.router,
		Addr:         "localhost:8080",
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}
}

func (s *server) routes() {
	s.router.HandleFunc("/", log.Middleware(
		func(w http.ResponseWriter, r *http.Request) {

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
			if r.URL.Path == "/fail" {
				w.WriteHeader(http.StatusInternalServerError)
				res.Status = "bad"
			} else {
				w.WriteHeader(http.StatusOK)
			}

			json.NewEncoder(w).Encode(res)
		}))
}

func main() {
	log.SetLevel(log.DEBUG)
	srv := newServer()
	log.Info("Started server")
	log.Fatal(srv.ListenAndServe())
}
