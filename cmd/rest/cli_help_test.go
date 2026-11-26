package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/taybart/rest/file"
	restlua "github.com/taybart/rest/lua"
)

func TestPrintCLIHelp(t *testing.T) {
	usage := "usage: x [a|b] <c>\n"
	cli := file.CLI{
		Usage: &usage,
		Flags: map[string]file.CLIFlag{
			"verbose": {Desc: "talk more", Short: "v", Bool: true},
			"env":     {Desc: "where", Default: "dev", Required: true},
		},
	}
	var buf bytes.Buffer
	if !printCLIHelp(&buf, cli) {
		t.Fatal("expected help")
	}
	want := `usage: x [a|b] <c>

flags:
    --env (required, default: dev)
	where
    --verbose, -v
	talk more
`
	if buf.String() != want {
		t.Fatalf("expected:\n%s\ngot:\n%s", want, buf.String())
	}
	if printCLIHelp(&buf, file.CLI{}) {
		t.Fatal("expected no help w/o a usage or flags")
	}
}

func writeCLIScript(t *testing.T, fn string) (script, marker string) {
	t.Helper()
	dir := t.TempDir()
	marker = filepath.Join(dir, "ran")
	script = filepath.Join(dir, "x.rest")
	content := `#!/usr/bin/env rest
config { no_flags = true }
cli {
  usage = "usage: x <thing>"
  fn = <<MIDNIGHT
    local f = io.open('` + marker + `', 'w'); f:write('ran'); f:close()
    ` + fn + `
  MIDNIGHT
}
`
	if err := os.WriteFile(script, []byte(content), 0755); err != nil {
		t.Fatal(err)
	}
	return script, marker
}

// -h/--help is answered by rest (the fn doesn't run), even w/o flags
func TestCLIHelpWithoutFlags(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	for _, flag := range []string{"-h", "--help"} {
		script, marker := writeCLIScript(t, "")
		os.Args = []string{"rest", script, flag}
		if err := run(); err != nil {
			t.Fatalf("%s: %v", flag, err)
		}
		if _, err := os.Stat(marker); !os.IsNotExist(err) {
			t.Fatalf("%s: the cli fn ran", flag)
		}
	}
}

func TestCLIFailUsage(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	script, _ := writeCLIScript(t, `if !cli.rest[1] { fail.usage('no thing') }`)
	os.Args = []string{"rest", script}
	err := run()
	var f *restlua.Failure
	if !errors.As(err, &f) {
		t.Fatalf("expected a *Failure, got %T: %v", err, err)
	}
	want := restlua.Failure{Msg: "no thing", Code: 2, Usage: "usage: x <thing>"}
	if *f != want {
		t.Fatalf("expected %+v, got %+v", want, *f)
	}
}
