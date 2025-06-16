package main

import (
	"fmt"
	"os"

	"github.com/taybart/rest/request"
)

var (
	filename = "cf.rest"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
}

func run() error {
	return request.RunFile(filename)
}
