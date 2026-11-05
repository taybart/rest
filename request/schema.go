package request

import (
	"encoding/json"
	"sort"
	"strings"
)

// SchemaOptions changes how matchers translate to JSON Schema
type SchemaOptions struct {
	// Additional mirrors expect { additional = true }: objects allow
	// properties that weren't listed, at any depth
	Additional bool
	// Param is set for path/query/header values, which are always strings on
	// the wire, so not_empty only has to mean a non-empty string
	Param bool

	// Ref, when set, is used for matchers from a schemas block (schemas.x)
	// instead of expanding them, so exports can keep the name
	Ref func(name string) map[string]any
}

// DecodeExpectBody turns an expect body as the parser stores it (json with
// matcher tokens inside, or a single matcher token) into a value for
// MatcherSchema
func DecodeExpectBody(body string) (any, error) {
	if strings.HasPrefix(body, IsPrefix) {
		return body, nil
	}
	var v any
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		return nil, err
	}
	return v, nil
}

// Patterns for matchers JSON Schema has no format for. They describe the
// same shapes the matchers accept, close enough for documentation.
const (
	uuidv4Pattern = `^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-4[0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`
	ulidPattern   = `^[0-7][0-9A-HJKMNP-TV-Za-hjkmnp-tv-z]{25}$`
	nanoidPattern = `^[A-Za-z0-9_-]{21}$`
	jwtPattern    = `^[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]*$`
	// is.url wants a scheme and a host, format uri alone allows mailto:x
	urlPattern = `^[A-Za-z][A-Za-z0-9+.-]*://[^/?#]+`
)

// MatcherSchema converts an expect matcher into JSON Schema (2020-12, as
// used by OpenAPI 3.1). expected is what an expect block decodes to: a
// literal, an is.* token string, or maps/lists of those from a json body.
func MatcherSchema(expected any, opts SchemaOptions) map[string]any {
	if name, inner, ok := CutNamed(expected); ok {
		if opts.Ref != nil {
			return opts.Ref(name)
		}
		return MatcherSchema(inner, opts)
	}
	switch exp := expected.(type) {
	case string:
		if rule, ok := strings.CutPrefix(exp, IsPrefix); ok {
			return ruleSchema(rule, opts)
		}
		return map[string]any{"const": exp}
	case nil:
		return map[string]any{"type": "null"}
	case bool, float64, json.Number:
		return map[string]any{"const": exp}
	case map[string]any:
		props := map[string]any{}
		required := []string{}
		for k, v := range exp {
			props[k] = MatcherSchema(v, opts)
			if !IsOptional(v) {
				required = append(required, k)
			}
		}
		sort.Strings(required)
		s := map[string]any{"type": "object", "properties": props}
		if len(required) > 0 {
			s["required"] = required
		}
		if !opts.Additional {
			s["additionalProperties"] = false
		}
		return s
	case []any:
		// a literal list in expect is an exact length tuple
		items := make([]any, len(exp))
		for i, v := range exp {
			items[i] = MatcherSchema(v, opts)
		}
		return map[string]any{
			"type":        "array",
			"prefixItems": items,
			"minItems":    len(exp),
			"maxItems":    len(exp),
		}
	}
	return map[string]any{}
}

func ruleSchema(rule string, opts SchemaOptions) map[string]any {
	if kind, inner, ok, err := cutCompound(rule); ok {
		if err != nil {
			return map[string]any{}
		}
		if kind == optionalRule {
			// optional only changes `required`, which the parent handles
			return MatcherSchema(inner, opts)
		}
		return map[string]any{"type": "array", "items": MatcherSchema(inner, opts)}
	}
	if pat, ok := cutRegexRule(rule); ok {
		return map[string]any{"type": "string", "pattern": pat}
	}
	str := func(k, v string) map[string]any { return map[string]any{"type": "string", k: v} }
	switch strings.ToLower(strings.TrimSpace(rule)) {
	case "string":
		return map[string]any{"type": "string"}
	case "number":
		return map[string]any{"type": "number"}
	case "int", "integer":
		return map[string]any{"type": "integer"}
	case "bool", "boolean":
		return map[string]any{"type": "boolean"}
	case "array", "list":
		return map[string]any{"type": "array"}
	case "object", "map":
		return map[string]any{"type": "object"}
	case "null", "nil":
		return map[string]any{"type": "null"}
	case "not_empty", "nonempty":
		if opts.Param {
			return map[string]any{"type": "string", "minLength": 1}
		}
		return map[string]any{"anyOf": []any{
			map[string]any{"type": "string", "minLength": 1},
			map[string]any{"type": "array", "minItems": 1},
			map[string]any{"type": "object", "minProperties": 1},
		}}
	case "uuid":
		return str("format", "uuid")
	case "uuidv4":
		return map[string]any{"type": "string", "format": "uuid", "pattern": uuidv4Pattern}
	case "ulid":
		return str("pattern", ulidPattern)
	case "nanoid":
		return str("pattern", nanoidPattern)
	case "jwt":
		return str("pattern", jwtPattern)
	case "email":
		return str("format", "email")
	case "url", "uri":
		return map[string]any{"type": "string", "format": "uri", "pattern": urlPattern}
	}
	// "any" and anything unknown accept every value
	return map[string]any{}
}

// IsOptional reports whether a matcher is wrapped in is::optional(...)
func IsOptional(expected any) bool {
	return isOptional(expected)
}
