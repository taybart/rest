package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/taybart/args"
	"github.com/taybart/log"
	"github.com/taybart/rest"
	"github.com/taybart/rest/server"
)

func usage(u args.Usage) {
	cli := []string{
		"no-color", "list",
	}
	server := []string{
		"addr", "serve", "dir", "spa", "file",
		"cors", "response", "tls", "quiet",
	}
	client := []string{
		"file", "block", "label",
		"socket", "export", "verbose",
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
		Version: "v0.7.2",
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
			"list": {
				Help:    "List labels in file",
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
			"response": {
				Short: "r",
				Help:  `Response to send, json file path or inline in the format {"status": 200, "body": {"hello": "world"}}`,
			},
			"cors": {
				Help:    "Add cors headers",
				Default: false,
			},
			"tls": {
				Short:   "t",
				Help:    "TLS path name to be used for tls key/cert (defaults to no TLS)\n\tex: '-t ./keys/site.com' where the files ./keys/site.com.{key,crt} exist",
				Default: "",
			},
			// TODO: add to server block
			"spa": {
				Help:    "Serve index.html in directory instead of 404",
				Default: false,
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
			"ignore-fail": {
				Help:    "Ignore errors and run all blocks",
				Default: false,
			},
			/*** socket ***/
			"socket": {
				Short:            "S",
				Help:             "Run the socket block (ignores requests)\n\tif set like \"--socket/-S run\", rest will run socket.run.order and exit",
				DoesNotNeedValue: true,
				Default:          "",
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
		Addr     string `arg:"addr"`
		Serve    bool   `arg:"serve"`
		Dir      string `arg:"dir"`
		Response string `arg:"response"`
		Cors     bool   `arg:"cors"`
		TLS      string `arg:"tls"`
		SPA      bool   `arg:"spa"`

		// client
		File       string `arg:"file"`
		Block      int    `arg:"block"`
		Label      string `arg:"label"`
		List       bool   `arg:"list"`
		Socket     string `arg:"socket"`
		Export     string `arg:"export"`
		IgnoreFail bool   `arg:"ignore-fail"`
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
		if a.UserSet("file") {
			f, err := rest.NewFile(c.File)
			if err != nil {
				return err
			}
			return f.RunServer()
		}
		// FIXME: idk if this works the same as before
		res, err := parseServerResponse(c.Response)
		if err != nil {
			return err
		}
		s := server.New(server.Config{
			Addr:     c.Addr,
			Dir:      c.Dir,
			Quiet:    c.Quiet,
			Cors:     c.Cors,
			Response: res,
			SPA:      c.SPA,
		})
		return s.Serve()
	}

	/**********
	 * CLIENT *
	 **********/
	if !a.UserSet("file") {
		return fmt.Errorf("missing required flag -f")
	}

	f, err := rest.NewFile(c.File)
	if err != nil {
		return err
	}

	if c.Export != "" {
		log.Debugf("exporting file %s to %s\n", c.File, c.Export)
		return f.Export(c.Export, c.Label, c.Block)
	}

	if a.Get("socket").Provided {
		log.Debug("running socket block on file", c.File)
		return f.RunSocket(c.Socket)
	}

	if c.List {
		for _, b := range f.Requests {
			fmt.Println(b.Label)
		}
		return nil
	}

	if c.Block >= 0 {
		log.Debug("running block", c.Block, "on file", c.File)
		return f.RunIndex(c.Block)
	} else if c.Label != "" {
		log.Debug("running request", c.Label, "on file", c.File)
		return f.RunLabel(c.Label)
	} else {

		if f.Parser.Root.CLI != nil && os.Getenv("__REST_CLI") != "true" {
			os.Setenv("__REST_CLI", "true") // inf loop guard
			return RunCLITool(f)
		}
		log.Debug("running file", c.File)
		return f.RunFile(c.IgnoreFail)
	}
}

func parseServerResponse(responseFlag string) (*server.Response, error) {
	if responseFlag == "" {
		return nil, nil
	}
	// check if c.Res is a file or inline
	if _, err := os.Stat(responseFlag); err == nil {
		f, err := os.ReadFile(responseFlag)
		if err != nil {
			return nil, err
		}
		res := &server.Response{}
		if err := json.Unmarshal(f, res); err != nil {
			return nil, err
		}
		return res, nil
	}
	res := &server.Response{}
	if err := json.Unmarshal([]byte(responseFlag), res); err != nil {
		return nil, fmt.Errorf(
			"could not unmarshal inline response: %s %w",
			[]byte(c.Response), err)
	}
	return res, nil
}
