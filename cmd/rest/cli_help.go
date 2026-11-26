package main

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/taybart/log"
	"github.com/taybart/rest/file"
	restlua "github.com/taybart/rest/lua"
)

// printCLIHelp writes a cli block's usage and flags for -h/--help, false if
// it has neither
func printCLIHelp(w io.Writer, cli file.CLI) bool {
	usage := ""
	if cli.Usage != nil {
		usage = strings.TrimRight(*cli.Usage, "\n")
	}
	if usage == "" && len(cli.Flags) == 0 {
		return false
	}
	if usage != "" {
		fmt.Fprintln(w, usage)
	}
	if len(cli.Flags) == 0 {
		return true
	}
	if usage != "" {
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w, "flags:")
	names := make([]string, 0, len(cli.Flags))
	for name := range cli.Flags {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		flag := cli.Flags[name]
		fmt.Fprintf(w, "    --%s", name)
		if flag.Short != "" {
			fmt.Fprintf(w, ", -%s", flag.Short)
		}
		var notes []string
		if flag.Required {
			notes = append(notes, "required")
		}
		if flag.Default != "" && !flag.Bool {
			notes = append(notes, "default: "+flag.Default)
		}
		if len(notes) > 0 {
			fmt.Fprintf(w, " (%s)", strings.Join(notes, ", "))
		}
		fmt.Fprintln(w)
		if flag.Desc != "" {
			fmt.Fprintf(w, "\t%s\n", flag.Desc)
		}
	}
	return true
}

// printFailure writes a script's fail(msg) / fail.usage(msg) where rest's
// errors go: the message as an error, then the usage
func printFailure(f *restlua.Failure) {
	if f.Msg != "" {
		log.Errorf("%s\n", f.Msg)
	}
	if f.Usage != "" {
		if f.Msg != "" {
			fmt.Fprintln(log.Output)
		}
		fmt.Fprintln(log.Output, strings.TrimRight(f.Usage, "\n"))
	}
}
