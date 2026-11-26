package file_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/taybart/rest"
)

// fakeOp puts an `op` on PATH that knows one reference and logs each call to
// the returned file
func fakeOp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	calls := filepath.Join(dir, "calls")
	script := `#!/bin/sh
echo "$@" >> "` + calls + `"
case "$3" in
  op://vault/item/*) printf 'hunter2' ;;
  *) echo '[ERROR] "item" isn'"'"'t an item in the "vault" vault' >&2; exit 1 ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "op"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return calls
}

func TestOpRead(t *testing.T) {
	calls := fakeOp(t)
	dir := writeFiles(t, map[string]string{"op.rest": `
locals {
  password = op::read("op://vault/item/password")
}
request "op" {
  url = "http://localhost:18080/op"
  method = "GET"
  headers = {
    "x-local"  = locals.password
    "x-direct" = op::read("op://vault/item/password")
  }
}
`})
	r, err := rest.NewFile(filepath.Join(dir, "op.rest"))
	if err != nil {
		t.Fatal(err)
	}
	req, err := r.Request("op")
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range []string{"x-local", "x-direct"} {
		if req.Headers[h] != "hunter2" {
			t.Fatalf("header %s: expected %q, got %q", h, "hunter2", req.Headers[h])
		}
	}
	log, err := os.ReadFile(calls)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(log)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected op to be called once (cached), got %d: %q", len(lines), lines)
	}
	if lines[0] != "read --no-newline op://vault/item/password" {
		t.Fatalf("unexpected op args: %q", lines[0])
	}
}

func TestOpReadErrors(t *testing.T) {
	fakeOp(t)
	tests := map[string]string{
		`op::read("vault/item/password")`:  "secret reference",
		`op::read("op://vault/missing/x")`: "isn't an item",
	}
	for expr, want := range tests {
		src := `
request "op" {
  url = "http://localhost:18080/op"
  headers = { "x-secret" = ` + expr + ` }
}
`
		var diags bytes.Buffer
		r, err := rest.NewSource("op.rest", []byte(src), &diags)
		if err == nil {
			_, err = r.Request("op")
		}
		if err == nil {
			t.Fatalf("%s: expected an error", expr)
		}
		if !strings.Contains(diags.String(), want) {
			t.Fatalf("%s: expected diagnostics containing %q, got:\n%s", expr, want, diags.String())
		}
	}
}

func TestOpReadAccount(t *testing.T) {
	calls := fakeOp(t)
	src := `
request "op" {
  url = "http://localhost:18080/op"
  headers = {
    "x-default" = op::read("op://vault/item/password")
    "x-account" = op::read("op://vault/item/password", "work.1password.com")
  }
}
`
	r, err := rest.NewSource("op.rest", []byte(src), &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Request("op"); err != nil {
		t.Fatal(err)
	}
	log, err := os.ReadFile(calls)
	if err != nil {
		t.Fatal(err)
	}
	// the account is part of the cache key, so both are read
	want := "read --no-newline op://vault/item/password --account work.1password.com"
	if !strings.Contains(string(log), want+"\n") {
		t.Fatalf("expected a call with %q, got:\n%s", want, log)
	}
}

// env's default is only evaluated when the variable is unset, so op isn't
// called when it's overridden
func TestEnvDefaultIsLazy(t *testing.T) {
	calls := fakeOp(t)
	src := `
locals {
  prod     = true
  password = env("REST_TEST_OP_PASSWORD", locals.prod ?
    op::read("op://vault/missing/password") : "admin"
  )
}
request "op" {
  url = "http://localhost:18080/op"
  headers = { "x-password" = locals.password }
}
`
	t.Setenv("REST_TEST_OP_PASSWORD", "from-env")
	r, err := rest.NewSource("op.rest", []byte(src), &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	req, err := r.Request("op")
	if err != nil {
		t.Fatal(err)
	}
	if req.Headers["x-password"] != "from-env" {
		t.Fatalf("expected %q, got %q", "from-env", req.Headers["x-password"])
	}
	if _, err := os.Stat(calls); !os.IsNotExist(err) {
		t.Fatal("op was called even though the env var was set")
	}

	// unset, the default runs (and its error comes through)
	t.Setenv("REST_TEST_OP_PASSWORD", "")
	var diags bytes.Buffer
	if _, err := rest.NewSource("op.rest", []byte(src), &diags); err == nil {
		t.Fatal("expected the default's op error")
	}
	if !strings.Contains(diags.String(), "isn't an item") {
		t.Fatalf("expected op's error in diagnostics, got:\n%s", diags.String())
	}
}

// `? :` only evaluates the branch it picks, so op isn't called locally
func TestConditionalIsLazy(t *testing.T) {
	calls := fakeOp(t)
	src := `
locals {
  prod     = false
  password = env("REST_TEST_OP_UNSET", locals.prod ?
    op::read("op://vault/item/lazy0") : "admin"
  )
  nested = locals.prod ? "a" : (true ? "b" : op::read("op://vault/item/lazy1"))
  number = locals.prod ? 1 : 2
  object = !locals.prod ? { a = 1 } : { a = op::read("op://vault/item/lazy2") }
}
request "op" {
  url = "http://localhost:18080/op"
  headers = {
    "x-password" = locals.password
    "x-nested"   = locals.nested
    "x-number"   = locals.number
    "x-object"   = locals.object.a
    "x-prod"     = !locals.prod ? op::read("op://vault/item/lazy3") : "local"
  }
}
`
	r, err := rest.NewSource("op.rest", []byte(src), &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	req, err := r.Request("op")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"x-password": "admin", "x-nested": "b", "x-number": "2", "x-object": "1",
		"x-prod": "hunter2",
	}
	for k, v := range want {
		if req.Headers[k] != v {
			t.Fatalf("header %s: expected %q, got %q", k, v, req.Headers[k])
		}
	}
	// only x-prod's branch calls op
	log, _ := os.ReadFile(calls)
	if n := strings.Count(string(log), "\n"); n != 1 {
		t.Fatalf("expected op to be called once, got %d:\n%s", n, log)
	}
}
