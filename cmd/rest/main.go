package main

import (
	"fmt"
	"os"

	"github.com/taybart/log"
	"github.com/taybart/rest"
)

func main() {
	log.SetLevel(log.WARN)
	for _, f := range os.Args[1:] {
		if fileExists(f) {
			r := rest.New()
			err := r.Read(f)
			if err != nil {
				log.Error(err)
			}
			responses := r.Exec()
			for _, res := range responses {
				fmt.Println(res)
			}
		}
	}
}

func fileExists(fn string) bool {
	info, err := os.Stat(fn)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}
