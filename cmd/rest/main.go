package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/taybart/log"
	"github.com/taybart/rest"
)

type filenames []string

func (i *filenames) String() string {
	return fmt.Sprintf("%v", *i)
}

func (i *filenames) Set(value string) error {
	*i = append(*i, value)
	return nil
}

var fns filenames

func init() {
	flag.Var(&fns, "f", "Filenames of .rest file")
}

func help() {
	if len(fns) == 0 {
		fmt.Println("At least one file is required")
	}
	flag.PrintDefaults()
}

func main() {
	flag.Parse()
	log.SetLevel(log.INFO)
	if len(fns) == 0 {
		help()
		os.Exit(1)
	}
	r := rest.New()
	for _, f := range fns {
		if fileExists(f) {
			log.Info("Reading...", f)
			err := r.Read(f)
			if err != nil {
				log.Error(err)
			}
		}
	}
	log.Info("Excuting...")
	success, failed := r.Exec()
	for _, res := range success {
		fmt.Println(res)
	}
	if len(failed) > 0 {
		log.Error("Failed requests")
		for _, res := range failed {
			fmt.Println(res)
		}
	}
	log.Info("Done")
}

func fileExists(fn string) bool {
	info, err := os.Stat(fn)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}
