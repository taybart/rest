package exports_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/taybart/rest"
	"github.com/taybart/rest/exports"
	"github.com/taybart/rest/exports/templates"
)

func postmanRequests(base string) []templates.Request {
	return []templates.Request{
		{
			Label:   "basic auth get",
			Method:  "GET",
			BaseURL: base + "/a/b/",
			Query:   url.Values{"keep": {"1"}, "multi": {"x", "y"}, "spaced": {"two words & more"}},
			Auth:    templates.Auth{Type: "basic", User: "user", Password: "p:ss"},
			Expect:  templates.Expect{Status: 201},
		},
		{
			Label:          "bearer post json",
			Method:         "POST",
			BaseURL:        base + "/json",
			RequestHeaders: map[string]string{"Content-Type": "application/json", "X-Thing": `it's "quoted"`},
			Auth:           templates.Auth{Type: "bearer", Token: "tok"},
			UserAgent:      "custom/1.0",
			Cookies:        map[string]string{"session": "abc", "other": "def"},
			Body:           `{"msg":"hi 💥","n":[1,2]}`,
		},
		{
			Label:          "put text",
			Method:         "PUT",
			BaseURL:        base + "/text",
			RequestHeaders: map[string]string{"Content-Type": "text/plain"},
			Body:           "plain\nbody",
		},
	}
}

func render(t *testing.T, reqs []templates.Request) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := exports.ToPostman(&out, "/some/dir/api.rest", reqs, nil); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// TestPostmanShape checks the collection without needing postman
func TestPostmanShape(t *testing.T) {
	var c struct {
		Info struct{ Name, Schema string }
		Item []struct {
			Name    string
			Request struct {
				Method string
				URL    struct {
					Raw   string
					Path  []string
					Query []struct{ Key, Value string }
				}
				Header []struct{ Key, Value string }
				Auth   map[string]any
				Body   *struct {
					Raw     string
					Options map[string]any
				}
			}
			Event []struct{ Listen string }
		}
	}
	if err := json.Unmarshal(render(t, postmanRequests("http://localhost:1234")), &c); err != nil {
		t.Fatal(err)
	}
	if c.Info.Name != "api" || !strings.Contains(c.Info.Schema, "v2.1.0") {
		t.Fatalf("bad info: %+v", c.Info)
	}
	if len(c.Item) != 3 || c.Item[0].Name != "basic auth get" || c.Item[2].Name != "put text" {
		t.Fatalf("items out of order: %+v", c.Item)
	}

	get := c.Item[0].Request
	// no leading "" segment (shows up as //a in postman), trailing slash kept
	if strings.Join(get.URL.Path, ",") != "a,b," {
		t.Errorf("path segments: %q", get.URL.Path)
	}
	if len(get.URL.Query) != 4 || !strings.Contains(get.URL.Raw, "multi=x&multi=y") {
		t.Errorf("query: %+v raw %s", get.URL.Query, get.URL.Raw)
	}
	if get.Body != nil {
		t.Errorf("GET without a body shouldn't have one: %+v", get.Body)
	}
	if len(c.Item[0].Event) != 1 || c.Item[0].Event[0].Listen != "test" {
		t.Errorf("expect status should become a test: %+v", c.Item[0].Event)
	}

	post := c.Item[1].Request
	for _, h := range post.Header {
		if h.Key == "Authorization" {
			t.Errorf("auth belongs in the auth object, not a header")
		}
	}
	if post.Body == nil || post.Body.Options == nil {
		t.Errorf("json body should set the raw language: %+v", post.Body)
	}
}

type received struct {
	Method, URI, Body string
	Header            http.Header
}

// TestPostmanNewman runs the collection with postman's cli runner and checks
// what the server received. Skipped without newman on PATH.
func TestPostmanNewman(t *testing.T) {
	newman, err := exec.LookPath("newman")
	if err != nil {
		t.Skip("newman not installed (npm i -g newman)")
	}
	var mu sync.Mutex
	var got []received
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		got = append(got, received{r.Method, r.URL.RequestURI(), string(body), r.Header.Clone()})
		mu.Unlock()
		if r.URL.Path == "/a/b/" {
			w.WriteHeader(http.StatusCreated) // satisfies the exported status test
		}
	}))
	defer srv.Close()

	reqs := postmanRequests(srv.URL)
	file := filepath.Join(t.TempDir(), "collection.json")
	if err := os.WriteFile(file, render(t, reqs), 0o644); err != nil {
		t.Fatal(err)
	}
	// newman exits non-zero when a pm.test fails
	out, err := exec.Command(newman, "run", file, "--reporters", "cli", "--color", "off").CombinedOutput()
	if err != nil {
		t.Fatalf("newman: %v\n%s", err, out)
	}

	if len(got) != len(reqs) {
		t.Fatalf("server got %d requests, want %d\n%s", len(got), len(reqs), out)
	}
	for i, want := range reqs {
		g := got[i]
		if g.Method != want.Method {
			t.Errorf("%s: method %s", want.Label, g.Method)
		}
		// compare decoded, postman writes a space as %20 where go uses +
		gu, _ := url.Parse(g.URI)
		wu, _ := url.Parse(want.URL())
		if gu.Path != wu.Path || gu.Query().Encode() != wu.Query().Encode() {
			t.Errorf("%s: uri %q want %q", want.Label, g.URI, want.URL())
		}
		for k, v := range want.Headers() {
			if g.Header.Get(k) != v {
				t.Errorf("%s: header %s = %q want %q", want.Label, k, g.Header.Get(k), v)
			}
		}
		if want.IsJSON() {
			var a, b any
			_ = json.Unmarshal([]byte(g.Body), &a)
			_ = json.Unmarshal([]byte(want.Body), &b)
			if !jsonEqual(a, b) {
				t.Errorf("%s: body %q want %q", want.Label, g.Body, want.Body)
			}
		} else if g.Body != want.Body {
			t.Errorf("%s: body %q want %q", want.Label, g.Body, want.Body)
		}
	}
}

func jsonEqual(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(x, y)
}

const schemaFile = `
schemas {
  tag      = { id = is::optional(is.int), name = is::optional(is.string) }
  category = { id = is::optional(is.int), name = is::optional(is.string) }
  pet = {
    id       = is.uuid
    name     = is.not_empty
    tags     = is::array_of(schemas.tag)
    category = is::optional(schemas.category)
    pair     = [is.string, 1]
  }
}

request "good pet" {
  url = "BASE/good"
  expect {
    status  = 200
    headers = { "X-Kind" = "pet", "X-Trace" = is("regex:^t-[0-9]+$") }
    body    = schemas.pet
  }
}

request "bad pet" {
  url = "BASE/bad"
  expect {
    status = 200
    body   = schemas.pet
  }
}

request "list" {
  url = "BASE/list"
  expect {
    status     = 200
    additional = true
    body       = is::array_of(schemas.pet)
  }
}
`

const goodPet = `{"id":"7d444840-9dc0-11d1-b245-5ffdce74fad2","name":"rex","tags":[{"id":1,"name":"a"}],"category":{"id":2},"pair":["x",1]}`

func exportSchemaCollection(t *testing.T, base string) []byte {
	t.Helper()
	filename := filepath.Join(t.TempDir(), "pets.rest")
	if err := os.WriteFile(filename, []byte(strings.ReplaceAll(schemaFile, "BASE", base)), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := rest.NewFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := r.ExportTo(&out, "postman", "", -1); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestPostmanSchemasShape(t *testing.T) {
	out := string(exportSchemaCollection(t, "http://localhost:1"))
	for _, want := range []string{
		// defined once at collection level, dependencies first
		`"schemas = {};"`,
		`"schemas.tag = {"`,
		`"schemas.category = {"`,
		`"schemas.pet = {"`,
		// pet refers to the others by name, tag and category have the same
		// shape and still keep their own names
		`"$ref`,
		`pm.response.to.have.jsonSchema(schemas.pet);`,
		`body matches schemas.pet`,
	} {
		if want == `"$ref` {
			if strings.Contains(out, want) {
				t.Errorf("postman scripts should reference schemas.x, not $ref")
			}
			continue
		}
		if !strings.Contains(out, want) {
			t.Errorf("missing %s", want)
		}
	}
	for _, want := range []string{`schemas.tag`, `schemas.category`} {
		start := strings.Index(out, `"schemas.pet = {"`)
		if start < 0 || !strings.Contains(out[start:], want) {
			t.Errorf("schemas.pet should reference %s", want)
		}
	}
	if !strings.Contains(out, `items: [`) {
		t.Errorf("fixed length lists should use draft-07 items: [...]:\n%s", out)
	}
	if strings.Contains(out, "prefixItems") {
		t.Error("postman's validator ignores prefixItems")
	}
}

// TestPostmanSchemasNewman runs the schema tests in postman's runner against
// good and bad responses
func TestPostmanSchemasNewman(t *testing.T) {
	newman, err := exec.LookPath("newman")
	if err != nil {
		t.Skip("newman not installed (npm i -g newman)")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/good":
			w.Header().Set("X-Kind", "pet")
			w.Header().Set("X-Trace", "t-42")
			io.WriteString(w, goodPet)
		case "/bad":
			// bad id format, a tag name that isn't a string, wrong tuple value
			io.WriteString(w, `{"id":"nope","name":"rex","tags":[{"name":5}],"pair":["x",2]}`)
		case "/list":
			io.WriteString(w, "["+strings.TrimSuffix(goodPet, "}")+`,"extra":true}]`)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	collection := filepath.Join(dir, "collection.json")
	report := filepath.Join(dir, "report.json")
	if err := os.WriteFile(collection, exportSchemaCollection(t, srv.URL), 0o644); err != nil {
		t.Fatal(err)
	}
	// exits non-zero because bad pet fails, the report says what failed
	out, _ := exec.Command(newman, "run", collection, "--reporters", "json", "--reporter-json-export", report).CombinedOutput()
	raw, err := os.ReadFile(report)
	if err != nil {
		t.Fatalf("no newman report: %v\n%s", err, out)
	}
	var rep struct {
		Run struct {
			Executions []struct {
				Item       struct{ Name string }
				Assertions []struct {
					Assertion string
					Error     *struct{ Message string }
				}
			}
		}
	}
	if err := json.Unmarshal(raw, &rep); err != nil {
		t.Fatal(err)
	}
	results := map[string]string{} // "item: assertion" -> "" or the error
	for _, e := range rep.Run.Executions {
		for _, a := range e.Assertions {
			msg := ""
			if a.Error != nil {
				msg = a.Error.Message
			}
			results[e.Item.Name+": "+a.Assertion] = msg
		}
	}
	for _, pass := range []string{
		"good pet: status is 200",
		"good pet: header X-Kind",
		"good pet: header X-Trace",
		"good pet: body matches schemas.pet",
		"list: body matches expect",
	} {
		msg, ran := results[pass]
		if !ran || msg != "" {
			t.Errorf("%s should pass, got ran=%v %q", pass, ran, msg)
		}
	}
	msg := results["bad pet: body matches schemas.pet"]
	for _, want := range []string{"id", "tags[0].name", "pair[1]"} {
		if !strings.Contains(msg, want) {
			t.Errorf("bad pet failure should mention %s: %q", want, msg)
		}
	}
	if t.Failed() {
		t.Logf("results: %v", results)
	}
}
