package main

import (
	"fmt"
	"os"

	"github.com/taybart/rest/request"
)

var (
	filename = "cf.rest"
)

func run() error {
	return request.RunFile(filename)
}

func main() {
	if err := run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
