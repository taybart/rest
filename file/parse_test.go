package file_test

import (
	"os"
	"path/filepath"
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
  after = lib.print_req_res
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
	if precanned.After != "print(rest.req.dump)\nprint(rest.res.dump)" {
		t.Fatalf("unexpected precanned after: %q", precanned.After)
	}

	inspectReq, err := r.Request("inspect_fn")
	if err != nil {
		t.Fatal(err)
	}
	if inspectReq.After != `tools.inspect_res(json.decode("\"id\""), json.decode("\"name\""))` {
		t.Fatalf("unexpected inspect_fn after: %q", inspectReq.After)
	}

	combined, err := r.Request("combined_list")
	if err != nil {
		t.Fatal(err)
	}
	wantCombined := "tools.expected()\ntools.inspect_res(json.decode(\"[\\\"id\\\",\\\"name\\\"]\"))\ntools.export_res(json.decode(\"{\\\"user_id\\\":\\\"user.id\\\"}\"))"
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
	if err := request.MatchExpectedBody(req.Expect.Body, `{"token":"`+req.BearerToken+`"}`, false); err != nil {
		t.Fatalf("expected is.jwt matcher to pass, got: %v", err)
	}
	if err := request.MatchExpectedBody(req.Expect.Body, `{"token":"not-a-jwt"}`, false); err == nil {
		t.Fatal("expected is.jwt matcher to fail on invalid jwt")
	}
}

