// Package imports scaffolds rest files from other formats
package imports

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"regexp/syntax"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

// Side picks what an import generates
type Side string

const (
	Client Side = "client"
	Server Side = "server"
	Both   Side = "both"
)

// Options for FromOpenAPI
type Options struct {
	Side Side
	// Addr is the generated server's address, and the client's default base
	// url when both sides are generated
	Addr string
}

// maxDepth bounds how far one schema's inline nesting is expanded into a
// matcher (named schemas start over), deeper values become is.any
const maxDepth = 8

// maxStubDepth bounds example values, which nest through every schema.
// Recursion is caught by tracking refs, this only stops absurd inline nesting.
const maxStubDepth = 64

var methodOrder = []string{"get", "put", "post", "delete", "options", "head", "patch", "trace"}

// FromOpenAPI scaffolds a rest file from an OpenAPI 3.x spec (json). The
// client side gets a request per operation with example values, the server
// side gets a handler per operation whose expect block validates requests
// and whose response returns the spec's example.
func FromOpenAPI(spec []byte, opts Options) ([]byte, error) {
	if opts.Side == "" {
		opts.Side = Both
	}
	if opts.Side != Client && opts.Side != Server && opts.Side != Both {
		return nil, fmt.Errorf("import side must be client, server or both, got %q", opts.Side)
	}
	if opts.Addr == "" {
		opts.Addr = "localhost:8080"
	}

	trimmed := bytes.TrimSpace(spec)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, errors.New("only json specs are supported, convert yaml first (yq -o json spec.yaml > spec.json)")
	}
	dec := json.NewDecoder(bytes.NewReader(spec))
	dec.UseNumber()
	var doc map[string]any
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("parsing spec: %w", err)
	}
	if v, _ := doc["openapi"].(string); !strings.HasPrefix(v, "3.") {
		return nil, fmt.Errorf("only openapi 3.x specs are supported (swagger 2.0 isn't), got %q", v)
	}

	g := &gen{
		doc: doc, opts: opts, labels: map[string]int{}, routes: http.NewServeMux(),
		schemaDefs: map[string]*schemaDef{}, schemaState: map[string]int{}, schemaNames: map[string]*schemaDef{},
	}
	ops := g.operations()
	for i := range ops {
		g.route(&ops[i])
	}
	return g.write(ops), nil
}

type gen struct {
	doc    map[string]any
	opts   Options
	labels map[string]int
	// routes detects handler patterns go's ServeMux would reject or that
	// conflict with an earlier one
	routes *http.ServeMux

	// components.schemas written once into a schemas block, keyed by
	// "<ref>|req" / "<ref>|res"
	schemaDefs  map[string]*schemaDef
	schemaState map[string]int
	schemaNames map[string]*schemaDef
}

// schemaDef is one entry in the generated schemas block
type schemaDef struct {
	name   string
	tokens hclwrite.Tokens
	// additional is set when the schema allows properties it doesn't list
	additional bool
}

const schemaPrefix = "#/components/schemas/"

type param struct {
	name, in string
	required bool
	schema   any
	example  any
	hasEx    bool
}

type operation struct {
	label, method, path string
	params              []param
	body                any // request body schema ($refs kept), json only
	hasBody             bool
	bodyExample         any
	hasBodyEx           bool
	bodyNote            string // why a request body was left out
	status              int
	resContentType      string
	resExample          any
	hasResEx            bool
	resSchema           any // json response schema ($refs kept)
	bearer              bool
	apiKeys             []string // header names
	// pattern is the go ServeMux pattern, skip why it couldn't be served
	pattern string
	names   map[string]string
	skip    string
	// fixedQuery comes from a query string written into the spec's path key
	fixedQuery url.Values
}

func (g *gen) operations() []operation {
	paths, _ := g.doc["paths"].(map[string]any)
	pathKeys := sortedKeys(paths)
	var ops []operation
	for _, path := range pathKeys {
		item, _ := g.resolve(paths[path]).(map[string]any)
		pathParams, _ := item["parameters"].([]any)
		for _, method := range methodOrder {
			raw, ok := item[method].(map[string]any)
			if !ok {
				continue
			}
			ops = append(ops, g.operation(method, path, raw, pathParams))
		}
	}
	return ops
}

func (g *gen) operation(method, path string, op map[string]any, pathParams []any) operation {
	o := operation{method: strings.ToUpper(method), path: path}
	// some specs (openai) key operations by "/x?beta=true"
	if base, query, ok := strings.Cut(path, "?"); ok {
		o.path = base
		o.fixedQuery, _ = url.ParseQuery(query)
	}

	label, _ := op["operationId"].(string)
	if label == "" {
		label = method + " " + path
	}
	if n := g.labels[label]; n > 0 {
		g.labels[label] = n + 1
		label = fmt.Sprintf("%s %d", label, n+1)
	} else {
		g.labels[label] = 1
	}
	o.label = label

	// operation parameters override path item ones with the same name+in
	seen := map[string]bool{}
	opParams, _ := op["parameters"].([]any)
	for _, raw := range append(append([]any{}, opParams...), pathParams...) {
		p, _ := g.resolve(raw).(map[string]any)
		name, _ := p["name"].(string)
		in, _ := p["in"].(string)
		if name == "" || seen[in+":"+name] || (in != "path" && in != "query" && in != "header") {
			continue
		}
		seen[in+":"+name] = true
		required, _ := p["required"].(bool)
		ex, hasEx := firstExample(p)
		o.params = append(o.params, param{
			name: name, in: in, required: required || in == "path",
			schema: p["schema"], example: ex, hasEx: hasEx,
		})
	}
	sort.SliceStable(o.params, func(i, j int) bool {
		return o.params[i].in < o.params[j].in
	})

	if rb, ok := g.resolve(op["requestBody"]).(map[string]any); ok {
		content, _ := rb["content"].(map[string]any)
		if _, media := jsonMedia(content); media != nil {
			o.body, o.hasBody = media["schema"], true
			o.bodyExample, o.hasBodyEx = firstExample(media)
		} else if len(content) > 0 {
			o.bodyNote = "request body is " + strings.Join(sortedKeys(content), ", ") + ", only json bodies are scaffolded"
		}
	}

	o.status = http.StatusOK
	responses, _ := op["responses"].(map[string]any)
	code := ""
	for _, k := range sortedKeys(responses) {
		if strings.HasPrefix(k, "2") {
			code = k
			break
		}
	}
	if code == "" {
		if _, ok := responses["default"]; ok {
			code = "default"
		}
	}
	if n, err := strconv.Atoi(strings.ReplaceAll(strings.ToUpper(code), "X", "0")); err == nil && n >= 200 {
		o.status = n
	}
	if res, ok := g.resolve(responses[code]).(map[string]any); ok {
		content, _ := res["content"].(map[string]any)
		if mt, media := jsonMedia(content); media != nil {
			o.resContentType = mt
			o.resSchema = media["schema"]
			if ex, ok := firstExample(media); ok {
				o.resExample, o.hasResEx = ex, true
			} else if schema := g.resolve(media["schema"]); schema != nil {
				o.resExample, o.hasResEx = g.stub(schema, false, 0, nil), true
			}
		} else if len(content) > 0 {
			mt := sortedKeys(content)[0]
			o.resContentType = mt
			media, _ := content[mt].(map[string]any)
			if ex, ok := firstExample(media); ok {
				if s, ok := ex.(string); ok {
					o.resExample, o.hasResEx = s, true
				}
			}
		}
	}

	g.security(op, &o)
	return o
}

// security picks up bearer auth and api keys sent in headers, the schemes a
// rest file can express
func (g *gen) security(op map[string]any, o *operation) {
	reqs, ok := op["security"].([]any)
	if !ok {
		reqs, _ = g.doc["security"].([]any)
	}
	comps, _ := g.doc["components"].(map[string]any)
	schemes, _ := comps["securitySchemes"].(map[string]any)
	// any one requirement satisfies, the first one is what the client sends
	for _, r := range reqs {
		req, _ := r.(map[string]any)
		for _, name := range sortedKeys(req) {
			s, _ := g.resolve(schemes[name]).(map[string]any)
			switch {
			case s["type"] == "http" && strings.EqualFold(fmt.Sprint(s["scheme"]), "bearer"),
				s["type"] == "oauth2", s["type"] == "openIdConnect":
				o.bearer = true
			case s["type"] == "apiKey" && s["in"] == "header":
				if n, ok := s["name"].(string); ok {
					o.apiKeys = append(o.apiKeys, n)
				}
			}
		}
		if o.bearer || len(o.apiKeys) > 0 {
			return
		}
	}
}

// resolve follows local $refs ("#/components/...")
func (g *gen) resolve(v any) any {
	for range 32 {
		m, ok := v.(map[string]any)
		if !ok {
			return v
		}
		ref, ok := m["$ref"].(string)
		if !ok {
			return v
		}
		v = g.pointer(ref)
	}
	return nil
}

func (g *gen) pointer(ref string) any {
	if !strings.HasPrefix(ref, "#/") {
		return nil // external refs aren't followed
	}
	var cur any = g.doc
	for _, part := range strings.Split(ref[2:], "/") {
		part = strings.NewReplacer("~1", "/", "~0", "~").Replace(part)
		if u, err := url.PathUnescape(part); err == nil {
			part = u
		}
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[part]
	}
	return cur
}

func jsonMedia(content map[string]any) (string, map[string]any) {
	for _, mt := range sortedKeys(content) {
		base, _, _ := strings.Cut(mt, ";")
		if base == "application/json" || strings.HasSuffix(base, "+json") {
			m, _ := content[mt].(map[string]any)
			if m == nil {
				m = map[string]any{}
			}
			return mt, m
		}
	}
	return "", nil
}

// firstExample reads `example`, or the first of `examples` (a map of
// example objects on parameters/media, a list on schemas)
func firstExample(m map[string]any) (any, bool) {
	if m == nil {
		return nil, false
	}
	if ex, ok := m["example"]; ok {
		return ex, true
	}
	switch exs := m["examples"].(type) {
	case map[string]any:
		for _, k := range sortedKeys(exs) {
			if e, ok := exs[k].(map[string]any); ok {
				if v, ok := e["value"]; ok {
					return v, true
				}
			}
		}
	case []any:
		if len(exs) > 0 {
			return exs[0], true
		}
	}
	return nil, false
}

/*** schema -> matcher ***/

// matcherCtx collects what one expect block needs while walking schemas
type matcherCtx struct {
	// additional is set when any object allows unlisted properties, expect
	// only has one switch for the whole body
	additional bool
	// response schemas skip writeOnly properties, request ones readOnly
	response bool
}

func (c *matcherCtx) mode() string {
	if c.response {
		return "res"
	}
	return "req"
}

// matcher writes a schema as an expect matcher expression. Refs to
// components.schemas become schemas.<name>, defined once in the schemas block.
func (g *gen) matcher(schema any, ctx *matcherCtx, depth int, refs []string) hclwrite.Tokens {
	if depth > maxDepth {
		return is("any")
	}
	if m, ok := schema.(map[string]any); ok {
		if ref, ok := m["$ref"].(string); ok && strings.HasPrefix(ref, schemaPrefix) {
			return g.schemaRef(ref, ctx)
		}
		if ref, ok := m["$ref"].(string); ok {
			for _, r := range refs {
				if r == ref {
					return is("any") // recursive schema
				}
			}
			refs = append(refs, ref)
		}
	}
	s, ok := g.resolve(schema).(map[string]any)
	if !ok || len(s) == 0 {
		return is("any")
	}
	if c, ok := s["const"]; ok {
		return literal(c)
	}
	if enum, ok := s["enum"].([]any); ok && len(enum) > 0 {
		return enumMatcher(enum)
	}
	if all, ok := s["allOf"].([]any); ok {
		if merged, ok := g.mergeAllOf(all); ok {
			return g.matcher(merged, ctx, depth+1, refs)
		}
		return is("any")
	}
	for _, k := range []string{"oneOf", "anyOf"} {
		if alts, ok := s[k].([]any); ok {
			if len(alts) == 1 {
				return g.matcher(alts[0], ctx, depth+1, refs)
			}
			return is("any") // matchers have no unions
		}
	}

	typ, nullable := schemaType(s)
	if nullable {
		return is("any") // matchers can't say "x or null"
	}
	switch typ {
	case "string":
		switch s["format"] {
		case "uuid":
			return is("uuid")
		case "email":
			return is("email")
		case "uri", "url":
			return is("url")
		}
		if p, ok := s["pattern"].(string); ok && validRE2(p) {
			return isCall("regex:" + p)
		}
		return is("string")
	case "integer":
		return is("int")
	case "number":
		return is("number")
	case "boolean":
		return is("bool")
	case "null":
		return is("null")
	case "array":
		if items, ok := s["items"]; ok {
			inner := g.matcher(items, ctx, depth+1, refs)
			if string(inner.Bytes()) != "is.any" {
				return fn("is::array_of", inner)
			}
		}
		return is("array")
	case "object", "":
		props, _ := s["properties"].(map[string]any)
		if typ == "" && props == nil {
			return is("any")
		}
		if ap, ok := s["additionalProperties"].(bool); !ok || ap {
			ctx.additional = true
		}
		required := stringSet(s["required"])
		var attrs []hclwrite.ObjectAttrTokens
		for _, k := range sortedKeys(props) {
			p, _ := g.resolve(props[k]).(map[string]any)
			ro, _ := p["readOnly"].(bool)
			wo, _ := p["writeOnly"].(bool)
			if (ro && !ctx.response) || (wo && ctx.response) {
				continue
			}
			m := g.matcher(props[k], ctx, depth+1, refs)
			if !required[k] {
				m = fn("is::optional", m)
			}
			attrs = append(attrs, hclwrite.ObjectAttrTokens{Name: key(k), Value: m})
		}
		if len(attrs) == 0 {
			return is("object")
		}
		return hclwrite.TokensForObject(attrs)
	}
	return is("any")
}

// schemaRef points at a components.schemas entry, defining it first. A ref
// back into a schema that's still being defined would be a cycle, which the
// parser rejects, so the loop is cut with is.any.
func (g *gen) schemaRef(ref string, ctx *matcherCtx) hclwrite.Tokens {
	key := ref + "|" + ctx.mode()
	switch g.schemaState[key] {
	case visiting:
		return is("any")
	case 0:
		g.defineSchema(ref, ctx.response)
	}
	def := g.schemaDefs[key]
	if def.additional {
		ctx.additional = true
	}
	return traversal("schemas", def.name)
}

const (
	visiting = 1
	done     = 2
)

func (g *gen) defineSchema(ref string, response bool) {
	mode := map[bool]string{true: "res", false: "req"}[response]
	key := ref + "|" + mode
	g.schemaState[key] = visiting
	sub := &matcherCtx{response: response}
	toks := g.matcher(g.pointer(ref), sub, 0, nil)
	g.schemaState[key] = done
	def := &schemaDef{tokens: toks, additional: sub.additional}

	base := schemaName(ref)
	if !response {
		// most schemas read the same both ways, share the name when they do
		resKey := ref + "|res"
		if g.schemaState[resKey] == 0 {
			g.defineSchema(ref, true)
		}
		if res := g.schemaDefs[resKey]; res != nil && bytes.Equal(res.tokens.Bytes(), toks.Bytes()) {
			g.schemaDefs[key] = res
			return
		}
		base += "_input"
	}
	def.name = base
	for n := 2; g.schemaNames[def.name] != nil; n++ {
		def.name = fmt.Sprintf("%s_%d", base, n)
	}
	g.schemaDefs[key] = def
	g.schemaNames[def.name] = def
}

// schemaName: "#/components/schemas/PublicCreateEndpointBody" ->
// "public_create_endpoint_body"
func schemaName(ref string) string {
	name := strings.NewReplacer("~1", "/", "~0", "~").Replace(strings.TrimPrefix(ref, schemaPrefix))
	var b strings.Builder
	runes := []rune(name)
	for i, r := range runes {
		switch {
		case unicode.IsUpper(r):
			// a new word starts at an upper case letter after a lower case one
			// or a digit, and at the last capital of an acronym (HTTPError)
			if i > 0 && (unicode.IsLower(runes[i-1]) || unicode.IsDigit(runes[i-1]) ||
				(i+1 < len(runes) && unicode.IsLower(runes[i+1]) && unicode.IsUpper(runes[i-1]))) {
				b.WriteByte('_')
			}
			b.WriteRune(unicode.ToLower(r))
		case r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := strings.Trim(regexp.MustCompile(`_+`).ReplaceAllString(b.String(), "_"), "_")
	if out == "" || unicode.IsDigit(rune(out[0])) {
		out = "s_" + out
	}
	return out
}

// bare reports whether a matcher is a single is.* rule, following schemas.x
// references. The server checks a bare top level body matcher against the
// raw body text instead of the decoded json, so those are left out.
func (g *gen) bare(toks hclwrite.Tokens) bool {
	b := string(toks.Bytes())
	if strings.HasPrefix(b, "is.") {
		return true
	}
	if name, ok := strings.CutPrefix(b, "schemas."); ok {
		if def := g.schemaNames[name]; def != nil {
			return g.bare(def.tokens)
		}
	}
	return false
}

// mergeAllOf flattens allOf object schemas into one, the common way specs
// compose (base + extension)
func (g *gen) mergeAllOf(parts []any) (map[string]any, bool) {
	if len(parts) == 1 {
		m, ok := g.resolve(parts[0]).(map[string]any)
		return m, ok
	}
	props := map[string]any{}
	var required []any
	additional := false
	for _, part := range parts {
		p, ok := g.resolve(part).(map[string]any)
		if !ok {
			return nil, false
		}
		if inner, ok := p["allOf"].([]any); ok {
			if p, ok = g.mergeAllOf(inner); !ok {
				return nil, false
			}
		}
		if t, _ := schemaType(p); t != "object" && t != "" {
			return nil, false
		}
		pp, _ := p["properties"].(map[string]any)
		for k, v := range pp {
			props[k] = v
		}
		r, _ := p["required"].([]any)
		required = append(required, r...)
		if ap, ok := p["additionalProperties"].(bool); !ok || ap {
			additional = true
		}
	}
	return map[string]any{
		"type": "object", "properties": props, "required": required,
		"additionalProperties": additional,
	}, true
}

// schemaType returns the schema's type and whether null is also allowed
// (3.1 type arrays, 3.0 nullable)
func schemaType(s map[string]any) (string, bool) {
	nullable, _ := s["nullable"].(bool)
	switch t := s["type"].(type) {
	case string:
		return t, nullable
	case []any:
		var types []string
		for _, v := range t {
			if v == "null" {
				nullable = true
			} else if str, ok := v.(string); ok {
				types = append(types, str)
			}
		}
		if len(types) == 1 {
			return types[0], nullable
		}
		if len(types) == 0 && nullable {
			return "null", false
		}
		return "", true // a real union, treat like any
	}
	if _, ok := s["properties"]; ok {
		return "object", nullable
	}
	if _, ok := s["items"]; ok {
		return "array", nullable
	}
	return "", nullable
}

func enumMatcher(enum []any) hclwrite.Tokens {
	if len(enum) == 1 {
		return literal(enum[0])
	}
	alts := make([]string, 0, len(enum))
	for _, v := range enum {
		s, ok := v.(string)
		if !ok {
			return is("any") // only string enums can be a regex
		}
		alts = append(alts, regexp.QuoteMeta(s))
	}
	return isCall("regex:^(" + strings.Join(alts, "|") + ")$")
}

// validRE2 drops patterns go can't compile (lookarounds, backrefs), the
// matcher would fail at parse time otherwise
func validRE2(p string) bool {
	_, err := regexp.Compile(p)
	return err == nil
}

/*** example values ***/

// stub builds an example value from a schema. requests only fill required
// properties (and never readOnly ones), responses fill every property.
func (g *gen) stub(schema any, request bool, depth int, refs []string) any {
	if depth > maxStubDepth {
		return nil
	}
	if m, ok := schema.(map[string]any); ok {
		if ref, ok := m["$ref"].(string); ok {
			for _, r := range refs {
				if r == ref {
					return nil
				}
			}
			refs = append(refs, ref)
		}
	}
	s, ok := g.resolve(schema).(map[string]any)
	if !ok {
		return nil
	}
	if ex, ok := firstExample(s); ok {
		return ex
	}
	for _, k := range []string{"const", "default"} {
		if v, ok := s[k]; ok {
			return v
		}
	}
	if enum, ok := s["enum"].([]any); ok && len(enum) > 0 {
		return enum[0]
	}
	if all, ok := s["allOf"].([]any); ok {
		if merged, ok := g.mergeAllOf(all); ok {
			return g.stub(merged, request, depth+1, refs)
		}
	}
	for _, k := range []string{"oneOf", "anyOf"} {
		if alts, ok := s[k].([]any); ok && len(alts) > 0 {
			return g.stub(alts[0], request, depth+1, refs)
		}
	}

	typ, _ := schemaType(s)
	switch typ {
	case "string":
		switch s["format"] {
		case "uuid":
			return "00000000-0000-4000-8000-000000000000"
		case "email":
			return "user@example.com"
		case "uri", "url":
			return "https://example.com"
		case "date-time":
			return "1970-01-01T00:00:00Z"
		case "date":
			return "1970-01-01"
		}
		if p, ok := s["pattern"].(string); ok {
			if ex, ok := patternExample(p); ok {
				return ex
			}
		}
		return "string"
	case "integer", "number":
		if min, ok := s["minimum"].(json.Number); ok {
			return min
		}
		return json.Number("0")
	case "boolean":
		return false
	case "null":
		return nil
	case "array":
		if items, ok := s["items"]; ok {
			if v := g.stub(items, request, depth+1, refs); v != nil {
				return []any{v}
			}
		}
		return []any{}
	case "object", "":
		props, _ := s["properties"].(map[string]any)
		if typ == "" && props == nil {
			return nil
		}
		required := stringSet(s["required"])
		out := map[string]any{}
		for _, k := range sortedKeys(props) {
			p, _ := g.resolve(props[k]).(map[string]any)
			ro, _ := p["readOnly"].(bool)
			wo, _ := p["writeOnly"].(bool)
			if (ro && request) || (wo && !request) {
				continue
			}
			// optional fields only when the spec author gave them an example,
			// filling in every optional field everywhere is mostly noise
			if _, hasEx := firstExample(p); !required[k] && !hasEx {
				continue
			}
			if v := g.stub(props[k], request, depth+1, refs); v != nil || required[k] {
				out[k] = v
			}
		}
		return out
	}
	return nil
}

// patternExample builds a string the pattern matches: the first branch of
// every alternation, the fewest repeats (at least one for +), a letter or
// digit for character classes. It's checked against the pattern before use.
func patternExample(pattern string) (string, bool) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", false
	}
	parsed, err := syntax.Parse(pattern, syntax.Perl)
	if err != nil {
		return "", false
	}
	var b strings.Builder
	if !writeExample(&b, parsed.Simplify()) {
		return "", false
	}
	if !re.MatchString(b.String()) {
		return "", false
	}
	return b.String(), true
}

func writeExample(b *strings.Builder, re *syntax.Regexp) bool {
	switch re.Op {
	case syntax.OpLiteral:
		b.WriteString(string(re.Rune))
	case syntax.OpCharClass:
		b.WriteRune(classRune(re.Rune))
	case syntax.OpAnyChar, syntax.OpAnyCharNotNL:
		b.WriteRune('a')
	case syntax.OpCapture:
		return writeExample(b, re.Sub[0])
	case syntax.OpPlus:
		return writeExample(b, re.Sub[0])
	case syntax.OpRepeat:
		for range re.Min {
			if !writeExample(b, re.Sub[0]) {
				return false
			}
		}
	case syntax.OpConcat:
		for _, sub := range re.Sub {
			if !writeExample(b, sub) {
				return false
			}
		}
	case syntax.OpAlternate:
		return writeExample(b, re.Sub[0])
	case syntax.OpNoMatch:
		return false
	}
	// star, quest, anchors, word boundaries and empty matches add nothing
	return true
}

// classRune prefers a readable character from a class's ranges
func classRune(ranges []rune) rune {
	for _, want := range []rune{'a', 'A', '0', '_', '-'} {
		for i := 0; i+1 < len(ranges); i += 2 {
			if ranges[i] <= want && want <= ranges[i+1] {
				return want
			}
		}
	}
	if len(ranges) > 0 {
		return ranges[0]
	}
	return 'a'
}

func (g *gen) paramValue(p param) string {
	v := p.example
	if !p.hasEx {
		v = g.stub(p.schema, true, 0, nil)
	}
	switch v := v.(type) {
	case nil:
		return ""
	case string:
		return v
	case []any:
		// arrays are sent as the first value, rest's query/headers are maps
		if len(v) > 0 {
			return fmt.Sprint(v[0])
		}
		return ""
	}
	return fmt.Sprint(v)
}

/*** writing hcl ***/

func (g *gen) write(ops []operation) []byte {
	f := hclwrite.NewEmptyFile()
	root := f.Body()

	info, _ := g.doc["info"].(map[string]any)
	title, _ := info["title"].(string)
	root.AppendUnstructuredTokens(comment(fmt.Sprintf("generated by rest --import from %q, edit away", title)))
	root.AppendNewline()

	if g.opts.Side != Server {
		base := "http://" + g.opts.Addr
		if g.opts.Side == Client {
			if servers, ok := g.doc["servers"].([]any); ok && len(servers) > 0 {
				if s, ok := servers[0].(map[string]any); ok {
					if u, ok := s["url"].(string); ok && strings.Contains(u, "://") {
						base = strings.TrimSuffix(u, "/")
					}
				}
			}
		}
		locals := root.AppendNewBlock("locals", nil).Body()
		locals.SetAttributeRaw("base", hclwrite.TokensForFunctionCall("env",
			hclwrite.TokensForValue(cty.StringVal("BASE_URL")),
			hclwrite.TokensForValue(cty.StringVal(base))))
		if anyOp(ops, func(o operation) bool { return o.bearer }) {
			locals.SetAttributeRaw("token", hclwrite.TokensForFunctionCall("env",
				hclwrite.TokensForValue(cty.StringVal("API_TOKEN")),
				hclwrite.TokensForValue(cty.StringVal("token"))))
		}
		if anyOp(ops, func(o operation) bool { return len(o.apiKeys) > 0 }) {
			locals.SetAttributeRaw("api_key", hclwrite.TokensForFunctionCall("env",
				hclwrite.TokensForValue(cty.StringVal("API_KEY")),
				hclwrite.TokensForValue(cty.StringVal("key"))))
		}
		root.AppendNewline()
	}

	// server and requests are written first so the schemas they use are known
	rest := hclwrite.NewEmptyFile().Body()
	if g.opts.Side != Client {
		srv := rest.AppendNewBlock("server", nil).Body()
		srv.SetAttributeValue("address", cty.StringVal(g.opts.Addr))
		for _, o := range ops {
			srv.AppendNewline()
			g.writeHandler(srv, o)
		}
		rest.AppendNewline()
	}
	if g.opts.Side != Server {
		for _, o := range ops {
			g.writeRequest(rest, o)
			rest.AppendNewline()
		}
	}

	if len(g.schemaNames) > 0 {
		root.AppendUnstructuredTokens(comment("from the spec's components.schemas, *_input variants leave out readOnly fields"))
		schemas := root.AppendNewBlock("schemas", nil).Body()
		for _, name := range sortedKeys(g.schemaNames) {
			schemas.SetAttributeRaw(name, g.schemaNames[name].tokens)
		}
		root.AppendNewline()
	}
	root.AppendUnstructuredTokens(rest.BuildTokens(nil))
	return hclwrite.Format(f.Bytes())
}

var goWildcard = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// muxPattern turns an openapi path into a go ServeMux pattern. Parameter
// names that aren't go identifiers are renamed, and paths with a parameter
// inside a segment (/{id}.json) can't be expressed.
func muxPattern(path string) (string, map[string]string, error) {
	names := map[string]string{}
	segs := strings.Split(path, "/")
	for i, seg := range segs {
		if !strings.Contains(seg, "{") {
			continue
		}
		if !strings.HasPrefix(seg, "{") || !strings.HasSuffix(seg, "}") || strings.Count(seg, "{") != 1 {
			return "", nil, fmt.Errorf("parameter inside a path segment (%s)", seg)
		}
		name := seg[1 : len(seg)-1]
		wild := name
		if !goWildcard.MatchString(wild) {
			wild = strings.Map(func(r rune) rune {
				if r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
					return r
				}
				return '_'
			}, wild)
			if wild == "" || unicode.IsDigit(rune(wild[0])) {
				wild = "p_" + wild
			}
		}
		names[name] = wild
		segs[i] = "{" + wild + "}"
	}
	return strings.Join(segs, "/"), names, nil
}

// register reports why go's ServeMux would refuse the pattern (invalid or
// conflicting with an earlier handler), the server would panic on it
func (g *gen) register(pattern string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			msg := registeredAt.ReplaceAllString(fmt.Sprint(r), "")
			if m := conflictSays.FindStringSubmatch(msg); m != nil {
				if m[1] == m[2] {
					msg = "same route as an earlier operation"
				} else {
					msg = "conflicts with " + m[2] + " in go's router"
				}
			}
			err = errors.New(msg)
		}
	}()
	g.routes.HandleFunc(pattern, func(http.ResponseWriter, *http.Request) {})
	return nil
}

// route works out the handler pattern, or why there can't be one
func (g *gen) route(o *operation) {
	pattern, names, err := muxPattern(o.path)
	if err == nil {
		err = g.register(o.method + " " + pattern)
	}
	if err != nil {
		o.skip = err.Error()
		return
	}
	o.pattern, o.names = pattern, names
}

// go's conflict panic names this file and line, which means nothing in the
// generated file, and then explains the conflict at length
var (
	registeredAt = regexp.MustCompile(` \(registered at [^)]*\)`)
	conflictSays = regexp.MustCompile(`(?s)^pattern "([^"]*)" conflicts with pattern "([^"]*)":.*$`)
)

func (g *gen) writeHandler(srv *hclwrite.Body, o operation) {
	if o.skip != "" {
		srv.AppendUnstructuredTokens(comment(fmt.Sprintf("skipped %s %s: %s", o.method, o.path, o.skip)))
		return
	}
	pattern, names := o.pattern, o.names

	h := srv.AppendNewBlock("handler", []string{o.method, pattern}).Body()

	ctx := &matcherCtx{}
	groups := map[string][]hclwrite.ObjectAttrTokens{}
	for _, p := range o.params {
		name := p.name
		if p.in == "path" {
			name = names[p.name]
		}
		if p.in == "header" && reservedHeader(name) {
			continue
		}
		m := g.matcher(p.schema, ctx, 0, nil)
		if !p.required {
			m = fn("is::optional", m)
		}
		groups[p.in] = append(groups[p.in], hclwrite.ObjectAttrTokens{Name: key(name), Value: m})
	}
	for _, k := range sortedKeys(o.fixedQuery) {
		groups["query"] = append(groups["query"], hclwrite.ObjectAttrTokens{
			Name: key(k), Value: hclwrite.TokensForValue(cty.StringVal(o.fixedQuery.Get(k))),
		})
	}
	if o.bearer {
		groups["header"] = append(groups["header"], hclwrite.ObjectAttrTokens{
			Name: key("Authorization"), Value: isCall("regex:^Bearer .+"),
		})
	}
	for _, k := range o.apiKeys {
		groups["header"] = append(groups["header"], hclwrite.ObjectAttrTokens{Name: key(k), Value: is("not_empty")})
	}

	var body hclwrite.Tokens
	if o.hasBody {
		body = g.matcher(o.body, ctx, 0, nil)
		if g.bare(body) {
			body = nil
		}
	}
	if len(groups) > 0 || body != nil {
		e := h.AppendNewBlock("expect", nil).Body()
		if ctx.additional {
			e.SetAttributeValue("additional", cty.True)
		}
		for _, in := range []string{"path", "query", "header"} {
			if attrs := groups[in]; len(attrs) > 0 {
				name := in
				if in == "header" {
					name = "headers"
				}
				e.SetAttributeRaw(name, hclwrite.TokensForObject(attrs))
			}
		}
		if body != nil {
			e.SetAttributeRaw("body", body)
		}
	}
	if o.bodyNote != "" {
		h.AppendUnstructuredTokens(comment(o.bodyNote))
	}

	res := h.AppendNewBlock("response", nil).Body()
	res.SetAttributeValue("status", cty.NumberIntVal(int64(o.status)))
	if o.resContentType != "" && !strings.HasPrefix(o.resContentType, "application/json") {
		res.SetAttributeRaw("headers", hclwrite.TokensForObject([]hclwrite.ObjectAttrTokens{
			{Name: key("Content-Type"), Value: hclwrite.TokensForValue(cty.StringVal(o.resContentType))},
		}))
	}
	if o.hasResEx && o.resExample != nil {
		res.SetAttributeRaw("body", literal(o.resExample))
	}
}

func (g *gen) writeRequest(root *hclwrite.Body, o operation) {
	r := root.AppendNewBlock("request", []string{o.label}).Body()
	if o.skip != "" && g.opts.Side == Both {
		r.AppendUnstructuredTokens(comment("the generated server has no handler for this route (" + o.skip + ")"))
	}

	path := o.path
	query := []hclwrite.ObjectAttrTokens{}
	headers := []hclwrite.ObjectAttrTokens{}
	for _, p := range o.params {
		switch p.in {
		case "path":
			path = strings.ReplaceAll(path, "{"+p.name+"}", url.PathEscape(g.paramValue(p)))
		case "query":
			if p.required || p.hasEx {
				query = append(query, hclwrite.ObjectAttrTokens{Name: key(p.name), Value: hclwrite.TokensForValue(cty.StringVal(g.paramValue(p)))})
			}
		case "header":
			if (p.required || p.hasEx) && !reservedHeader(p.name) {
				headers = append(headers, hclwrite.ObjectAttrTokens{Name: key(p.name), Value: hclwrite.TokensForValue(cty.StringVal(g.paramValue(p)))})
			}
		}
	}
	for _, k := range sortedKeys(o.fixedQuery) {
		query = append(query, hclwrite.ObjectAttrTokens{Name: key(k), Value: hclwrite.TokensForValue(cty.StringVal(o.fixedQuery.Get(k)))})
	}
	for _, k := range o.apiKeys {
		headers = append(headers, hclwrite.ObjectAttrTokens{Name: key(k), Value: traversal("locals", "api_key")})
	}

	r.SetAttributeRaw("url", baseURL(path))
	if o.method != "GET" {
		r.SetAttributeValue("method", cty.StringVal(o.method))
	}
	if o.bearer {
		r.SetAttributeRaw("bearer_token", traversal("locals", "token"))
	}
	if len(headers) > 0 {
		r.SetAttributeRaw("headers", hclwrite.TokensForObject(headers))
	}
	if len(query) > 0 {
		r.SetAttributeRaw("query", hclwrite.TokensForObject(query))
	}
	if o.hasBody {
		ex := o.bodyExample
		if !o.hasBodyEx {
			ex = g.stub(o.body, true, 0, nil)
		}
		if ex == nil {
			ex = map[string]any{}
		}
		r.SetAttributeRaw("body", literal(ex))
	}
	if o.bodyNote != "" {
		r.AppendUnstructuredTokens(comment(o.bodyNote))
	}
	// check responses against the spec's schema, useful against the real api
	var resBody hclwrite.Tokens
	resCtx := &matcherCtx{response: true}
	if o.resSchema != nil {
		resBody = g.matcher(o.resSchema, resCtx, 0, nil)
		if g.bare(resBody) {
			resBody = nil
		}
	}
	if resBody == nil {
		r.SetAttributeValue("expect", cty.NumberIntVal(int64(o.status)))
		return
	}
	e := r.AppendNewBlock("expect", nil).Body()
	e.SetAttributeValue("status", cty.NumberIntVal(int64(o.status)))
	if resCtx.additional {
		e.SetAttributeValue("additional", cty.True)
	}
	e.SetAttributeRaw("body", resBody)
}

/*** token helpers ***/

const (
	hclsyntaxIdent   = hclsyntax.TokenIdent
	hclsyntaxDot     = hclsyntax.TokenDot
	hclsyntaxComment = hclsyntax.TokenComment
)

// is.<rule>
func is(rule string) hclwrite.Tokens {
	return hclwrite.Tokens{
		{Type: hclsyntaxIdent, Bytes: []byte("is")},
		{Type: hclsyntaxDot, Bytes: []byte(".")},
		{Type: hclsyntaxIdent, Bytes: []byte(rule)},
	}
}

// is("<rule>")
func isCall(rule string) hclwrite.Tokens {
	return hclwrite.TokensForFunctionCall("is", hclwrite.TokensForValue(cty.StringVal(rule)))
}

func fn(name string, args ...hclwrite.Tokens) hclwrite.Tokens {
	return hclwrite.TokensForFunctionCall(name, args...)
}

func traversal(parts ...string) hclwrite.Tokens {
	var toks hclwrite.Tokens
	for i, p := range parts {
		if i > 0 {
			toks = append(toks, &hclwrite.Token{Type: hclsyntaxDot, Bytes: []byte(".")})
		}
		toks = append(toks, &hclwrite.Token{Type: hclsyntaxIdent, Bytes: []byte(p)})
	}
	return toks
}

// baseURL writes "${locals.base}<path>" with path escaped for hcl
func baseURL(path string) hclwrite.Tokens {
	lit := hclwrite.TokensForValue(cty.StringVal(path)).Bytes() // "..." with ${ escaped
	src := `"${locals.base}` + string(lit[1:])
	return hclwrite.Tokens{{Type: hclsyntaxIdent, Bytes: []byte(src)}}
}

var hclIdent = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)

// key writes an object key bare when hcl allows it, quoted otherwise
func key(k string) hclwrite.Tokens {
	if hclIdent.MatchString(k) && k != "null" && k != "true" && k != "false" {
		return hclwrite.TokensForIdentifier(k)
	}
	return hclwrite.TokensForValue(cty.StringVal(k))
}

// literal writes a decoded json value as an hcl literal
func literal(v any) hclwrite.Tokens {
	switch v := v.(type) {
	case map[string]any:
		attrs := make([]hclwrite.ObjectAttrTokens, 0, len(v))
		for _, k := range sortedKeys(v) {
			attrs = append(attrs, hclwrite.ObjectAttrTokens{Name: key(k), Value: literal(v[k])})
		}
		return hclwrite.TokensForObject(attrs)
	case []any:
		elems := make([]hclwrite.Tokens, 0, len(v))
		for _, e := range v {
			elems = append(elems, literal(e))
		}
		return hclwrite.TokensForTuple(elems)
	case string:
		return hclwrite.TokensForValue(cty.StringVal(v))
	case json.Number:
		if n, err := cty.ParseNumberVal(v.String()); err == nil {
			return hclwrite.TokensForValue(n)
		}
	case bool:
		return hclwrite.TokensForValue(cty.BoolVal(v))
	case float64:
		return hclwrite.TokensForValue(cty.NumberFloatVal(v))
	}
	return hclwrite.TokensForValue(cty.NullVal(cty.DynamicPseudoType))
}

func comment(text string) hclwrite.Tokens {
	text = strings.ReplaceAll(text, "\n", " ")
	return hclwrite.Tokens{{Type: hclsyntaxComment, Bytes: []byte("# " + text + "\n")}}
}

/*** misc ***/

func reservedHeader(name string) bool {
	switch strings.ToLower(name) {
	case "accept", "content-type", "authorization":
		return true
	}
	return false
}

func anyOp(ops []operation, f func(operation) bool) bool {
	for _, o := range ops {
		if f(o) {
			return true
		}
	}
	return false
}

func stringSet(v any) map[string]bool {
	out := map[string]bool{}
	list, _ := v.([]any)
	for _, s := range list {
		if str, ok := s.(string); ok {
			out[str] = true
		}
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
