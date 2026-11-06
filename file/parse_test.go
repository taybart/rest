package file_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/taybart/rest"
	"github.com/taybart/rest/request"
)

func parse(t *testing.T, filename string, expectedReqs int) *rest.Rest {
	rest, err := rest.NewFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if len(rest.Requests) != expectedReqs {
		t.Fatalf("expected %d request(s), got %d", expectedReqs, len(rest.Requests))
	}
	return rest
}

func TestBasicParse(t *testing.T) {
	rest := parse(t, "../doc/examples/client/basic.rest", 1)

	basic := request.Request{
		Label:   "basic",
		URL:     "http://localhost:18080/__echo__",
		Method:  "POST",
		Headers: map[string]string{"Content-Type": "application/json"},
		Body:    `{"data":"hello world"}`,
	}

	built, err := rest.Request("basic")
	if err != nil {
		t.Fatal(err)
	}
	if !built.Equal(basic) {
		t.Fatalf("expected request to match:\n%s\n%s\n", basic, built)
	}

}

func TestTemplateParse(t *testing.T) {
	rest := parse(t, "../doc/examples/client/template.rest", 1)

	templated := request.Request{
		Label:   "template",
		URL:     "http://localhost:18080/hello-world",
		Method:  "POST",
		Headers: map[string]string{"Content-Type": "application/json"},
		Body:    `{"myName":"foobah"}`,
	}

	built, err := rest.Request("template")
	if err != nil {
		t.Fatal(err)
	}
	if !built.Equal(templated) {
		t.Fatalf("expected request to match:\n%s\n%s\n", templated, built)
	}

}
func TestImportParse(t *testing.T) {
	rest := parse(t, "../doc/examples/client/import.rest", 2)
	built, err := rest.Request("test")
	if err != nil {
		t.Fatal(err)
	}
	headers := built.Headers
	if headers["X-imported-header"] != "success" {
		t.Fatal(`unexpected import result expected "success" got:`,
			headers["X-imported-header"])
	}
	if headers["X-imported-local"] != "success" {
		t.Fatal(`unexpected import result expected "success" got:`,
			headers["X-imported-local"])
	}
}
func TestSocketParse(t *testing.T) {
	rest := parse(t, "../doc/examples/client/socket.rest", 0)
	socket, err := rest.Parser.Socket()
	if err != nil {
		t.Fatal(err)
	}
	if len(socket.Playbook) != 3 {
		t.Fatal("expected 3 socket plays, got", len(socket.Playbook))
	}
	if len(socket.Run.Order) != 4 {
		t.Fatal("expected 4 socket calls in run, got", len(socket.Run.Order))
	}
}

func TestSocketRawParse(t *testing.T) {
	rest := parse(t, "../doc/examples/client/socket_raw.rest", 0)
	socket, err := rest.Parser.Socket()
	if err != nil {
		t.Fatal(err)
	}
	expected := map[string]string{
		// raw() sends the string verbatim, interpolation still resolves
		"connect": "CONNECT abc123",
		// objects are still json encoded
		"sub": `{"channel":"#general","msg":"sub"}`,
		// strings that are already json are still used as-is
		"ping": `{"msg":"ping"}`,
		// plain strings are still json encoded
		"hi": `"hello"`,
	}
	if len(socket.Playbook) != len(expected) {
		t.Fatalf("expected %d socket plays, got %d", len(expected), len(socket.Playbook))
	}
	for k, want := range expected {
		if socket.Playbook[k] != want {
			t.Fatalf("playbook %q: expected %s got %s", k, want, socket.Playbook[k])
		}
	}
}

func TestServerParse(t *testing.T) {
	// TODO: test all fields
	rest := parse(t, "../doc/examples/server/basic.rest", 0)
	config, err := rest.Parser.Server()
	if err != nil {
		t.Fatal(err)
	}
	if config.Addr != "localhost:18080" {
		t.Fatal("expected address localhost:18080, got", config.Addr)
	}
	if config.Response.Status != 418 {
		t.Fatal("expected response statuscode to be", 418, "got", config.Response.Status)
	}
	if string(config.Response.Body) != `` {
		t.Fatal("expected body to be empty got:", string(config.Response.Body))
	}
}

func TestShebangParse(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "shebang.rest")
	content := `#!/usr/bin/env rest

request "shebang" {
  url = "http://localhost:18080/"
  method = "GET"
}
`
	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	r, err := rest.NewFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(r.Requests))
	}
	req, err := r.Request("shebang")
	if err != nil {
		t.Fatal(err)
	}
	if req.URL != "http://localhost:18080/" {
		t.Fatalf("unexpected url: %s", req.URL)
	}
}

func TestNoFlagsAndCLIParse(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "cli.rest")
	content := `#!/usr/bin/env rest

config {
  no_flags = true
}

cli {
  flags = {
    dir = {
      desc = "Directory"
      short = "d"
      default = "/tmp"
      required = true
    }
    restart = {
      desc = "Restart"
      short = "r"
      bool = true
    }
  }
  fn = "print('ok')"
}
`
	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	r, err := rest.NewFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Parser.Config.NoFlags {
		t.Fatal("expected config.no_flags to be true")
	}

	cli, err := r.Parser.CLI()
	if err != nil {
		t.Fatal(err)
	}
	dirFlag, ok := cli.Flags["dir"]
	if !ok {
		t.Fatal("expected dir flag")
	}
	if dirFlag.Short != "d" || dirFlag.Default != "/tmp" || !dirFlag.Required || dirFlag.Desc != "Directory" {
		t.Fatalf("unexpected dir flag: %+v", dirFlag)
	}

	restartFlag, ok := cli.Flags["restart"]
	if !ok {
		t.Fatal("expected restart flag")
	}
	if restartFlag.Short != "r" || !restartFlag.Bool || restartFlag.Desc != "Restart" {
		t.Fatalf("unexpected restart flag: %+v", restartFlag)
	}
}

func TestTryExportsArrayAndTypes(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "exports.rest")
	content := `request "restart" {
  url = "http://localhost:18080/restart"
  method = "POST"
  body = {
    dir = try_exports("dir", "/default/dir")
    args = try_exports("args", [])
  }
}
`
	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	r, err := rest.NewFile(filename)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Default values (empty array)
	req, err := r.Request("restart")
	if err != nil {
		t.Fatal(err)
	}
	if req.Body != `{"args":[],"dir":"/default/dir"}` {
		t.Fatalf("unexpected default body: %s", req.Body)
	}

	// 2. With exported array
	r.Parser.AddExportsCtx(map[string]any{
		"dir":  "/custom/dir",
		"args": []any{"one", "two"},
	})
	req, err = r.Request("restart")
	if err != nil {
		t.Fatal(err)
	}
	if req.Body != `{"args":["one","two"],"dir":"/custom/dir"}` {
		t.Fatalf("unexpected exported body: %s", req.Body)
	}
}

func TestFormHeaderBodyParse(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "form.rest")
	content := `request "form_auto" {
  url = "http://localhost:18080/login"
  method = "POST"
  headers = {
    "Content-Type" = "application/x-www-form-urlencoded"
  }
  body = {
    username = "alice"
    count    = 2
    tags     = ["a", "b"]
  }
}

request "json_default" {
  url = "http://localhost:18080/login"
  method = "POST"
  body = {
    username = "alice"
  }
}
`
	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	r, err := rest.NewFile(filename)
	if err != nil {
		t.Fatal(err)
	}

	formReq, err := r.Request("form_auto")
	if err != nil {
		t.Fatal(err)
	}
	if formReq.Body != "count=2&tags=a&tags=b&username=alice" {
		t.Fatalf("unexpected form body: %s", formReq.Body)
	}

	jsonReq, err := r.Request("json_default")
	if err != nil {
		t.Fatal(err)
	}
	if jsonReq.Body != `{"username":"alice"}` {
		t.Fatalf("unexpected json body: %s", jsonReq.Body)
	}
	if jsonReq.Headers["Content-Type"] != "application/json" {
		t.Fatalf("expected default Content-Type application/json, got: %q", jsonReq.Headers["Content-Type"])
	}
}

func TestDefaultContentTypeConfig(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "no_default_ct.rest")
	content := `config {
  default_content_type = false
}

request "json_no_ct" {
  url    = "http://localhost:18080/login"
  method = "POST"
  body   = {
    username = "alice"
  }
}
`
	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	r, err := rest.NewFile(filename)
	if err != nil {
		t.Fatal(err)
	}

	req, err := r.Request("json_no_ct")
	if err != nil {
		t.Fatal(err)
	}
	if req.Body != `{"username":"alice"}` {
		t.Fatalf("unexpected json body: %s", req.Body)
	}
	if _, ok := req.Headers["Content-Type"]; ok {
		t.Fatalf("expected no Content-Type header when default_content_type = false, got: %v", req.Headers)
	}
}

func TestServerKVAndSetupParse(t *testing.T) {
	r, err := rest.NewFile("../doc/examples/server/handler_fns.rest")
	if err != nil {
		t.Fatal(err)
	}
	srv, err := r.Parser.Server()
	if err != nil {
		t.Fatal(err)
	}
	db, ok := srv.KV["db"].(map[string]any)
	if !ok {
		t.Fatalf("expected db map in server KV, got %#v", srv.KV)
	}
	users, ok := db["users"].(map[string]any)
	if !ok {
		t.Fatalf("expected users map in db, got %#v", db["users"])
	}
	u4, ok := users["4"].(map[string]any)
	if !ok || u4["name"] != "Frankie Muniz" {
		t.Fatalf("expected user 4 to be Frankie Muniz, got %#v", users["4"])
	}
}

func TestLibAndHookFuncsParse(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "hooks.rest")
	content := `request "precanned" {
  url   = "http://localhost:18080/test"
  after = lib.dump
}

request "inspect_fn" {
  url   = "http://localhost:18080/test"
  after = lib::inspect_res("id", "name")
}

request "combined_list" {
  url   = "http://localhost:18080/test"
  after = [
    lib.expected,
    lib::inspect_res(["id", "name"]),
    lib::export_res({ user_id = "user.id" }),
  ]
}
`
	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	r, err := rest.NewFile(filename)
	if err != nil {
		t.Fatal(err)
	}

	precanned, err := r.Request("precanned")
	if err != nil {
		t.Fatal(err)
	}
	if precanned.After != "print(rest.req.dump..'\\n')\nprint(rest.res.dump)" {
		t.Fatalf("unexpected precanned after: %q", precanned.After)
	}

	inspectReq, err := r.Request("inspect_fn")
	if err != nil {
		t.Fatal(err)
	}
	if inspectReq.After != `tools.inspect_res(json.decode('"id"'), json.decode('"name"'))` {
		t.Fatalf("unexpected inspect_fn after: %q", inspectReq.After)
	}

	combined, err := r.Request("combined_list")
	if err != nil {
		t.Fatal(err)
	}
	wantCombined := "tools.expected()\n" +
		`tools.inspect_res(json.decode('["id","name"]'))` + "\n" +
		`tools.export_res(json.decode('{"user_id":"user.id"}'))`
	if combined.After != wantCombined {
		t.Fatalf("unexpected combined after:\ngot:  %q\nwant: %q", combined.After, wantCombined)
	}
}

func TestJWTFunctionsAndExpectParse(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "jwt.rest")
	content := `locals {
  token = jwt_sign({ sub = "user-42", role = "admin" }, "my-secret")
}

request "admin" {
  url          = "http://localhost:18080/users/${jwt_dec(locals.token).sub}"
  bearer_token = locals.token
  body = {
    role = jwt_dec(locals.token).role
  }
  expect {
    status = 200
    body = {
      token = is.jwt
    }
  }
  after = [
    lib.expected,
    lib.inspect_jwt,
  ]
}
`
	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	r, err := rest.NewFile(filename)
	if err != nil {
		t.Fatal(err)
	}

	req, err := r.Request("admin")
	if err != nil {
		t.Fatal(err)
	}
	if req.URL != "http://localhost:18080/users/user-42" {
		t.Fatalf("unexpected url: %s", req.URL)
	}
	if req.Body != `{"role":"admin"}` {
		t.Fatalf("unexpected body: %s", req.Body)
	}
	if err := request.MatchExpectedBody(req.Expect.Body, `{"token":"`+req.BearerToken+`"}`, request.BodyOptions{}); err != nil {
		t.Fatalf("expected is.jwt matcher to pass, got: %v", err)
	}
	if err := request.MatchExpectedBody(req.Expect.Body, `{"token":"not-a-jwt"}`, request.BodyOptions{}); err == nil {
		t.Fatal("expected is.jwt matcher to fail on invalid jwt")
	}
}

func TestHeredocLangHints(t *testing.T) {
	const scripts = `
request "erde" {
  url   = "http://localhost:18080"
  after = <<ERDE
    print("{rest.res.status}")
  ERDE
}

request "indented_lua" {
  url   = "http://localhost:18080"
  after = <<-LUA
    print(rest.res.status)
  LUA
}

request "plain" {
  url   = "http://localhost:18080"
  after = "print(rest.res.status)"
}

request "none" {
  url = "http://localhost:18080"
}

request "list" {
  url   = "http://localhost:18080"
  after = [
    lib.dump,
    <<ERDE
      print("{rest.res.status}")
    ERDE
  ]
}

server {
  address = "localhost:18080"
  setup = <<ERDE
    print("setup")
  ERDE
  middleware "auth" {
    fn = <<LUA
      print("auth")
    LUA
  }
  handler "GET" "/" {
    fn = <<ERDE
      return { status = 200 }
    ERDE
  }
  handler "GET" "/plain" {
    fn = "return { status = 200 }"
  }
  handler "GET" "/read_erde" {
    fn = read("{{dir}}/script.erde")
  }
  handler "GET" "/read_lua" {
    fn = read("{{dir}}/script.lua")
  }
}

cli {
  fn = <<ERDE
    print("fn")
  ERDE
  loop = "print(input)"
}
`
	tests := []struct {
		name  string
		reqs  map[string]string
		setup string
		mw    string
		hdl   []string
		fn    string
		loop  string
	}{
		{
			name: "hints",
			reqs: map[string]string{
				"erde": "erde", "indented_lua": "lua", "plain": "lua", "none": "lua", "list": "erde",
			},
			setup: "erde", mw: "lua", hdl: []string{"erde", "lua", "erde", "lua"}, fn: "erde", loop: "lua",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, src := range map[string]string{"script.erde": "return { status = 200 }", "script.lua": "return { status = 200 }"} {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0644); err != nil {
					t.Fatal(err)
				}
			}
			filename := filepath.Join(dir, "hints.rest")
			if err := os.WriteFile(filename, []byte(strings.ReplaceAll(scripts, "{{dir}}", dir)), 0644); err != nil {
				t.Fatal(err)
			}
			r, err := rest.NewFile(filename)
			if err != nil {
				t.Fatal(err)
			}
			for label, want := range tt.reqs {
				req, err := r.Request(label)
				if err != nil {
					t.Fatal(err)
				}
				if req.Lang != want {
					t.Errorf("request %s: expected lang %q, got %q", label, want, req.Lang)
				}
			}

			serv, err := r.Parser.Server()
			if err != nil {
				t.Fatal(err)
			}
			if serv.SetupLang != tt.setup {
				t.Errorf("setup: expected lang %q, got %q", tt.setup, serv.SetupLang)
			}
			if serv.Middlewares[0].Lang != tt.mw {
				t.Errorf("middleware: expected lang %q, got %q", tt.mw, serv.Middlewares[0].Lang)
			}
			for i, want := range tt.hdl {
				if serv.Handlers[i].Lang != want {
					t.Errorf("handler %d: expected lang %q, got %q", i, want, serv.Handlers[i].Lang)
				}
			}

			cli, err := r.Parser.CLI()
			if err != nil {
				t.Fatal(err)
			}
			if cli.FnLang != tt.fn || cli.LoopLang != tt.loop {
				t.Errorf("cli: expected fn/loop %q/%q, got %q/%q", tt.fn, tt.loop, cli.FnLang, cli.LoopLang)
			}
		})
	}
}

func TestEnvDefault(t *testing.T) {
	t.Setenv("REST_TEST_ENV_SET", "from-env")
	t.Setenv("REST_TEST_ENV_EMPTY", "")
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "env.rest")
	content := `request "env" {
  url = "http://localhost:18080/env"
  method = "GET"
  headers = {
    "x-set"      = env("REST_TEST_ENV_SET", "fallback")
    "x-empty"    = env("REST_TEST_ENV_EMPTY", "fallback")
    "x-unset"    = env("REST_TEST_ENV_UNSET", "fallback")
    "x-no-default" = env("REST_TEST_ENV_SET")
  }
}
`
	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	r, err := rest.NewFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	req, err := r.Request("env")
	if err != nil {
		t.Fatal(err)
	}
	expected := map[string]string{
		"x-set":        "from-env",
		"x-empty":      "fallback",
		"x-unset":      "fallback",
		"x-no-default": "from-env",
	}
	for k, v := range expected {
		if req.Headers[k] != v {
			t.Fatalf("header %s: expected %q, got %q", k, v, req.Headers[k])
		}
	}
}

func writeFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestLocalsReferenceOrder(t *testing.T) {
	dir := writeFiles(t, map[string]string{"locals.rest": `
locals {
  pet  = { id = is.int, category = schemas.category, n = locals.a }
  zzz  = "z"
  path = "${locals.base}/pets/${locals.zzz}"
}
schemas {
  category = { tags = is::array_of(schemas.tag), a = locals.a }
}
locals {
  a = 1
}
schemas {
  tag = { name = is.string }
}
locals {
  base = "http://x"
}
request "r" {
  url = locals.path
  headers = { n = "${locals.pet.n}" }
}
`})
	// evaluation order used to follow map iteration, so run it a few times
	for range 20 {
		r, err := rest.NewFile(filepath.Join(dir, "locals.rest"))
		if err != nil {
			t.Fatal(err)
		}
		req, err := r.Request("r")
		if err != nil {
			t.Fatal(err)
		}
		if req.URL != "http://x/pets/z" || req.Headers["n"] != "1" {
			t.Fatalf("got url %q n %q", req.URL, req.Headers["n"])
		}
		if _, ok := r.Parser.Schemas["category"]; !ok {
			t.Fatal("schemas.category wasn't set")
		}
	}
}

func TestLocalsErrors(t *testing.T) {
	for name, content := range map[string]string{
		"cycle": `
locals {
  a = locals.b
}
schemas {
  b = { x = locals.a }
}`,
		"duplicate in one file": `
locals {
  a = 1
}
locals {
  a = 2
}`,
	} {
		dir := writeFiles(t, map[string]string{"x.rest": content})
		if _, err := rest.NewFile(filepath.Join(dir, "x.rest")); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestLocalsImportOverrides(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		// the library uses url without defining it, and defines a default
		// the importer overrides
		"lib.rest": `
locals {
  user = "lib"
  path = "${locals.url}/users/${locals.user}"
  only = "from lib"
}
request "get" {
  url = locals.path
  headers = { only = locals.only, layer = locals.layer }
}`,
		"lib2.rest": `
locals {
  layer = "lib2"
}`,
		"lib3.rest": `
locals {
  layer = "lib3"
}`,
		"main.rest": `
imports = ["./lib.rest", "./lib2.rest", "./lib3.rest"]
locals {
  url  = "http://api"
  user = "main"
}`,
	})
	r, err := rest.NewFile(filepath.Join(dir, "main.rest"))
	if err != nil {
		t.Fatal(err)
	}
	req, err := r.Request("lib::get")
	if err != nil {
		t.Fatal(err)
	}
	if req.URL != "http://api/users/main" {
		t.Errorf("importer should define url and override user: %s", req.URL)
	}
	if req.Headers["only"] != "from lib" {
		t.Errorf("lib's own locals should still work: %v", req.Headers)
	}
	if req.Headers["layer"] != "lib3" {
		t.Errorf("a later import should override an earlier one: %v", req.Headers)
	}
}

func TestImportNamespacesEachRequestOnce(t *testing.T) {
	dir := t.TempDir()
	lib := `
request "one" { url = "http://x/1" }
request "two" { url = "http://x/2" }
request "three" { url = "http://x/3" }
`
	main := `
imports = ["./lib.rest"]
request "main" { url = "http://x/main" }
`
	if err := os.WriteFile(filepath.Join(dir, "lib.rest"), []byte(lib), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.rest"), []byte(main), 0644); err != nil {
		t.Fatal(err)
	}
	r, err := rest.NewFile(filepath.Join(dir, "main.rest"))
	if err != nil {
		t.Fatal(err)
	}
	// used to come out as lib::lib::lib::one, one prefix per imported request
	for _, label := range []string{"main", "lib::one", "lib::two", "lib::three"} {
		if _, ok := r.Requests[label]; !ok {
			t.Errorf("missing %s in %v", label, r.Requests)
		}
	}
	if len(r.Requests) != 4 {
		t.Errorf("expected 4 requests, got %d", len(r.Requests))
	}
}

func TestImportPathResolution(t *testing.T) {
	lib := `
locals { who = "lib" }
request "one" { url = "http://x/${locals.who}" }
`
	cases := map[string]func(libDir string) string{
		"relative": func(string) string { return "./lib/lib.rest" },
		"absolute": func(libDir string) string { return filepath.Join(libDir, "lib.rest") },
		"home":     func(string) string { return "~/lib/lib.rest" },
	}
	for name, imp := range cases {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			libDir := filepath.Join(home, "lib")
			if err := os.MkdirAll(libDir, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(libDir, "lib.rest"), []byte(lib), 0644); err != nil {
				t.Fatal(err)
			}
			main := fmt.Sprintf(`
imports = [%q]
locals { who = "main" }
request "main" { url = "http://x/main" }
`, imp(libDir))
			// relative imports resolve from the importing file, so put main in home
			mainPath := filepath.Join(home, "main.rest")
			if err := os.WriteFile(mainPath, []byte(main), 0644); err != nil {
				t.Fatal(err)
			}
			r, err := rest.NewFile(mainPath)
			if err != nil {
				t.Fatal(err)
			}
			req, err := r.Request("lib::one")
			if err != nil {
				t.Fatal(err)
			}
			// importing file's locals still override the library's
			if req.URL != "http://x/main" {
				t.Errorf("expected url http://x/main, got %s", req.URL)
			}
		})
	}
}

func TestReadExpandsHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.WriteFile(filepath.Join(home, "body.txt"), []byte("from home"), 0644); err != nil {
		t.Fatal(err)
	}
	dir := writeFiles(t, map[string]string{"main.rest": `
request "r" {
  url  = "http://x/"
  method = "POST"
  body = read("~/body.txt")
}
`})
	r, err := rest.NewFile(filepath.Join(dir, "main.rest"))
	if err != nil {
		t.Fatal(err)
	}
	req, err := r.Request("r")
	if err != nil {
		t.Fatal(err)
	}
	if got := req.Body; got != "from home" {
		t.Errorf("expected body %q, got %q", "from home", got)
	}
}
