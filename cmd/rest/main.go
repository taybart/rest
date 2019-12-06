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
var stdin bool
var port string
var servelog bool
var servedir bool
var nocolor bool
var outputType string
var index int

func init() {
	flag.Var(&fns, "f", "Filenames of .rest file")
	flag.StringVar(&port, "p", "8080", "Port to attach to, requires -s or -d")
	flag.BoolVar(&servelog, "s", false, "Accept and log requests at localhost:8080")
	flag.BoolVar(&servedir, "d", false, "Serve directory at localhost:8080")
	flag.BoolVar(&stdin, "i", false, "Exec requests in stdin")
	flag.BoolVar(&nocolor, "nc", false, "Remove all color from output")
	flag.StringVar(&outputType, "o", "curl", "Output type")
	flag.IntVar(&index, "b", -1, "Only execute specific index block starting at 0, works with single file only")
}

func help() {
	if len(fns) == 0 {
		fmt.Println("At least one file is required")
	}
	flag.PrintDefaults()
}

func main() {
	flag.Parse()
	log.SetLevel(log.WARN)
	if servelog || servedir {
		serve(servedir, port)
		return
	}
	r := rest.New()
	if nocolor {
		r.NoColor()
		log.UseColors(false)
	}
	switch {
	case stdin:
		r.ReadIO(os.Stdin)
		success, failed := r.Exec()
		// fmt.Println(s, f)
		for _, res := range success {
			fmt.Println(res)
		}
		if len(failed) > 0 {
			if nocolor {
				fmt.Println("Failed requests")
			} else {
				fmt.Printf("%sFailed requests%s\n", log.Red, log.Rtd)
			}
			for _, res := range failed {
				fmt.Println(res)
			}
		}
		os.Exit(0)
	case len(fns) > 0:
		// only use block number when 1 file specified
		if index >= 0 && len(fns) > 1 {
			index = -1
		}
		for _, f := range fns {
			if fileExists(f) {
				valid, err := r.IsRestFile(f)
				if !valid {
					log.Error(err)
					continue
				}
				fmt.Println("Reading...", f)
				err = r.Read(f)
				if err != nil {
					log.Error(err)
				}
			}
		}

		fmt.Println("Done")
		if index >= 0 {
			res, err := r.ExecIndex(index)
			if err != nil {
				fmt.Println(err)
			}
			fmt.Println(res)
			return
		}

		success, failed := r.Exec()
		for _, res := range success {
			fmt.Println(res)
		}
		if len(failed) > 0 {
			if nocolor {
				fmt.Println("Failed requests")
			} else {
				fmt.Printf("%sFailed requests%s\n", log.Red, log.Rtd)
			}
			for _, res := range failed {
				fmt.Println(res)
			}
		}
	default:
		help()
		os.Exit(1)
	}
}

func fileExists(fn string) bool {
	info, err := os.Stat(fn)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}
