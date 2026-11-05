package rest_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/taybart/rest"
)

const exportFile = `
request "first" {
  url = "http://localhost/a?keep=1"
  query = { add = "2" }
  basic_auth = "user:p:ss"
  expect = 201
}
request "second" {
  url = "http://localhost/b"
  method = "post"
  bearer_token = "tok"
  body = { hello = "world" }
}
request "third" {
  url = "http://localhost/c"
}
`

func exportOut(t *testing.T, label string, block int) (string, error) {
	t.Helper()
	filename := filepath.Join(t.TempDir(), "export.rest")
	if err := os.WriteFile(filename, []byte(exportFile), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := rest.NewFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err = r.ExportTo(&out, "curl", label, block)
	return out.String(), err
}

func TestExportWholeFileInOrder(t *testing.T) {
	// run a few times, map iteration order used to leak into output order
	for range 5 {
		out, err := exportOut(t, "", -1)
		if err != nil {
			t.Fatal(err)
		}
		a, b, c := strings.Index(out, "# first"), strings.Index(out, "# second"), strings.Index(out, "# third")
		if a < 0 || !(a < b && b < c) {
			t.Fatalf("requests out of order:\n%s", out)
		}
	}
}

func TestExportFlattensRequest(t *testing.T) {
	out, err := exportOut(t, "", -1)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`'http://localhost/a?add=2&keep=1'`,
		// password may contain ':'
		`-u 'user:p:ss'`,
		`curl -X POST 'http://localhost/b'`,
		`--oauth2-bearer 'tok'`,
		`--data-raw '{"hello":"world"}'`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in:\n%s", want, out)
		}
	}
}

func TestExportFilters(t *testing.T) {
	out, err := exportOut(t, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "# second") || strings.Contains(out, "# first") || strings.Contains(out, "# third") {
		t.Fatalf("block filter failed:\n%s", out)
	}

	out, err = exportOut(t, "third", -1)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "# third") || strings.Count(out, "curl") != 1 {
		t.Fatalf("label filter failed:\n%s", out)
	}

	if _, err := exportOut(t, "missing", -1); err == nil {
		t.Fatal("expected error for missing label")
	}
	if _, err := exportOut(t, "", 9); err == nil {
		t.Fatal("expected error for missing block")
	}
}

func TestExportTemplateParts(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "export.rest")
	if err := os.WriteFile(filename, []byte(exportFile), 0o644); err != nil {
		t.Fatal(err)
	}
	tmpl := filepath.Join(dir, "parts.tmpl")
	// a python requests style template that wants the parts, not the url/headers rest sends
	src := `{{.Method}} {{.BaseURL}} params={{range $k, $v := .Query}}{{$k}}={{index $v 0}},{{end}}` +
		` auth={{.Auth.Type}}:{{.Auth.User}}:{{.Auth.Password}}{{.Auth.Token}}` +
		` sent={{.Headers.Authorization}}`
	if err := os.WriteFile(tmpl, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := rest.NewFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := r.ExportTo(&out, tmpl, "", -1); err != nil {
		t.Fatal(err)
	}
	want := "GET http://localhost/a params=add=2,keep=1, auth=basic:user:p:ss sent=Basic dXNlcjpwOnNz\n\n" +
		"POST http://localhost/b params= auth=bearer::tok sent=Bearer tok\n\n" +
		"GET http://localhost/c params= auth=:: sent=\n"
	if out.String() != want {
		t.Fatalf("got:\n%q\nwant:\n%q", out.String(), want)
	}
}

func TestExportSkip(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "skip.rest")
	src := `
request "base" {
  url = "http://localhost/base"
  headers = { "X-Base" = "1" }
  skip = true
}
request "child" {
  copy_from = "base"
  url = "http://localhost/child"
}
`
	if err := os.WriteFile(filename, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := rest.NewFile(filename)
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := r.ExportTo(&out, "curl", "", -1); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "# base") || !strings.Contains(out.String(), "# child") {
		t.Fatalf("expected only child exported:\n%s", out.String())
	}
	// copy_from still pulls from the skipped base
	if !strings.Contains(out.String(), "X-Base: 1") {
		t.Fatalf("expected child to inherit base headers:\n%s", out.String())
	}

	if err := r.ExportTo(&out, "curl", "base", -1); err == nil || !strings.Contains(err.Error(), "skip") {
		t.Fatalf("expected skip error for -l base, got %v", err)
	}
	if err := r.ExportTo(&out, "curl", "", 0); err == nil || !strings.Contains(err.Error(), "skip") {
		t.Fatalf("expected skip error for -b 0, got %v", err)
	}
}
