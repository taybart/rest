package main

import (
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

	os.Args = []string{"rest", "-d", "/custom/dir", "-r", "--script=app.sh"}
	vals, help, err := parseCLIFlagsWithArgs(flags, nil)
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
	vals, help, err := parseCLIFlagsWithArgs(flags, nil)
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
	_, help, err = parseCLIFlagsWithArgs(flags, nil)
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
	_, _, err := parseCLIFlagsWithArgs(flags, reserved)
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
    f:write((cli.dir or "") .. ":" .. tostring(cli.restart == true))
    f:close()
  LUA
}
`
	if err := os.WriteFile(scriptFile, []byte(content), 0755); err != nil {
		t.Fatal(err)
	}

	os.Args = []string{"rest", scriptFile, "-d", "/my/dir", "-r"}
	if err := run(); err != nil {
		t.Fatalf("run() failed: %v", err)
	}

	got, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "/my/dir:true" {
		t.Fatalf("expected /my/dir:true, got %q", string(got))
	}
}
