package exports

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/taybart/rest/exports/postman"
	"github.com/taybart/rest/exports/templates"
	"github.com/taybart/rest/request"
)

// ToPostman writes requests as a postman v2.1 collection named after the
// rest file. expect blocks become postman tests, and the schemas they use
// (named, see file.Parser.SchemaValues) are defined once in a collection
// level script so tests can say schemas.pet like the rest file does.
func ToPostman(w io.Writer, filename string, reqs []templates.Request, named map[string]any) error {
	name := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	c := postman.CreateCollection(name, "")
	schemas := newComponentSet(named, func(def string) map[string]any {
		return map[string]any{postmanRef: def}
	})
	for _, r := range reqs {
		item, err := postmanItem(r, schemas)
		if err != nil {
			return fmt.Errorf("%s: %w", r.Label, err)
		}
		c.AddItem(item)
	}
	if len(schemas.order) > 0 {
		// collection test scripts run before each request's, in the same scope
		script := []string{"// schemas from the rest file, used by the request tests", "schemas = {};"}
		for _, def := range schemas.order {
			script = append(script, strings.Split("schemas."+def+" = "+jsSchema(schemas.defs[def])+";", "\n")...)
		}
		c.Events = append(c.Events, postman.CreateEvent(postman.Test, script))
	}
	if err := c.Write(w, postman.V210); err != nil {
		return err
	}
	_, err := io.WriteString(w, "\n")
	return err
}

func postmanItem(r templates.Request, schemas *componentSet) (*postman.Items, error) {
	u, err := postmanURL(r)
	if err != nil {
		return nil, err
	}
	req := &postman.Request{
		URL:    u,
		Method: postman.Method(r.Method),
	}

	// Authorization goes in auth below so it shows up in postman's auth tab
	headers := map[string]string{}
	for k, v := range r.RequestHeaders {
		headers[k] = v
	}
	if r.UserAgent != "" {
		headers["User-Agent"] = r.UserAgent
	}
	if c := r.CookieHeader(); c != "" {
		headers["Cookie"] = c
	}
	for _, k := range sortedKeys(headers) {
		req.Header = append(req.Header, &postman.Header{Key: k, Value: headers[k]})
	}

	switch r.Auth.Type {
	case "basic":
		req.Auth = postman.CreateAuth(postman.Basic,
			postman.CreateAuthParam("username", r.Auth.User),
			postman.CreateAuthParam("password", r.Auth.Password),
		)
	case "bearer":
		req.Auth = postman.CreateAuth(postman.Bearer,
			postman.CreateAuthParam("token", r.Auth.Token),
		)
	}

	if r.Body != "" {
		req.Body = &postman.Body{Mode: "raw", Raw: r.Body}
		if r.IsJSON() {
			// pretty so it reads well in postman's body editor
			var buf bytes.Buffer
			if json.Indent(&buf, []byte(r.Body), "", "  ") == nil {
				req.Body.Raw = buf.String()
			}
			req.Body.Options = &postman.BodyOptions{Raw: postman.BodyOptionsRaw{Language: postman.JSON}}
		}
	}

	item := &postman.Items{Name: r.Label, Request: req}
	tests, err := postmanTests(r.Expect, schemas)
	if err != nil {
		return nil, err
	}
	if len(tests) > 0 {
		item.Events = append(item.Events, postman.CreateEvent(postman.Test, tests))
	}
	return item, nil
}

func postmanURL(r templates.Request) (*postman.URL, error) {
	parsed, err := url.Parse(r.BaseURL)
	if err != nil {
		return nil, err
	}
	u := &postman.URL{
		Raw:      r.URL(),
		Protocol: parsed.Scheme,
		Host:     strings.Split(parsed.Hostname(), "."),
		Port:     parsed.Port(),
	}
	// "/a/b" -> ["a", "b"], postman joins path segments with "/" itself
	if p := strings.TrimPrefix(parsed.Path, "/"); p != "" {
		u.Path = strings.Split(p, "/")
	}
	for _, k := range sortedKeys(r.Query) {
		for _, v := range r.Query[k] {
			u.Query = append(u.Query, &postman.QueryParam{Key: k, Value: v})
		}
	}
	return u, nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// postmanRef marks a reference to a named schema until it's written as
// schemas.<name>
const postmanRef = "\x00rest_schema"

// postmanTests turns an expect block into postman test script lines
func postmanTests(e templates.Expect, schemas *componentSet) ([]string, error) {
	var lines []string
	test := func(name string, body ...string) {
		lines = append(lines, fmt.Sprintf("pm.test(%s, function () {", jsString(name)))
		for _, b := range body {
			lines = append(lines, "  "+b)
		}
		lines = append(lines, "});")
	}

	if e.Status != 0 {
		test(fmt.Sprintf("status is %d", e.Status), fmt.Sprintf("pm.response.to.have.status(%d);", e.Status))
	}

	for _, k := range sortedKeys(e.Headers) {
		expected := e.Headers[k]
		if _, inner, ok := request.CutNamed(expected); ok {
			if s, isStr := inner.(string); isStr {
				expected = s
			}
		}
		get := fmt.Sprintf("pm.response.headers.get(%s)", jsString(k))
		rule, isRule := strings.CutPrefix(expected, request.IsPrefix)
		switch {
		case !isRule:
			test("header "+k, fmt.Sprintf("pm.response.to.have.header(%s, %s);", jsString(k), jsString(expected)))
		case request.IsOptional(expected):
			// optional headers are only checked when they're there, which a
			// presence test can't express, leave them out
		case strings.HasPrefix(rule, "regex:") || strings.HasPrefix(rule, "re:"):
			pattern := strings.TrimPrefix(strings.TrimPrefix(rule, "regex:"), "re:")
			test("header "+k, fmt.Sprintf("pm.expect(%s).to.match(new RegExp(%s));", get, jsString(pattern)))
		default:
			// other matchers (is.uuid, is.int, ...) only check the header is there
			test("header "+k, fmt.Sprintf("pm.response.to.have.header(%s);", jsString(k)))
		}
	}

	if e.Body != "" {
		body, err := request.DecodeExpectBody(e.Body)
		if err != nil {
			return nil, fmt.Errorf("expect body: %w", err)
		}
		if !bareMatcher(body) {
			schema := request.MatcherSchema(body, schemas.opts(request.SchemaOptions{Additional: e.Additional}))
			name := "body matches expect"
			if def, ok := schema[postmanRef].(string); ok {
				name = "body matches schemas." + def
			}
			test(name, strings.Split("pm.response.to.have.jsonSchema("+jsSchema(schema)+");", "\n")...)
		}
	}
	return lines, nil
}

// bareMatcher: rest checks a single top level rule like is.string against the
// raw body text, not the json, which a json schema can't express
func bareMatcher(v any) bool {
	for {
		_, inner, ok := request.CutNamed(v)
		if !ok {
			break
		}
		v = inner
	}
	s, ok := v.(string)
	if !ok {
		return false
	}
	rule, isRule := strings.CutPrefix(s, request.IsPrefix)
	if !isRule {
		return true // a literal string body, compared as text
	}
	return !strings.HasPrefix(rule, "optional:") && !strings.HasPrefix(rule, "array_of:")
}

// jsSchema writes a JSON schema as readable javascript: bare keys, the
// keys that say what something is first, short values on one line, and
// references to named schemas as schemas.<name>. It's in draft-07 form
// (postman's validator), where a fixed length list is items: [...] rather
// than prefixItems.
func jsSchema(schema map[string]any) string {
	return jsValue(draft07(schema), "")
}

// keys read in this order, the rest alphabetically after
var jsKeyOrder = []string{"type", "format", "pattern", "const", "required", "properties", "items", "additionalProperties"}

var jsIdent = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*$`)

const jsLineWidth = 80

func jsValue(v any, indent string) string {
	if inline := jsInline(v); len(indent)+len(inline) <= jsLineWidth {
		return inline
	}
	inner := indent + "  "
	switch v := v.(type) {
	case map[string]any:
		if ref, ok := v[postmanRef].(string); ok {
			return "schemas." + ref
		}
		var b strings.Builder
		b.WriteString("{\n")
		for _, k := range jsKeys(v) {
			b.WriteString(inner + jsKey(k) + ": " + jsValue(v[k], inner) + ",\n")
		}
		b.WriteString(indent + "}")
		return b.String()
	case []any:
		var b strings.Builder
		b.WriteString("[\n")
		for _, item := range v {
			b.WriteString(inner + jsValue(item, inner) + ",\n")
		}
		b.WriteString(indent + "]")
		return b.String()
	}
	return jsInline(v)
}

func jsInline(v any) string {
	switch v := v.(type) {
	case map[string]any:
		if ref, ok := v[postmanRef].(string); ok {
			return "schemas." + ref
		}
		if len(v) == 0 {
			return "{}"
		}
		parts := make([]string, 0, len(v))
		for _, k := range jsKeys(v) {
			parts = append(parts, jsKey(k)+": "+jsInline(v[k]))
		}
		return "{ " + strings.Join(parts, ", ") + " }"
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			parts = append(parts, jsInline(item))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func jsKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	seen := map[string]bool{}
	for _, k := range jsKeyOrder {
		if _, ok := m[k]; ok {
			keys = append(keys, k)
			seen[k] = true
		}
	}
	for _, k := range sortedKeys(m) {
		if !seen[k] {
			keys = append(keys, k)
		}
	}
	return keys
}

func jsKey(k string) string {
	if jsIdent.MatchString(k) {
		return k
	}
	return jsString(k)
}

func draft07(v any) any {
	switch v := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, val := range v {
			if k == "prefixItems" {
				k = "items"
			}
			out[k] = draft07(val)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, val := range v {
			out[i] = draft07(val)
		}
		return out
	case []string:
		out := make([]any, len(v))
		for i, val := range v {
			out[i] = val
		}
		return out
	}
	return v
}

func jsString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
