package main

import (
	"encoding/json"
	"net/http"
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

			if r.Header.Get("Content-Type") == "application/json" {
				body := struct {
					Data string `json:"data"`
				}{}
				err := json.NewDecoder(r.Body).Decode(&body)
				log.Info(body, err)
			}
			w.WriteHeader(http.StatusOK)

			res := struct {
				Status string `json:"status"`
			}{
				Status: "ok",
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
