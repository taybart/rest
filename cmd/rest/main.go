package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/taybart/args"
	"github.com/taybart/log"
	"github.com/taybart/rest/request"
	"github.com/taybart/rest/server"
)

func usage(u args.Usage) {
	cli := []string{
		"no-color",
	}
	server := []string{
		"addr", "serve", "dir",
		"origins", "tls", "quiet",
	}
	client := []string{
		"file", "block", "label",
		"export", "client", "verbose",
	}

	var usage strings.Builder
	usage.WriteString(
		fmt.Sprintf("%s\t\t=== Rest Easy ===\n%s",
			log.BoldBlue, log.Reset))
	usage.WriteString(
		fmt.Sprintf("%sCLI:\n%s", log.BoldGreen, log.Reset))
	u.BuildFlagString(&usage, cli)
	usage.WriteString(
		fmt.Sprintf("%sServer:\n%s", log.BoldGreen, log.Reset))
	u.BuildFlagString(&usage, server)
	usage.WriteString(
		fmt.Sprintf("%sClient:\n%s", log.BoldGreen, log.Reset))
	u.BuildFlagString(&usage, client)
	fmt.Println(usage.String())
}

var (
	a = args.App{
		Name:    "Rest",
		Version: "v0.0.1",
		Author:  "taybart <taybart@email.com>",
		About:   "all the rest",
		Args: map[string]*args.Arg{
			/*** cli ***/
			"no-color": {
				Short:   "nc",
				Help:    "No colors",
				Default: false,
			},
			"quiet": {
				Short:   "q",
				Help:    "Don't log server requests",
				Default: false,
			},
			"verbose": {
				Short:   "v",
				Help:    "More client logging",
				Default: false,
			},

			/*** server ***/
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
			"origins": {
				Short:   "o",
				Help:    "Add Access-Control-Allow-Origin header value\n\t\tex: -o * or -o 'http://localhost:8080 http://localhost:3000' ",
				Default: "",
			},
			"tls": {
				Short:   "t",
				Help:    "TLS path name to be used for tls key/cert (defaults to no TLS)\n\t\tex: '-t ./keys/site.com' where the files ./keys/site.com.{key,crt} exist",
				Default: "",
			},

			/*** client ***/
			"file": {
				Short: "f",
				Help:  "File to run",
			},
			"block": {
				Short:   "b",
				Help:    "Request block to run, 0-indexed",
				Default: -1,
			},
			"label": {
				Short: "l",
				Help:  "Request label to run",
			},
			"export": {
				Short: "e",
				Help:  "Export file to specified language",
			},
			"client": {
				Short:   "c",
				Help:    "Export full client instead of individual requests",
				Default: false,
			},
		},
		UsageFunc: usage,
	}

	c = struct {
		// cli
		NoColor bool `arg:"no-color"`
		Quiet   bool `arg:"quiet"`
		Verbose bool `arg:"verbose"`

		// server
		Addr    string `arg:"addr"`
		Serve   bool   `arg:"serve"`
		Dir     string `arg:"dir"`
		Origins string `arg:"origins"`
		TLS     string `arg:"tls"`

		// client
		File   string `arg:"file"`
		Block  int    `arg:"block"`
		Label  string `arg:"label"`
		Export string `arg:"export"`
		Client bool   `arg:"client"`
	}{}
)

func main() {
	if err := run(); err != nil {
		log.Error(err)
		os.Exit(1)
	}
}

func run() error {

	log.SetNoTime()

	if err := a.Parse(); err != nil {
		if errors.Is(err, args.ErrUsageRequested) {
			return nil
		}
		return err
	}

	if err := a.Marshal(&c); err != nil {
		return err
	}

	log.UseColors(!c.NoColor)

	if c.Verbose {
		log.SetLevel(log.TRACE)
	}

	/**********
	 * SERVER *
	 **********/
	if c.Serve {
		headers := map[string]string{}
		if c.Origins != "" {
			headers["Access-Control-Allow-Origin"] = c.Origins
		}
		s := server.New(server.Config{
			Addr:    c.Addr,
			Dir:     c.Dir,
			Dump:    c.Quiet,
			Headers: headers,
		})
		log.Infof("listening to %s...\n", c.Addr)
		if c.TLS != "" {
			crt := fmt.Sprintf("%s.crt", c.TLS)
			key := fmt.Sprintf("%s.key", c.TLS)
			if err := s.ListenAndServeTLS(crt, key); err != nil {
				if !errors.Is(err, http.ErrServerClosed) {
					log.Fatal(err)
				}
			}
			return nil
		}
		if err := s.ListenAndServe(); err != nil {
			if !errors.Is(err, http.ErrServerClosed) {
				log.Fatal(err)
			}
		}
		return nil
	}

	/**********
	 * CLIENT *
	 **********/
	if c.File == "" {
		return fmt.Errorf("missing required flag -f")
	}

	if c.Export != "" {
		log.Debugf("exporting file %s to %s (client: %t)\n", c.File, c.Export, c.Client)
		return request.ExportFile(c.File, c.Export, c.Client)
	}

	if c.Block >= 0 {
		log.Debug("running block", c.Block, "on file", c.File)
		return request.RunBlock(c.File, c.Block)
	} else if c.Label != "" {
		log.Debug("running request", c.Label, "on file", c.File)
		return request.RunLabel(c.File, c.Label)
	} else {
		log.Debug("running file", c.File)
		return request.RunFile(c.File)
	}
}
