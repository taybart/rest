package exports

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/taybart/rest/request"
	"github.com/taybart/rest/server"
)

// methods a handler without a method ("*") is listed under
var anyMethods = []string{"get", "post", "put", "patch", "delete"}

// openapi ignores header parameters with these names, they're described by
// requestBody/responses/security instead
var reservedHeaders = map[string]bool{"accept": true, "content-type": true, "authorization": true}

// ToOpenAPI writes the server block's handlers as an OpenAPI 3.1 spec. The
// handler's expect block becomes parameters and the request body schema, its
// response block becomes the documented response. Shapes that came from the
// file's schemas blocks (named, see file.Parser.SchemaValues) become
// components.schemas and are $ref'd.
func ToOpenAPI(w io.Writer, filename string, cfg server.Config, named map[string]any) error {
	components := newComponentSet(named, func(def string) map[string]any {
		return map[string]any{"$ref": "#/components/schemas/" + def}
	})
	spec := map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":   strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename)),
			"version": "0.0.0",
		},
	}
	if cfg.Addr != "" {
		scheme := "http"
		if cfg.TLS != "" || cfg.Tailscale {
			scheme = "https"
		}
		spec["servers"] = []any{map[string]any{"url": scheme + "://" + cfg.Addr}}
	}

	paths := map[string]any{}
	for _, h := range cfg.Handlers {
		if h.WS {
			continue // websockets aren't describable in openapi
		}
		path, wildcards := openapiPath(h.Path)
		methods := []string{strings.ToLower(h.Method)}
		if h.Method == "" || h.Method == "*" {
			methods = anyMethods
		}
		item, _ := paths[path].(map[string]any)
		if item == nil {
			item = map[string]any{}
			paths[path] = item
		}
		for _, m := range methods {
			op, err := openapiOperation(m, path, wildcards, h, components)
			if err != nil {
				return fmt.Errorf("%s %s: %w", strings.ToUpper(m), h.Path, err)
			}
			item[m] = op
		}
	}
	spec["paths"] = paths
	if len(components.defs) > 0 {
		schemas := map[string]any{}
		for name, def := range components.defs {
			schemas[name] = def
		}
		spec["components"] = map[string]any{"schemas": schemas}
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(spec)
}

var pathWildcard = regexp.MustCompile(`\{([^}]*)\}`)

// openapiPath converts a go ServeMux pattern: "/files/{rest...}" ->
// "/files/{rest}", "/{$}" -> "/". It returns the wildcards in order, and
// which ones matched the rest of the path.
func openapiPath(pattern string) (string, []wildcard) {
	var ws []wildcard
	path := pathWildcard.ReplaceAllStringFunc(pattern, func(m string) string {
		name := m[1 : len(m)-1]
		if name == "$" {
			return ""
		}
		rest := strings.HasSuffix(name, "...")
		name = strings.TrimSuffix(name, "...")
		ws = append(ws, wildcard{name: name, rest: rest})
		return "{" + name + "}"
	})
	if path == "" {
		path = "/"
	}
	return path, ws
}

type wildcard struct {
	name string
	rest bool
}

func openapiOperation(method, path string, wildcards []wildcard, h *server.Handler, components *componentSet) (map[string]any, error) {
	op := map[string]any{"operationId": operationID(method, path)}
	params := []any{}
	expect := h.Expect
	if expect == nil {
		expect = &server.Expect{}
	}

	// every wildcard has to be declared, expect.path only narrows the schema
	for _, w := range wildcards {
		schema := map[string]any{"type": "string"}
		if m, ok := expect.Path[w.name]; ok {
			schema = request.MatcherSchema(m, components.opts(request.SchemaOptions{Param: true}))
		}
		p := map[string]any{"name": w.name, "in": "path", "required": true, "schema": schema}
		if w.rest {
			p["description"] = "matches the rest of the path, slashes included"
		}
		params = append(params, p)
	}
	params = append(params, openapiParams("query", expect.Query, components)...)
	params = append(params, openapiParams("header", expect.Headers, components)...)
	if len(params) > 0 {
		op["parameters"] = params
	}

	if expect.Body != "" {
		// body is json with matcher tokens inside, or a single matcher token
		// (body = is::array_of(...))
		body, err := request.DecodeExpectBody(expect.Body)
		if err != nil {
			return nil, fmt.Errorf("expect body: %w", err)
		}
		op["requestBody"] = map[string]any{
			"required": !request.IsOptional(body),
			"content": map[string]any{
				requestMediaType(expect.Headers): map[string]any{
					"schema": request.MatcherSchema(body, components.opts(request.SchemaOptions{Additional: expect.Additional})),
				},
			},
		}
	}

	responses := map[string]any{}
	if h.Response != nil {
		status := h.Response.Status
		if status == 0 {
			status = http.StatusOK
		}
		responses[strconv.Itoa(status)] = openapiResponse(h.Response, status)
	} else {
		// fn/proxy handlers decide their response at runtime
		desc := "handled by fn"
		if h.Proxy != nil {
			desc = "proxied"
		}
		responses["default"] = map[string]any{"description": desc}
	}
	if h.Expect != nil {
		responses["400"] = map[string]any{
			"description": "request didn't match the handler's expect block",
			"content": map[string]any{"application/json": map[string]any{
				"schema": map[string]any{
					"type":       "object",
					"properties": map[string]any{"error": map[string]any{"type": "string"}},
					"required":   []string{"error"},
				},
			}},
		}
	}
	op["responses"] = responses
	return op, nil
}

func openapiParams(in string, matchers map[string]string, components *componentSet) []any {
	params := []any{}
	for _, name := range sortedKeys(matchers) {
		if in == "header" && reservedHeaders[strings.ToLower(name)] {
			continue
		}
		m := matchers[name]
		params = append(params, map[string]any{
			"name":     name,
			"in":       in,
			"required": !request.IsOptional(m),
			"schema":   request.MatcherSchema(m, components.opts(request.SchemaOptions{Param: true})),
		})
	}
	return params
}

// requestMediaType uses an exact Content-Type from expect.headers, json
// otherwise (expect bodies are always json)
func requestMediaType(headers map[string]string) string {
	for k, v := range headers {
		if strings.EqualFold(k, "content-type") && !strings.HasPrefix(v, request.IsPrefix) {
			return v
		}
	}
	return "application/json"
}

func openapiResponse(r *server.Response, status int) map[string]any {
	res := map[string]any{"description": http.StatusText(status)}
	if res["description"] == "" {
		res["description"] = "status " + strconv.Itoa(status)
	}

	headers := map[string]any{}
	contentType := ""
	for _, k := range sortedKeys(r.Headers) {
		if strings.EqualFold(k, "content-type") {
			contentType = r.Headers[k]
			continue
		}
		headers[k] = map[string]any{"schema": map[string]any{"type": "string"}, "example": r.Headers[k]}
	}
	if len(headers) > 0 {
		res["headers"] = headers
	}

	if len(r.Body) > 0 && string(r.Body) != "null" {
		var example any
		if err := json.Unmarshal(r.Body, &example); err != nil {
			example = string(r.Body)
		}
		if contentType == "" {
			contentType = "application/json"
			if _, isString := example.(string); isString {
				contentType = "text/plain"
			}
		}
		res["content"] = map[string]any{contentType: map[string]any{"example": example}}
	}
	return res
}

// operationID: "get", "/users/{id}/posts" -> "getUsersIdPosts"
func operationID(method, path string) string {
	words := strings.FieldsFunc(path, func(r rune) bool {
		return !(r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)))
	})
	var b strings.Builder
	b.WriteString(method)
	for _, w := range words {
		b.WriteString(strings.ToUpper(w[:1]) + w[1:])
	}
	return b.String()
}
