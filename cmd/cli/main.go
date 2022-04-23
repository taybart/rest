package main

import (
	"fmt"
	"os"

	"github.com/taybart/args"
	"github.com/taybart/log"
	"github.com/taybart/rest/request"
	"github.com/taybart/rest/server"
)

var (
	a = args.App{
		Name:    "Rest",
		Version: "v0.0.1",
		Author:  "Taybart <taybart@email.com>",
		About:   "all the rest",
		Args: map[string]*args.Arg{
			"addr": {
				Short:   "a",
				Help:    "Address to listen on",
				Default: "localhost:8080",
			},
			"serve": {
				Short:   "s",
				Help:    "Run a server",
				Default: false,
			},
			"dir": {
				Short:   "d",
				Help:    "Directory to serve",
				Default: "",
			},
			"file": {
				Short:   "f",
				Help:    "File to run",
				Default: "",
			},
		},
	}

	c = struct {
		Addr  string `arg:"addr"`
		Serve bool   `arg:"serve"`
		File  string `arg:"file"`
		Dir   string `arg:"dir"`
	}{}
)

func main() {
	if err := run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func run() error {
	err := a.Parse()
	if err != nil {
		return err
	}
	err = a.Marshal(&c)
	if err != nil {
		return err
	}
	if c.Serve {
		s := server.New(server.Config{
			Addr: c.Addr,
			Dir:  c.Dir,
		})
		log.Infof("listening to %s...\n", c.Addr)
		log.Fatal(s.ListenAndServe())
	} else if c.File != "" {
		log.Info("running", c.File)
		request.RunFile(c.File)
	}

	return nil
}
