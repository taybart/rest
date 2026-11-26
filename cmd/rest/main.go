package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/taybart/args"
	"github.com/taybart/log"
	"github.com/taybart/rest"
	"github.com/taybart/rest/exports"
	"github.com/taybart/rest/file"
	"github.com/taybart/rest/imports"
	restlua "github.com/taybart/rest/lua"
	"github.com/taybart/rest/server"
	"golang.org/x/term"
)

func usage(u args.Usage) {
	cli := []string{
		"no-color", "list", "flags",
	}
	server := []string{
		"addr", "serve", "dir", "spa", "file", "dev",
		"cors", "response", "tls", "tailscale", "quiet",
	}
	client := []string{
		"file", "block", "label",
		"test", "socket", "export", "import", "side", "verbose",
	}

	var usage strings.Builder
	fmt.Fprintf(&usage, "%sRest %s\n%s", log.BoldBlue, u.Version, log.Reset)
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
		Version: "v0.11.0",
		Author:  "taybart",
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
			"flags": {
				Help:    "Force built-in flags (overrides config.no_flags)",
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
			"dev": {
				Help:    "Serve a file in dev mode: sets __REST_DEV=1, no handler cache, and pages reload when\n\tfiles change (under `run -w`) or rest restarts",
				Default: false,
			},
			"spa": {
				Help:    "Serve index.html in directory instead of 404",
				Default: false,
			},

			/*** client ***/
			"file": {
				Short: "f",
				Help:  "File to run, or a folder w/ one .rest file w/ a server block (ex. -s -f /var/www/site.com)",
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
				Help:  "Export file to specified language (use ls to list)",
			},
			"import": {
				Short: "i",
				Help:  "Scaffold a rest file from an OpenAPI spec (json), printed to stdout",
			},
			"side": {
				Help:    "What --import generates: client, server or both",
				Default: "both",
			},
			"ignore-fail": {
				Help:    "Ignore errors and run all blocks",
				Default: false,
			},
			"test": {
				Short:            "T",
				Help:             "Run the file's test blocks, or the one named (ex. -T checkout)",
				DoesNotNeedValue: true,
				Default:          "",
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
		Flags   bool `arg:"flags"`

		// server
		Addr      string `arg:"addr"`
		Serve     bool   `arg:"serve"`
		Dir       string `arg:"dir"`
		Response  string `arg:"response"`
		Cors      bool   `arg:"cors"`
		TLS       string `arg:"tls"`
		Tailscale bool   `arg:"tailscale"`
		SPA       bool   `arg:"spa"`
		Dev       bool   `arg:"dev"`

		// client
		File       string `arg:"file"`
		Block      int    `arg:"block"`
		Label      string `arg:"label"`
		List       bool   `arg:"list"`
		Socket     string `arg:"socket"`
		Test       string `arg:"test"`
		Export     string `arg:"export"`
		Import     string `arg:"import"`
		Side       string `arg:"side"`
		IgnoreFail bool   `arg:"ignore-fail"`
	}{}
)

func main() {
	if err := run(); err != nil {
		var failure *restlua.Failure
		if errors.As(err, &failure) {
			printFailure(failure)
			os.Exit(failure.Code)
		}
		log.Error(err)
		os.Exit(1)
	}
}

// serverFlags configure the ad-hoc server (-s w/o -f), a file's server block
// has its own settings for each
var serverFlags = map[string]string{
	"addr":      "address",
	"dir":       "directory",
	"cors":      "cors",
	"spa":       "spa",
	"tls":       "tls",
	"tailscale": "tailscale",
	"quiet":     "quiet",
	"response":  "response",
}

// checkServerFlags errors for server flags given w/ -f, which would be
// ignored (the server block is used as is)
func checkServerFlags() error {
	for _, flag := range slices.Sorted(maps.Keys(serverFlags)) {
		if a.UserSet(flag) {
			return fmt.Errorf("--%s doesn't apply to a file's server, set %s in its server block", flag, serverFlags[flag])
		}
	}
	return nil
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

func hasFlagsOverride(args []string) bool {
	for _, arg := range args {
		if arg == "--flags" || arg == "-flags" {
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

	flagsOverride := hasFlagsOverride(os.Args[1:])

	if preloaded != nil && preloaded.Parser.Config.NoFlags && !flagsOverride {
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
			if hasHelpFlag(os.Args[1:]) {
				if !printCLIHelp(os.Stdout, cliBlock) {
					fmt.Println("no usage or flags to show, add usage = \"...\" to the cli block")
				}
				return nil
			}
			cliFlagValues, restArgs, helpRequested, err := parseCLIFlagsWithArgs(cliBlock.Flags, nil)
			if err != nil {
				return err
			}
			if helpRequested {
				return nil
			}
			return runCLITool(preloaded, cliBlock, cliFlagValues, restArgs)
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
	if preloaded != nil && preloaded.Parser.Root.CLI != nil && hasHelpFlag(os.Args[1:]) && !flagsOverride {
		cliBlock, err := preloaded.Parser.CLI()
		if err != nil {
			return err
		}
		if !printCLIHelp(os.Stdout, cliBlock) {
			a.Usage() // nothing to show for the cli, rest's own help
		}
		return nil
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

	if c.Import != "" {
		return importSpec(c.Import, c.Side, c.Addr)
	}

	/**********
	 * SERVER *
	 **********/
	if c.Serve {
		if a.UserSet("file") {
			if err := checkServerFlags(); err != nil {
				return err
			}
			f := preloaded
			if c.Dev {
				// set before parsing, so env("__REST_DEV") sees it
				os.Setenv("__REST_DEV", "1")
				f = nil
			}
			if f == nil {
				var err error
				f, err = rest.NewFile(c.File)
				if err != nil {
					return err
				}
			}
			f.Dev = c.Dev
			return f.RunServer()
		}
		if c.Dev {
			return errors.New("--dev serves a file, use it w/ -f")
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
	// the systemd unit is the same for every site, w/ -f it checks the file
	// and names the site in the install comment
	if c.Export == "systemd" && !a.UserSet("file") {
		return exports.ToSystemd(os.Stdout, "")
	}
	if !a.UserSet("file") {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			c.File = "-"
		} else {
			fmt.Printf("%smissing required flag -f <file> %s\n\n", log.BoldRed, log.Reset)
			a.Usage()
			return nil
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

	if a.Get("test").Provided {
		log.Debug("running tests on file", c.File)
		return f.RunTests(c.Test)
	}

	if c.List {
		for _, b := range f.Requests {
			fmt.Println(b.Label)
		}
		for _, t := range f.Parser.Root.Tests {
			fmt.Printf("test %q\n", t.Label)
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
		if f.Parser.Root.CLI != nil && !c.Flags {
			cliBlock, err := f.Parser.CLI()
			if err != nil {
				return err
			}
			reserved := getReservedFlags(a)
			cliFlagValues, restArgs, helpRequested, err := parseCLIFlagsWithArgs(cliBlock.Flags, reserved)
			if err != nil {
				return err
			}
			if helpRequested {
				return nil
			}
			return runCLITool(f, cliBlock, cliFlagValues, restArgs)
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

func parseCLIFlagsWithArgs(flags map[string]file.CLIFlag, reserved map[string]bool) (map[string]string, []string, bool, error) {
	cliApp := args.App{
		Args: make(map[string]*args.Arg, len(flags)),
	}
	for name, flagDef := range flags {
		if reserved != nil {
			if reserved[name] {
				return nil, nil, false, fmt.Errorf("cli flag %q conflicts with a built-in flag", name)
			}
			if flagDef.Short != "" && reserved[flagDef.Short] {
				return nil, nil, false, fmt.Errorf("cli short flag %q conflicts with a built-in flag", flagDef.Short)
			}
		} else {
			if name == "help" || name == "h" {
				return nil, nil, false, fmt.Errorf("cli flag %q conflicts with help flag", name)
			}
			if flagDef.Short == "h" {
				return nil, nil, false, fmt.Errorf("cli short flag %q conflicts with help flag", flagDef.Short)
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
			return nil, nil, true, nil
		}
		return nil, nil, false, err
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

	restArgs := extractRemainingArgs(os.Args[1:], flags, reserved != nil)
	return parsed, restArgs, false, nil
}

func extractRemainingArgs(rawArgs []string, flags map[string]file.CLIFlag, includeBuiltins bool) []string {
	needsValue := make(map[string]bool)
	doesNotNeedValue := make(map[string]bool)

	if includeBuiltins {
		for name, arg := range a.Args {
			isBool := false
			if _, ok := arg.Default.(bool); ok {
				isBool = true
			}
			if !isBool {
				needsValue[name] = true
				if arg.Short != "" {
					needsValue[arg.Short] = true
				}
				if arg.DoesNotNeedValue {
					doesNotNeedValue[name] = true
					if arg.Short != "" {
						doesNotNeedValue[arg.Short] = true
					}
				}
			}
		}
	}

	for name, flagDef := range flags {
		if !flagDef.Bool {
			needsValue[name] = true
			if flagDef.Short != "" {
				needsValue[flagDef.Short] = true
			}
		}
	}

	var rest []string
	for i := 0; i < len(rawArgs); i++ {
		v := rawArgs[i]
		if v == "--" {
			rest = append(rest, rawArgs[i+1:]...)
			break
		}
		if strings.HasPrefix(v, "-") && len(v) > 1 {
			name := strings.TrimLeft(v, "-")
			hasEquals := false
			if idx := strings.IndexByte(name, '='); idx != -1 {
				name = name[:idx]
				hasEquals = true
			}
			if !hasEquals && needsValue[name] {
				if i+1 < len(rawArgs) {
					if doesNotNeedValue[name] && strings.HasPrefix(rawArgs[i+1], "-") {
						continue
					}
					i++ // skip flag value
				}
			}
			continue
		}
		rest = append(rest, v)
	}
	return rest
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

func importSpec(file, side, addr string) error {
	var spec []byte
	var err error
	if file == "-" {
		spec, err = io.ReadAll(os.Stdin)
	} else {
		spec, err = os.ReadFile(file)
	}
	if err != nil {
		return err
	}
	out, err := imports.FromOpenAPI(spec, imports.Options{Side: imports.Side(side), Addr: addr})
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(out)
	return err
}
