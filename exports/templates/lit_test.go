package templates_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	lua "github.com/taybart/midnight"
	"github.com/taybart/rest/exports/templates"
)

// a body with everything that tends to break literals: quotes of every kind,
// backslashes, control characters, a lua keyword and non-identifier keys,
// unicode, nesting and every json scalar type. null and empty containers are
// left out because lua can't round trip them (nil drops the key, {} is
// ambiguous).
const litBody = `{
  "str": "it's \"quoted\" ` + "`tick`" + ` \\ back $HOME ${x} \n\t\r \u0000\u001b\u007f 💥   <a>&amp;",
  "end": "lua keyword key",
  "with space": 1,
  "1starts_with_digit": -2.5,
  "quote\"key": true,
  "nested": {"list": [1, "two", false, {"deep": "\\d+"}], "big": 12345678901234567890, "exp": 1e-7}
}`

func litRequest() templates.Request {
	return templates.Request{
		Label:          "lit",
		Method:         "POST",
		BaseURL:        "http://x",
		RequestHeaders: map[string]string{"Content-Type": "application/json"},
		Body:           strings.ReplaceAll(litBody, "\n", ""),
	}
}

func wantBody(t *testing.T) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(litBody), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

// renderFile writes src to a template named file and renders it
func renderFile(t *testing.T, file, src string, req templates.Request) (string, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), file)
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	tmpl, err := templates.Get(path)
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	err = tmpl.Execute(&out, []templates.Request{req})
	return out.String(), err
}

// both the one line and pretty forms have to be valid
var litForms = map[string]string{
	"inline": `{{lit .JSON}}`,
	"pretty": `{{lit .JSON "  "}}`,
}

func assertJSONEqual(t *testing.T, form string, gotJSON []byte, want any) {
	t.Helper()
	var got any
	if err := json.Unmarshal(gotJSON, &got); err != nil {
		t.Fatalf("%s: %v\n%s", form, err, gotJSON)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s: round trip mismatch\ngot:  %#v\nwant: %#v", form, got, want)
	}
}

func TestLitLua(t *testing.T) {
	want := wantBody(t)
	for form, expr := range litForms {
		out, err := renderFile(t, "x.lua.tmpl", "return "+expr, litRequest())
		if err != nil {
			t.Fatal(err)
		}
		L := lua.NewState()
		defer L.Close()
		if err := L.DoString(out); err != nil {
			t.Fatalf("%s: lua rejected the literal: %v\n%s", form, err, out)
		}
		got, _ := json.Marshal(fromLua(L.Get(-1)))
		assertJSONEqual(t, form, got, want)
	}
}

// fromLua converts back to json shaped go values, a table with keys 1..n is
// a list
func fromLua(v lua.LValue) any {
	switch v := v.(type) {
	case lua.LString:
		return string(v)
	case lua.LNumber:
		return float64(v)
	case lua.LBool:
		return bool(v)
	case *lua.LTable:
		if n := v.Len(); n > 0 {
			list := make([]any, 0, n)
			for i := 1; i <= n; i++ {
				list = append(list, fromLua(v.RawGetInt(i)))
			}
			return list
		}
		m := map[string]any{}
		v.ForEach(func(k, val lua.LValue) { m[k.String()] = fromLua(val) })
		return m
	}
	return nil
}

func TestLitJS(t *testing.T) {
	node := lookPath(t, "node")
	want := wantBody(t)
	for form, expr := range litForms {
		out, err := renderFile(t, "x.js.tmpl", "process.stdout.write(JSON.stringify("+expr+"))", litRequest())
		if err != nil {
			t.Fatal(err)
		}
		got, err := exec.Command(node, "-e", out).CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v\n%s\n%s", form, err, got, out)
		}
		assertJSONEqual(t, form, got, want)
	}
}

func TestLitGo(t *testing.T) {
	gobin := lookPath(t, "go")
	want := wantBody(t)
	for form, expr := range litForms {
		src := `package main

import (
	"encoding/json"
	"os"
)

func main() {
	v := ` + expr + `
	b, _ := json.Marshal(v)
	os.Stdout.Write(b)
}
`
		out, err := renderFile(t, "main.go.tmpl", src, litRequest())
		if err != nil {
			t.Fatal(err)
		}
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(out), 0o644); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(gobin, "run", "main.go")
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=")
		got, err := cmd.Output()
		if err != nil {
			t.Fatalf("%s: %v\n%s", form, err, out)
		}
		// go decodes the 20 digit number into a float64, compare the same way
		assertJSONEqual(t, form, got, want)
	}
}

// strings in every language, including the raw string path in go
func TestLitStrings(t *testing.T) {
	var body map[string]any
	_ = json.Unmarshal([]byte(litBody), &body)
	nasty := body["str"].(string)
	req := templates.Request{Label: "s", Method: "GET", BaseURL: "http://x", Body: nasty}

	t.Run("sh", func(t *testing.T) {
		// a shell argument can't hold NUL, lit refuses it (see TestLitErrors)
		req := req
		req.Body = strings.ReplaceAll(nasty, "\x00", "")
		nasty := req.Body
		out, err := renderFile(t, "x.sh.tmpl", `printf %s {{lit .Body}}`, req)
		if err != nil {
			t.Fatal(err)
		}
		got, err := exec.Command("sh", "-c", out).Output()
		if err != nil || string(got) != nasty {
			t.Fatalf("got %q err %v\n%s", got, err, out)
		}
	})
	t.Run("lua", func(t *testing.T) {
		out, err := renderFile(t, "x.lua.tmpl", `return {{lit .Body}}`, req)
		if err != nil {
			t.Fatal(err)
		}
		L := lua.NewState()
		defer L.Close()
		if err := L.DoString(out); err != nil {
			t.Fatal(err)
		}
		if got := L.Get(-1).String(); got != nasty {
			t.Fatalf("got %q want %q", got, nasty)
		}
	})
	t.Run("go raw string", func(t *testing.T) {
		// json bodies are the common case, they should come out raw
		r := req
		r.Body = `{"a":"b\\c"}`
		out, err := renderFile(t, "x.go.tmpl", `{{lit .Body}}`, r)
		if err != nil || out != "`{\"a\":\"b\\\\c\"}`\n" {
			t.Fatalf("got %q err %v", out, err)
		}
	})
}

func TestLitLuaKeys(t *testing.T) {
	req := templates.Request{
		Label: "k", Method: "GET", BaseURL: "http://x",
		RequestHeaders: map[string]string{"X-Api-Key": "k", "plain": "p", "end": "e"},
	}
	out, err := renderFile(t, "x.lua.tmpl", `{{lit .RequestHeaders}}`, req)
	if err != nil {
		t.Fatal(err)
	}
	want := `{ ["X-Api-Key"] = "k", ["end"] = "e", plain = "p" }` + "\n"
	if out != want {
		t.Fatalf("got %q want %q", out, want)
	}
}

func TestLitErrors(t *testing.T) {
	req := litRequest()
	if _, err := renderFile(t, "nolang.tmpl", `{{lit .URL}}`, req); err == nil ||
		!strings.Contains(err.Error(), "foo.<lang>.tmpl") {
		t.Fatalf("expected a hint about naming the template, got %v", err)
	}
	if _, err := renderFile(t, "x.sh.tmpl", `{{lit .Headers}}`, req); err == nil {
		t.Fatal("expected sh to refuse a map")
	}
	nul := templates.Request{Label: "n", Method: "GET", BaseURL: "http://x", Body: "a\x00b"}
	if _, err := renderFile(t, "x.sh.tmpl", `{{lit .Body}}`, nul); err == nil {
		t.Fatal("expected sh to refuse a NUL byte")
	}
	// templates without lit don't need a language
	if _, err := renderFile(t, "nolang.tmpl", `{{.URL}}`, req); err != nil {
		t.Fatal(err)
	}
}
