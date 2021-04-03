package main

import (
	"errors"
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

var (
	fns        filenames
	stdin      bool
	port       string
	servelog   bool
	servedir   string
	servespa   bool
	nocolor    bool
	verbose    bool
	outputLang string
	makeClient bool
	index      int
	local      bool
)

func init() {
	flag.Var(&fns, "f", "Filenames of .rest file")
	flag.StringVar(&port, "p", "8080", "Port to attach to, requires -s or -d")
	flag.BoolVar(&servelog, "s", false, "Accept and log requests at :8080 or -p")
	flag.BoolVar(&servespa, "spa", false, "Use in case of SPA")
	flag.StringVar(&servedir, "d", "", "Serve directory at :8080 or -p")
	flag.BoolVar(&local, "l", false, "Bind only to localhost")
	flag.BoolVar(&stdin, "i", false, "Exec requests in stdin")
	flag.BoolVar(&nocolor, "nc", false, "Remove all color from output")
	flag.BoolVar(&verbose, "v", false, "Verbose output")
	flag.StringVar(&outputLang, "o", "", "Output type [go, js/javascript, curl]")
	flag.BoolVar(&makeClient, "c", false, "Make client instead of just requests")
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

	log.SetPlain()
	log.SetLevel(log.WARN)
	if verbose {
		log.SetLevel(log.DEBUG)
	}

	r := rest.New()
	if nocolor {
		r.NoColor()
		log.UseColors(false)
	}
	if err := run(r); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func run(r *rest.Rest) error {
	if servelog || servedir != "" || servespa {
		serve(port)
		return nil
	}

	// only use block number when 1 file specified
	if index >= 0 && len(fns) > 1 {
		index = -1
	}
	if stdin {
		readStdin(r)
	}
	if len(fns) > 0 {
		readFiles(r)
		if makeClient && outputLang == "" {
			return errors.New("Must specify output language with -o")
		}
		if makeClient {
			log.Debug("Making client")
			client, err := r.SynthesizeClient(outputLang)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			fmt.Println(client)
			return nil
		} else if outputLang != "" {
			requests, _, err := r.SynthesizeRequests(outputLang)
			for i, req := range requests {
				fmt.Printf("\n~~~~~~~ %d ~~~~~~~\n\n", i)
				fmt.Println(req)
			}
			if err != nil {
				return err
			}
			return nil
		}
		exec(r)
		return nil
	}
	help()
	return errors.New("")
}

func readFiles(r *rest.Rest) {
	for _, f := range fns {
		log.Debugf("Reading file %s...\n", f)
		if fileExists(f) {
			valid, err := r.IsRestFile(f)
			if !valid {
				log.Error(err)
				continue
			}
			err = r.Read(f)
			if err != nil {
				log.Error("Read error", err)
				continue
			}
			log.Debug("done\n")
		}
	}
}

func exec(r *rest.Rest) {
	log.Debug("\nExecuting all requests\n")
	if index >= 0 {
		res, err := r.ExecIndex(index)
		if err != nil {
			fmt.Println(err)
		}
		fmt.Println(res)
		os.Exit(0)
	}

	success, err := r.Exec()
	for _, res := range success {
		fmt.Println(res)
	}
	// if len(failed) > 0 {
	if err != nil {
		if nocolor {
			fmt.Println("Failed requests")
		} else {
			fmt.Printf("%sFailed requests%s\n", log.Red, log.Rtd)
		}
		fmt.Println(err)
		/* for _, res := range failed {
		} */
	}
}

func readStdin(r *rest.Rest) {
	err := r.ReadIO(os.Stdin)
	if err != nil {
		log.Error(err)
		return
	}
	success, err := r.Exec()
	for _, res := range success {
		fmt.Println(res)
	}
	// if len(failed) > 0 {
	if err != nil {
		if nocolor {
			fmt.Println("Failed requests")
		} else {
			fmt.Printf("%sFailed requests%s\n", log.Red, log.Rtd)
		}
		fmt.Println(err)
		// for _, res := range failed {
		// fmt.Println(res)
		// }
	}
	os.Exit(0)
}

func fileExists(fn string) bool {
	info, err := os.Stat(fn)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}
