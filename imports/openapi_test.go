package imports_test

import (
	"bytes"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/taybart/log"
	"github.com/taybart/rest"
	"github.com/taybart/rest/client"
	"github.com/taybart/rest/imports"
	"github.com/taybart/rest/server"
)

const spec = `{
  "openapi": "3.1.0",
  "info": {"title": "pets", "version": "1"},
  "servers": [{"url": "https://api.example.com/v1"}],
  "security": [{"bearer": []}],
  "components": {
    "securitySchemes": {
      "bearer": {"type": "http", "scheme": "bearer"},
      "key": {"type": "apiKey", "in": "header", "name": "X-Api-Key"}
    },
    "schemas": {
      "Base": {
        "type": "object",
        "required": ["id"],
        "properties": {
          "id": {"type": "string", "format": "uuid", "readOnly": true},
          "created": {"type": "string", "format": "date-time", "readOnly": true}
        }
      },
      "Pet": {
        "allOf": [
          {"$ref": "#/components/schemas/Base"},
          {
            "type": "object",
            "required": ["name", "kind"],
            "properties": {
              "name": {"type": "string", "pattern": "^[A-Za-z ]+$", "example": "Rex"},
              "kind": {"type": "string", "enum": ["dog", "cat", "a.b"]},
              "age": {"type": "integer", "minimum": 0},
              "owner": {"type": ["string", "null"]},
              "tags": {"type": "array", "items": {"type": "string"}},
              "parent": {"$ref": "#/components/schemas/Pet"},
              "toy": {"oneOf": [{"type": "string"}, {"type": "integer"}]},
              "lookahead": {"type": "string", "pattern": "^(?=a)a+$"},
              "$weird key": {"type": "string", "example": "${not interpolated}"}
            }
          }
        ]
      },
      "Error": {"type": "object", "properties": {"error": {"type": "string"}}}
    },
    "parameters": {
      "PetID": {"name": "pet-id", "in": "path", "required": true, "schema": {"type": "integer"}, "example": 7}
    }
  },
  "paths": {
    "/pets": {
      "get": {
        "operationId": "listPets",
        "parameters": [
          {"name": "limit", "in": "query", "required": true, "schema": {"type": "integer"}},
          {"name": "tag", "in": "query", "schema": {"type": "array", "items": {"type": "string"}}},
          {"name": "X-Trace", "in": "header", "schema": {"type": "string", "format": "uuid"}}
        ],
        "responses": {
          "200": {"description": "ok", "content": {"application/json": {"schema": {"type": "array", "items": {"$ref": "#/components/schemas/Pet"}}}}}
        }
      },
      "post": {
        "operationId": "createPet",
        "requestBody": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/Pet"}}}},
        "responses": {
          "201": {"description": "created", "content": {"application/json": {"example": {"id": "00000000-0000-4000-8000-000000000000", "name": "Rex"}}}},
          "400": {"description": "bad", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Error"}}}}
        }
      }
    },
    "/pets/{pet-id}": {
      "parameters": [{"$ref": "#/components/parameters/PetID"}],
      "get": {
        "operationId": "getPet",
        "security": [{"key": []}],
        "responses": {"200": {"description": "ok", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Pet"}}}}}
      },
      "delete": {
        "operationId": "deletePet",
        "responses": {"204": {"description": "gone"}}
      }
    },
    "/pets/{id}.json": {
      "get": {"operationId": "petJSON", "responses": {"200": {"description": "ok"}}}
    },
    "/pets/{zname}": {
      "get": {"operationId": "conflicts", "parameters": [{"name": "zname", "in": "path", "required": true, "schema": {"type": "string"}}], "responses": {"200": {"description": "ok"}}}
    },
    "/upload": {
      "post": {
        "operationId": "upload",
        "requestBody": {"content": {"multipart/form-data": {"schema": {"type": "object"}}}},
        "responses": {"200": {"description": "ok", "content": {"text/plain": {"example": "uploaded"}}}}
      }
    }
  }
}`

func init() {
	// the generated servers log every route and request
	log.SetLevel(log.WARN)
}

func importBoth(t *testing.T, spec string) string {
	t.Helper()
	out, err := imports.FromOpenAPI([]byte(spec), imports.Options{Side: imports.Both})
	if err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(t.TempDir(), "api.rest")
	if err := os.WriteFile(filename, out, 0o644); err != nil {
		t.Fatal(err)
	}
	return filename
}

var noHandler = regexp.MustCompile(`request "([^"]+)" \{\n\s*# the generated server has no handler`)

// roundTrip starts the generated server and runs every generated request
// against it. Requests the generated file says have no handler aren't
// expected to pass, they're returned in skipped.
func roundTrip(t *testing.T, filename string) (ran int, failed map[string]error, skipped []string) {
	t.Helper()
	parsed, err := rest.NewFile(filename)
	if err != nil {
		src, _ := os.ReadFile(filename)
		t.Fatalf("generated file doesn't parse: %v\n%s", err, src)
	}
	cfg, err := parsed.Parser.Server()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Quiet = true
	srv := httptest.NewServer(server.New(cfg).Server.Handler)
	defer srv.Close()

	// locals are evaluated at parse time, so reparse with the test server
	t.Setenv("BASE_URL", srv.URL)
	r, err := rest.NewFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	c, err := client.New(r.Parser.Config)
	if err != nil {
		t.Fatal(err)
	}
	c.Out = &bytes.Buffer{}
	src, _ := os.ReadFile(filename)
	known := map[string]bool{}
	for _, m := range noHandler.FindAllStringSubmatch(string(src), -1) {
		known[m[1]] = true
		skipped = append(skipped, m[1])
	}
	failed = map[string]error{}
	for label := range r.Requests {
		if known[label] {
			continue
		}
		req, err := r.Request(label)
		if err != nil {
			failed[label] = err
			continue
		}
		ran++
		if _, _, err := c.Do(req); err != nil {
			failed[label] = err
		}
	}
	return ran, failed, skipped
}

func TestImportRoundTrip(t *testing.T) {
	filename := importBoth(t, spec)
	ran, failed, skipped := roundTrip(t, filename)
	// petJSON has a param inside a segment, conflicts collides with getPet
	if ran != 5 || strings.Join(sorted(skipped), ",") != "conflicts,petJSON" {
		t.Errorf("ran %d, skipped %v", ran, skipped)
	}
	src, _ := os.ReadFile(filename)
	for label, err := range failed {
		t.Errorf("%s: %v", label, err)
	}
	if t.Failed() {
		t.Logf("generated:\n%s", src)
	}
}

func sorted(s []string) []string {
	out := append([]string{}, s...)
	sort.Strings(out)
	return out
}

func TestImportOutput(t *testing.T) {
	out, err := imports.FromOpenAPI([]byte(spec), imports.Options{Side: imports.Both})
	if err != nil {
		t.Fatal(err)
	}
	src := string(out)
	for _, want := range []string{
		// both sides default the client at the generated server
		`base = env("BASE_URL", "http://localhost:8080")`,
		`token = env("API_TOKEN", "token")`,
		// dashed path params get renamed for go's mux, values substituted in the client
		`handler "GET" "/pets/{pet_id}"`,
		`url    = "${locals.base}/pets/7"`,
		// unexpressible routes are skipped with a reason, not dropped silently
		`# skipped GET /pets/{id}.json: parameter inside a path segment`,
		`# skipped GET /pets/{zname}: conflicts with GET /pets/{pet_id} in go's router`,
		`# the generated server has no handler for this route (parameter inside a path segment ({id}.json))`,
		`# request body is multipart/form-data, only json bodies are scaffolded`,
		// matchers
		`kind = is("regex:^(dog|cat|a\\.b)$")`,
		`owner     = is::optional(is.any)`,
		`tags      = is::optional(is::array_of(is.string))`,
		`lookahead = is::optional(is.string)`,
		`Authorization = is("regex:^Bearer .+")`,
		`X-Api-Key = is.not_empty`,
		`X-Trace = is::optional(is.uuid)`,
		// ${ in data is escaped, not interpolated
		`"$${not interpolated}"`,
		// text responses keep their content type
		`Content-Type = "text/plain"`,
		// components.schemas are defined once and referenced, the request view
		// leaves out readOnly fields
		`schemas {`,
		`body = schemas.pet_input`,
		// the client checks responses against the spec
		`body = is::array_of(schemas.pet)`,
		// Pet.parent refers back to Pet, cut so the file has no cycle
		`parent = is::optional(is.any)`,
	} {
		// hclwrite aligns = signs, compare with spaces collapsed
		if !strings.Contains(squash(src), squash(want)) {
			t.Errorf("missing %s", want)
		}
	}
	inputStart := strings.Index(src, "pet_input = {")
	if inputStart < 0 {
		t.Error("missing pet_input")
	} else if input := src[inputStart : inputStart+strings.Index(src[inputStart:], "\n  }")]; strings.Contains(input, "created") || strings.Contains(input, " id ") {
		t.Errorf("pet_input has readOnly fields:\n%s", input)
	}
	if strings.Contains(src, "created") && strings.Contains(src, `request "createPet"`) {
		// readOnly fields shouldn't be sent by the client
		start := strings.Index(src, `request "createPet"`)
		end := start + strings.Index(src[start:], "\n}\n")
		if strings.Contains(src[start:end], "created") || strings.Contains(src[start:end], " id ") {
			t.Errorf("client sends readOnly fields:\n%s", src[start:end])
		}
	}
	if t.Failed() {
		t.Logf("generated:\n%s", src)
	}
}

func TestImportSides(t *testing.T) {
	client, err := imports.FromOpenAPI([]byte(spec), imports.Options{Side: imports.Client})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(client), "server {") || !strings.Contains(string(client), `"https://api.example.com/v1"`) {
		t.Errorf("client only should point at the spec's server:\n%s", client)
	}
	srv, err := imports.FromOpenAPI([]byte(spec), imports.Options{Side: imports.Server, Addr: "localhost:9999"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(srv), "request \"") || !strings.Contains(string(srv), `"localhost:9999"`) {
		t.Errorf("server only:\n%s", srv)
	}
}

func TestImportRejects(t *testing.T) {
	for name, in := range map[string]string{
		"yaml":    "openapi: 3.1.0\n",
		"swagger": `{"swagger": "2.0"}`,
	} {
		if _, err := imports.FromOpenAPI([]byte(in), imports.Options{}); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	if _, err := imports.FromOpenAPI([]byte(spec), imports.Options{Side: "sideways"}); err == nil {
		t.Error("expected an error for a bad side")
	}
}

// TestImportOpenAI round trips a large real world spec, skipped when it
// isn't checked out
func TestImportOpenAI(t *testing.T) {
	path := "../test/openapi_specs/openai.json"
	src, err := os.ReadFile(path)
	if err != nil {
		t.Skip("no openai spec at " + path)
	}
	filename := importBoth(t, string(src))
	ran, failed, skipped := roundTrip(t, filename)
	t.Logf("openai: %d requests passed against the generated server, %d failed, %d have no handler (%v)",
		ran-len(failed), len(failed), len(skipped), skipped)
	for label, err := range failed {
		t.Errorf("%s: %v", label, err)
	}
}

func squash(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func TestPatternExample(t *testing.T) {
	for _, p := range []string{
		`^https://`,
		`^live_[A-Za-z0-9_-]{1,128}$`,
		`^[a-z]{3}-\d{2,}$`,
		`^(foo|bar)+baz?$`,
		`^\p{Lu}[^0-9]*$`,
		`^v[0-9]+\.[0-9]+$`,
	} {
		ex, ok := imports.PatternExample(p)
		if !ok {
			t.Errorf("%s: no example", p)
			continue
		}
		t.Logf("%s -> %q", p, ex)
	}
	if _, ok := imports.PatternExample(`^(?=a)`); ok {
		t.Error("go can't compile lookaheads, there should be no example")
	}
}
