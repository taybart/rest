package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/taybart/rest/file"
)

func TestParseCLIFlagsWithArgs_NoFlags(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	flags := map[string]file.CLIFlag{
		"dir": {
			Desc:    "Directory",
			Short:   "d",
			Default: "/default/dir",
		},
		"restart": {
			Desc:  "Restart service",
			Short: "r",
			Bool:  true,
		},
		"script": {
			Desc:  "Script to run",
			Short: "s",
		},
	}

	os.Args = []string{"rest", "-d", "/custom/dir", "-r", "--script=app.sh", "what", "is", "the", "capital"}
	vals, restArgs, help, err := parseCLIFlagsWithArgs(flags, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if help {
		t.Fatal("expected help to be false")
	}
	if vals["dir"] != "/custom/dir" {
		t.Fatalf("expected dir=/custom/dir, got %q", vals["dir"])
	}
	if vals["restart"] != "true" {
		t.Fatalf("expected restart=true, got %q", vals["restart"])
	}
	if vals["script"] != "app.sh" {
		t.Fatalf("expected script=app.sh, got %q", vals["script"])
	}
	if len(restArgs) != 4 || restArgs[0] != "what" || restArgs[3] != "capital" {
		t.Fatalf("unexpected restArgs: %v", restArgs)
	}
}

func TestParseCLIFlagsWithArgs_DefaultsAndHelp(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	flags := map[string]file.CLIFlag{
		"dir": {
			Desc:    "Directory",
			Short:   "d",
			Default: "/default/dir",
		},
		"restart": {
			Desc:  "Restart service",
			Short: "r",
			Bool:  true,
		},
	}

	os.Args = []string{"rest"}
	vals, _, help, err := parseCLIFlagsWithArgs(flags, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if help {
		t.Fatal("expected help to be false")
	}
	if vals["dir"] != "/default/dir" {
		t.Fatalf("expected dir=/default/dir, got %q", vals["dir"])
	}
	if _, ok := vals["restart"]; ok {
		t.Fatalf("expected restart to not be set when false, got %q", vals["restart"])
	}

	os.Args = []string{"rest", "-h"}
	_, _, help, err = parseCLIFlagsWithArgs(flags, nil)
	if err != nil {
		t.Fatalf("unexpected error on -h: %v", err)
	}
	if !help {
		t.Fatal("expected help to be true on -h")
	}
}

func TestParseCLIFlagsWithArgs_ReservedConflict(t *testing.T) {
	reserved := getReservedFlags(a)
	flags := map[string]file.CLIFlag{
		"custom": {
			Desc:  "Conflicts with -d (dir)",
			Short: "d",
		},
	}
	_, _, _, err := parseCLIFlagsWithArgs(flags, reserved)
	if err == nil {
		t.Fatal("expected conflict error when short flag conflicts with built-in flag")
	}
}

func TestRunShebangWithNoFlags(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	tmpDir := t.TempDir()
	outFile := filepath.Join(tmpDir, "out.txt")
	scriptFile := filepath.Join(tmpDir, "run.rest")
	content := `#!/usr/bin/env rest

config {
  no_flags = true
}

cli {
  flags = {
    dir = {
      desc = "The directory"
      short = "d"
    }
    restart = {
      desc = "Restart service"
      short = "r"
      bool = true
    }
  }
  fn = <<LUA
    local f = io.open("` + outFile + `", "w")
    f:write((cli.dir or "") .. ":" .. tostring(cli.restart == true) .. ":" .. cli.rest:string() .. ":" .. cli.rest[1])
    f:close()
  LUA
}
`
	if err := os.WriteFile(scriptFile, []byte(content), 0755); err != nil {
		t.Fatal(err)
	}

	os.Args = []string{"rest", scriptFile, "-d", "/my/dir", "-r", "what", "is", "the", "capital"}
	if err := run(); err != nil {
		t.Fatalf("run() failed: %v", err)
	}

	got, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "/my/dir:true:what is the capital:what" {
		t.Fatalf("expected /my/dir:true:what is the capital:what, got %q", string(got))
	}
}

func TestRunHeredocStdin(t *testing.T) {
	origArgs := os.Args
	origStdin := os.Stdin
	defer func() {
		os.Args = origArgs
		os.Stdin = origStdin
	}()

	tmpDir := t.TempDir()
	outFile := filepath.Join(tmpDir, "stdin_out.txt")

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	content := `cli {
  fn = <<LUA
    local f = io.open("` + outFile + `", "w")
    f:write("from-stdin")
    f:close()
  LUA
}
`
	if _, err := w.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	w.Close()

	os.Stdin = r
	os.Args = []string{"rest"}
	if err := run(); err != nil {
		t.Fatalf("run() with stdin failed: %v", err)
	}

	got, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "from-stdin" {
		t.Fatalf("expected from-stdin, got %q", string(got))
	}
}

func TestRunCLIFnCallsRestLoop(t *testing.T) {
	origArgs := os.Args
	origStdin := os.Stdin
	defer func() {
		os.Args = origArgs
		os.Stdin = origStdin
	}()

	tmpDir := t.TempDir()
	outFile := filepath.Join(tmpDir, "loop_out.txt")
	scriptFile := filepath.Join(tmpDir, "loop.rest")
	content := `cli {
  fn = <<LUA
    history = { "init" }
    rest.loop()
    local f = io.open("` + outFile + `", "w")
    f:write(table.concat(history, ","))
    f:close()
  LUA
  loop = <<LUA
    if input == "quit" then
      break
    end
    table.insert(history, input)
  LUA
}
`
	if err := os.WriteFile(scriptFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("first\nsecond\nquit\n")); err != nil {
		t.Fatal(err)
	}
	w.Close()

	os.Stdin = r
	os.Args = []string{"rest", "-f", scriptFile}
	if err := run(); err != nil {
		t.Fatalf("run() failed: %v", err)
	}

	got, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "init,first,second" {
		t.Fatalf("expected init,first,second, got %q", string(got))
	}
}

func TestRunFlagsOverrideNoFlags(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	var hit string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = r.URL.Path
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	scriptFile := filepath.Join(tmpDir, "run.rest")
	content := `#!/usr/bin/env rest

config {
  no_flags = true
}

cli {
  flags = {
    dir = {
      desc = "The directory"
      short = "d"
    }
  }
  fn = <<LUA
    rest.label("default_cli")
  LUA
}

request "default_cli" {
  url = "` + srv.URL + `/default_cli"
}

request "specific_label" {
  url = "` + srv.URL + `/specific_label"
}
`
	if err := os.WriteFile(scriptFile, []byte(content), 0755); err != nil {
		t.Fatal(err)
	}

	os.Args = []string{"rest", "--flags", "-nc", "-f", scriptFile, "-l", "specific_label"}
	if err := run(); err != nil {
		t.Fatalf("run() with --flags failed: %v", err)
	}
	if hit != "/specific_label" {
		t.Fatalf("expected /specific_label to be hit when --flags -l specific_label is used, got %q", hit)
	}
}
