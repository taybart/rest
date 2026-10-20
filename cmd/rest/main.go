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
	"github.com/taybart/rest/file"
	"github.com/taybart/rest/server"
	"golang.org/x/term"
)

func usage(u args.Usage) {
	cli := []string{
		"no-color", "list",
	}
	server := []string{
		"addr", "serve", "dir", "spa", "file",
		"cors", "response", "tls", "tailscale", "quiet",
	}
	client := []string{
		"file", "block", "label",
		"socket", "export", "verbose",
	}

	var usage strings.Builder
	fmt.Fprintf(&usage, "%s\t\t=== Rest Easy ===\n%s", log.BoldBlue, log.Reset)
	fmt.Fprintf(&usage, "%sCLI:\n%s", log.BoldGreen, log.Reset)
	u.BuildFlagString(&usage, cli)
	fmt.Fprintf(&usage, "%sServer:\n%s", log.BoldGreen, log.Reset)
	u.BuildFlagString(&usage, server)
	fmt.Fprintf(&usage, "%sClient:\n%s", log.BoldGreen, log.Reset)
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
			"tailscale": {
				Help:    "Serve https using this machine's tailscale cert (needs the tailscale cli and HTTPS enabled on the tailnet)\n\tlistens on :8080 unless --addr is set so other tailnet devices can connect",
				Default: false,
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
		Addr      string `arg:"addr"`
		Serve     bool   `arg:"serve"`
		Dir       string `arg:"dir"`
		Response  string `arg:"response"`
		Cors      bool   `arg:"cors"`
		TLS       string `arg:"tls"`
		Tailscale bool   `arg:"tailscale"`
		SPA       bool   `arg:"spa"`

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

func detectShebang() string {
	// When invoked via shebang, the script path is os.Args[1]
	// (os.Args[0] is the interpreter: "rest")
	if len(os.Args) < 2 {
		return ""
	}
	script := os.Args[1]
	f, err := os.Open(script)
	if err != nil {
		return ""
	}
	defer f.Close()

	buf := make([]byte, 2)
	if _, err := f.Read(buf); err != nil {
		return ""
	}
	if buf[0] == '#' && buf[1] == '!' {
		return script
	}
	return ""
}

func detectFileFlag() string {
	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		if arg == "-f" || arg == "--file" {
			if i+1 < len(os.Args) {
				return os.Args[i+1]
			}
		} else if strings.HasPrefix(arg, "-f=") {
			return strings.TrimPrefix(arg, "-f=")
		} else if strings.HasPrefix(arg, "--file=") {
			return strings.TrimPrefix(arg, "--file=")
		}
	}
	return ""
}

func stripFileFlag(args []string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "-f" || arg == "--file" {
			i++ // skip value
			continue
		}
		if strings.HasPrefix(arg, "-f=") || strings.HasPrefix(arg, "--file=") {
			continue
		}
		out = append(out, arg)
	}
	return out
}

func hasHelpFlag(args []string) bool {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" {
			return true
		}
	}
	return false
}

func run() error {

	log.SetNoTime()

	isShebang := false
	targetFile := detectShebang()
	if targetFile != "" {
		isShebang = true
	} else {
		targetFile = detectFileFlag()
	}

	var preloaded *rest.Rest
	if targetFile != "" && targetFile != "-" {
		if f, err := rest.NewFile(targetFile); err == nil {
			preloaded = f
		}
	}

	if preloaded != nil && preloaded.Parser.Config.NoFlags {
		if isShebang {
			os.Args = append([]string{os.Args[0]}, os.Args[2:]...)
		} else {
			os.Args = append([]string{os.Args[0]}, stripFileFlag(os.Args[1:])...)
		}
		if preloaded.Parser.Root.CLI != nil {
			cliBlock, err := preloaded.Parser.CLI()
			if err != nil {
				return err
			}
			cliFlagValues, helpRequested, err := parseCLIFlagsWithArgs(cliBlock.Flags, nil)
			if err != nil {
				return err
			}
			if helpRequested {
				return nil
			}
			return runCLITool(preloaded, cliBlock, cliFlagValues)
		}
		return preloaded.RunFile(false)
	}

	// If invoked via shebang, inject -f with the script path
	// unless the user already provided -f
	if isShebang {
		if detectFileFlag() == "" {
			os.Args = append([]string{os.Args[0], "-f", targetFile}, os.Args[2:]...)
		}
	}

	// If file has a CLI block and user passed -h/--help, let the CLI block's args parser print help
	if preloaded != nil && preloaded.Parser.Root.CLI != nil && hasHelpFlag(os.Args[1:]) {
		cliBlock, err := preloaded.Parser.CLI()
		if err != nil {
			return err
		}
		reserved := getReservedFlags(a)
		_, _, err = parseCLIFlagsWithArgs(cliBlock.Flags, reserved)
		return err
	}

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
			f := preloaded
			if f == nil {
				var err error
				f, err = rest.NewFile(c.File)
				if err != nil {
					return err
				}
			}
			return f.RunServer()
		}
		// FIXME: idk if this works the same as before
		res, err := parseServerResponse(c.Response)
		if err != nil {
			return err
		}
		addr := c.Addr
		if c.Tailscale && !a.UserSet("addr") {
			// the default is localhost, which tailnet peers can't reach
			addr = ":8080"
		}
		s := server.New(server.Config{
			Addr:      addr,
			Dir:       c.Dir,
			Quiet:     c.Quiet,
			Cors:      c.Cors,
			Response:  res,
			SPA:       c.SPA,
			TLS:       c.TLS,
			Tailscale: c.Tailscale,
		})
		return s.Serve()
	}

	/**********
	 * CLIENT *
	 **********/
	if !a.UserSet("file") {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			c.File = "-"
		} else {
			return fmt.Errorf("missing required flag -f")
		}
	}

	f := preloaded
	if f == nil {
		var err error
		f, err = rest.NewFile(c.File)
		if err != nil {
			return err
		}
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
		if f.Parser.Root.CLI != nil {
			cliBlock, err := f.Parser.CLI()
			if err != nil {
				return err
			}
			reserved := getReservedFlags(a)
			cliFlagValues, helpRequested, err := parseCLIFlagsWithArgs(cliBlock.Flags, reserved)
			if err != nil {
				return err
			}
			if helpRequested {
				return nil
			}
			return runCLITool(f, cliBlock, cliFlagValues)
		}
		log.Debug("running file", c.File)
		return f.RunFile(c.IgnoreFail)
	}
}

func getReservedFlags(a args.App) map[string]bool {
	reserved := map[string]bool{
		"help": true,
		"h":    true,
	}
	for name, arg := range a.Args {
		reserved[name] = true
		if arg.Short != "" {
			reserved[arg.Short] = true
		}
	}
	return reserved
}

func parseCLIFlagsWithArgs(flags map[string]file.CLIFlag, reserved map[string]bool) (map[string]string, bool, error) {
	cliApp := args.App{
		Args: make(map[string]*args.Arg, len(flags)),
	}
	for name, flagDef := range flags {
		if reserved != nil {
			if reserved[name] {
				return nil, false, fmt.Errorf("cli flag %q conflicts with a built-in flag", name)
			}
			if flagDef.Short != "" && reserved[flagDef.Short] {
				return nil, false, fmt.Errorf("cli short flag %q conflicts with a built-in flag", flagDef.Short)
			}
		} else {
			if name == "help" || name == "h" {
				return nil, false, fmt.Errorf("cli flag %q conflicts with help flag", name)
			}
			if flagDef.Short == "h" {
				return nil, false, fmt.Errorf("cli short flag %q conflicts with help flag", flagDef.Short)
			}
		}
		arg := &args.Arg{
			Short:    flagDef.Short,
			Help:     flagDef.Desc,
			Required: flagDef.Required,
		}
		if flagDef.Bool {
			arg.Default = false
		} else if flagDef.Default != "" {
			arg.Default = flagDef.Default
		}
		cliApp.Args[name] = arg
	}

	if err := cliApp.Parse(); err != nil {
		if errors.Is(err, args.ErrUsageRequested) {
			return nil, true, nil
		}
		return nil, false, err
	}

	parsed := make(map[string]string)
	for name, flagDef := range flags {
		arg := cliApp.Get(name)
		if arg == nil {
			continue
		}
		if flagDef.Bool {
			if arg.Bool() {
				parsed[name] = "true"
			}
		} else if arg.IsSet() {
			parsed[name] = arg.String()
		}
	}
	return parsed, false, nil
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
