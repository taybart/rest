package exports_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/taybart/rest"
)

const openapiFile = `
server {
  address = "localhost:8080"
  tls = "cert.pem,key.pem"
  handler "POST" "/login" {
    expect {
      headers = {
        "Content-Type" = "application/json"
        "X-Request-Id" = is::optional(is.uuid)
      }
      body = {
        username = is.not_empty
        remember = is::optional(is.bool)
      }
    }
    response {
      status = 201
      headers = { "X-Thing" = "1" }
      body = { token = "abc" }
    }
  }
  handler "GET" "/users/{id}/files/{path...}" {
    expect {
      path  = { id = is.int }
      query = { tag = is::optional(is::array_of(is.string)), q = is.string }
    }
    response { status = 200 }
  }
  handler "*" "/any" {
    fn = "return { status = 204 }"
  }
  handler "GET" "/{$}" {
    response {
      status = 200
      body = "hello"
    }
  }
  handler "GET" "/ws" {
    ws = true
  }
  handler "PUT" "/tags" {
    expect {
      body = is::optional(is::array_of(is.string))
    }
    response { status = 204 }
  }
}
`

func exportOpenAPI(t *testing.T) map[string]any {
	t.Helper()
	filename := filepath.Join(t.TempDir(), "api.rest")
	if err := os.WriteFile(filename, []byte(openapiFile), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := rest.NewFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := r.ExportTo(&out, "openapi", "", -1); err != nil {
		t.Fatal(err)
	}
	var spec map[string]any
	if err := json.Unmarshal(out.Bytes(), &spec); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	return spec
}

// get walks the spec, "paths", "/login", "post" ...
func get(t *testing.T, v any, keys ...any) any {
	t.Helper()
	for _, k := range keys {
		switch k := k.(type) {
		case string:
			m, ok := v.(map[string]any)
			if !ok {
				t.Fatalf("expected object at %q, got %#v", k, v)
			}
			v = m[k]
		case int:
			l, ok := v.([]any)
			if !ok || k >= len(l) {
				t.Fatalf("expected list with index %d, got %#v", k, v)
			}
			v = l[k]
		}
	}
	return v
}

func jsonOf(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestOpenAPIExport(t *testing.T) {
	spec := exportOpenAPI(t)

	if spec["openapi"] != "3.1.0" || get(t, spec, "info", "title") != "api" {
		t.Fatalf("bad header: %v %v", spec["openapi"], spec["info"])
	}
	if got := get(t, spec, "servers", 0, "url"); got != "https://localhost:8080" {
		t.Errorf("server url %v", got)
	}

	paths := get(t, spec, "paths").(map[string]any)
	for _, p := range []string{"/login", "/users/{id}/files/{path}", "/any", "/", "/tags"} {
		if _, ok := paths[p]; !ok {
			t.Errorf("missing path %s in %v", p, jsonOf(paths))
		}
	}
	if _, ok := paths["/ws"]; ok {
		t.Error("websocket handlers shouldn't be exported")
	}

	t.Run("login", func(t *testing.T) {
		op := get(t, paths, "/login", "post")
		// Content-Type is reserved in openapi, it picks the media type instead
		params := get(t, op, "parameters").([]any)
		if len(params) != 1 || get(t, params, 0, "name") != "X-Request-Id" || get(t, params, 0, "required") != false {
			t.Errorf("header params: %s", jsonOf(params))
		}
		schema := get(t, op, "requestBody", "content", "application/json", "schema")
		if jsonOf(get(t, schema, "required")) != `["username"]` || get(t, schema, "additionalProperties") != false {
			t.Errorf("body schema: %s", jsonOf(schema))
		}
		res := get(t, op, "responses", "201")
		if jsonOf(get(t, res, "content", "application/json", "example")) != `{"token":"abc"}` ||
			get(t, res, "headers", "X-Thing", "example") != "1" {
			t.Errorf("response: %s", jsonOf(res))
		}
		if get(t, op, "responses", "400") == nil {
			t.Error("handlers with expect can answer 400")
		}
	})

	t.Run("params", func(t *testing.T) {
		params := get(t, paths, "/users/{id}/files/{path}", "get", "parameters").([]any)
		want := `[{"in":"path","name":"id","required":true,"schema":{"type":"integer"}},` +
			`{"description":"matches the rest of the path, slashes included","in":"path","name":"path","required":true,"schema":{"type":"string"}},` +
			`{"in":"query","name":"q","required":true,"schema":{"type":"string"}},` +
			`{"in":"query","name":"tag","required":false,"schema":{"items":{"type":"string"},"type":"array"}}]`
		if jsonOf(params) != want {
			t.Errorf("got  %s\nwant %s", jsonOf(params), want)
		}
	})

	t.Run("any method fn handler", func(t *testing.T) {
		item := get(t, paths, "/any").(map[string]any)
		if len(item) != 5 {
			t.Errorf("* should be listed under every method: %s", jsonOf(item))
		}
		if get(t, item, "post", "responses", "default", "description") != "handled by fn" {
			t.Errorf("fn handler response: %s", jsonOf(item["post"]))
		}
		if get(t, item, "post", "responses", "400") != nil {
			t.Error("no expect, no 400")
		}
	})

	t.Run("single matcher body", func(t *testing.T) {
		rb := get(t, paths, "/tags", "put", "requestBody")
		if get(t, rb, "required") != false ||
			jsonOf(get(t, rb, "content", "application/json", "schema")) != `{"items":{"type":"string"},"type":"array"}` {
			t.Errorf("request body: %s", jsonOf(rb))
		}
	})

	t.Run("text response", func(t *testing.T) {
		if get(t, paths, "/", "get", "responses", "200", "content", "text/plain", "example") != "hello" {
			t.Errorf("text body: %s", jsonOf(paths["/"]))
		}
	})
}

func TestOpenAPINeedsServer(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "client.rest")
	if err := os.WriteFile(filename, []byte(`request "a" { url = "http://x" }`), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := rest.NewFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ExportTo(&bytes.Buffer{}, "openapi", "", -1); err == nil {
		t.Fatal("expected an error without a server block")
	}
}
