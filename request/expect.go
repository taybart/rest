package request

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/mail"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
	restlua "github.com/taybart/rest/lua"
)

const IsPrefix = "\x00__rest_is__:"

var nanoidRegex = regexp.MustCompile(`^[A-Za-z0-9_-]{21}$`)

func IsToken(rule string) string {
	return IsPrefix + rule
}

// Compound rule prefixes. The text after the prefix is the JSON encoding of
// the wrapped matcher, which may be a token string, a literal or a structure.
const (
	optionalRule = "optional:"
	arrayOfRule  = "array_of:"
	// namedRule marks a matcher that came from a schemas block, so exports
	// can keep its name: named:<name>:<inner json>. Matching sees through it.
	namedRule = "named:"
)

// NamedToken wraps a JSON-encoded matcher with the schemas block name it was
// defined under.
func NamedToken(name, innerJSON string) string {
	return IsToken(namedRule + name + ":" + innerJSON)
}

// CutNamed reports whether expected is a named matcher (schemas.<name>) and
// returns its name and decoded matcher.
func CutNamed(expected any) (name string, inner any, ok bool) {
	s, isStr := expected.(string)
	if !isStr {
		return "", nil, false
	}
	rule, isRule := strings.CutPrefix(s, IsPrefix)
	if !isRule {
		return "", nil, false
	}
	after, isNamed := strings.CutPrefix(rule, namedRule)
	if !isNamed {
		return "", nil, false
	}
	name, innerJSON, found := strings.Cut(after, ":")
	if !found || json.Unmarshal([]byte(innerJSON), &inner) != nil {
		return "", nil, false
	}
	return name, inner, true
}

// unwrapNamed strips schema names, they don't change what matches
func unwrapNamed(expected any) any {
	for {
		_, inner, ok := CutNamed(expected)
		if !ok {
			return expected
		}
		expected = inner
	}
}

// OptionalToken wraps a JSON-encoded matcher so the value may be absent.
func OptionalToken(innerJSON string) string {
	return IsToken(optionalRule + innerJSON)
}

// ArrayOfToken wraps a JSON-encoded matcher that every array element must match.
func ArrayOfToken(innerJSON string) string {
	return IsToken(arrayOfRule + innerJSON)
}

// cutCompound splits a compound rule into its kind and decoded inner matcher.
func cutCompound(rule string) (kind string, inner any, ok bool, err error) {
	for _, k := range []string{optionalRule, arrayOfRule} {
		if after, found := strings.CutPrefix(rule, k); found {
			if err := json.Unmarshal([]byte(after), &inner); err != nil {
				return k, nil, true, fmt.Errorf("invalid inner matcher in %q: %w", strings.TrimSuffix(k, ":"), err)
			}
			return k, inner, true, nil
		}
	}
	return "", nil, false, nil
}

func isOptional(expected any) bool {
	s, ok := unwrapNamed(expected).(string)
	if !ok {
		return false
	}
	rule, ok := strings.CutPrefix(s, IsPrefix)
	return ok && strings.HasPrefix(rule, optionalRule)
}

// IsArrayOf reports whether expected is an is::array_of(...) matcher,
// optionally wrapped in is::optional(...).
func IsArrayOf(expected string) bool {
	if inner, ok := unwrapNamed(expected).(string); ok {
		expected = inner
	} else {
		return false
	}
	rule, ok := strings.CutPrefix(expected, IsPrefix)
	if !ok {
		return false
	}
	if after, ok := strings.CutPrefix(rule, optionalRule); ok {
		var inner string
		if json.Unmarshal([]byte(after), &inner) != nil {
			return false
		}
		return IsArrayOf(inner)
	}
	return strings.HasPrefix(rule, arrayOfRule)
}

func ValidateIsRule(rule string) error {
	if _, _, ok := CutNamed(IsPrefix + rule); ok {
		return nil
	}
	if _, _, ok, err := cutCompound(rule); ok {
		return err
	}
	switch strings.ToLower(strings.TrimSpace(rule)) {
	case "uuid", "uuidv4", "ulid", "nanoid", "jwt",
		"string", "number", "int", "integer",
		"bool", "boolean", "array", "list",
		"object", "map", "null", "nil",
		"any", "not_empty", "nonempty",
		"email", "url", "uri":
		return nil
	}
	if pat, ok := cutRegexRule(rule); ok {
		if _, err := regexp.Compile(pat); err != nil {
			return fmt.Errorf("invalid regex in is(%q): %w", rule, err)
		}
		return nil
	}
	return fmt.Errorf("unknown expectation matcher %q", rule)
}

func cutRegexRule(rule string) (string, bool) {
	if after, ok := strings.CutPrefix(rule, "regex:"); ok {
		return after, true
	}
	if after, ok := strings.CutPrefix(rule, "re:"); ok {
		return after, true
	}
	return "", false
}

func CheckExpectation(label string, expect *Expect, expectStatus int, statusCode int, headers http.Header, body string) error {
	if expect != nil {
		if expect.Status != 0 && statusCode != expect.Status {
			return fmt.Errorf(`request(%s): unexpected status code %d != %d`, label, expect.Status, statusCode)
		}
		if expect.Body != "" {
			if err := MatchExpectedBody(expect.Body, body, BodyOptions{Exact: expect.Exact, Additional: expect.Additional}); err != nil {
				return fmt.Errorf(`request(%s): %w`, label, err)
			}
		}
		if len(expect.Headers) != 0 {
			keys := make([]string, 0, len(expect.Headers))
			for k := range expect.Headers {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				if err := MatchStringValues("header "+k, expect.Headers[k], headers.Values(k)); err != nil {
					return fmt.Errorf(`request(%s): %w`, label, err)
				}
			}
		}
	} else if expectStatus != 0 {
		if statusCode != expectStatus {
			return fmt.Errorf(`request(%s): unexpected status code %d != %d`, label, expectStatus, statusCode)
		}
	}
	return nil
}

// MatchStringValues matches string values (headers, query params, path values)
// against an expectation. Values that look like numbers or bools are coerced
// for is.int/is.number/is.bool. Scalar rules pass if any value matches;
// is::array_of(...) requires every value to match.
func MatchStringValues(path, expected string, values []string) error {
	if name, inner, ok := CutNamed(expected); ok {
		innerStr, isStr := inner.(string)
		if !isStr {
			return fmt.Errorf("%s: cannot match schemas.%s (structured) against a string", path, name)
		}
		return MatchStringValues(path, innerStr, values)
	}
	rule, isRule := strings.CutPrefix(expected, IsPrefix)
	if len(values) == 0 {
		if isRule && strings.HasPrefix(rule, optionalRule) {
			return nil
		}
		return fmt.Errorf("%s: missing required value", path)
	}
	if !isRule {
		for _, v := range values {
			if v == expected {
				return nil
			}
		}
		return fmt.Errorf("%s: expected %q, got %q", path, expected, values[len(values)-1])
	}

	kind, inner, compound, err := cutCompound(rule)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if compound {
		innerStr, ok := inner.(string)
		if !ok {
			return fmt.Errorf("%s: cannot match structured value %s against a string", path, formatJSONVal(inner))
		}
		if kind == optionalRule {
			return MatchStringValues(path, innerStr, values)
		}
		for i, v := range values {
			if err := MatchStringValues(fmt.Sprintf("%s[%d]", path, i), innerStr, []string{v}); err != nil {
				return err
			}
		}
		return nil
	}

	var firstErr error
	for _, v := range values {
		err := matchIsRule(path, rule, coerceString(rule, v), false)
		if err == nil {
			return nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// coerceString converts a string to the JSON type a scalar rule expects so
// "4" satisfies is.int and "true" satisfies is.bool.
func coerceString(rule, v string) any {
	switch strings.ToLower(strings.TrimSpace(rule)) {
	case "number", "int", "integer":
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	case "bool", "boolean":
		switch v {
		case "true":
			return true
		case "false":
			return false
		}
	}
	return v
}

// BodyOptions controls how MatchExpectedBody compares bodies.
type BodyOptions struct {
	// Exact compares the raw strings instead of matching JSON structurally.
	Exact bool
	// Additional allows fields in the actual body (at any depth) that the
	// expected body doesn't list.
	Additional bool
}

func MatchExpectedBody(expectedBody, actualBody string, opts BodyOptions) error {
	// body = schemas.x: match against what x holds
	if _, inner, ok := CutNamed(expectedBody); ok && !opts.Exact {
		inner = unwrapNamed(inner)
		if s, isStr := inner.(string); isStr {
			return MatchExpectedBody(s, actualBody, opts)
		}
		b, err := json.Marshal(inner)
		if err != nil {
			return err
		}
		return MatchExpectedBody(string(b), actualBody, opts)
	}
	if opts.Exact {
		if expectedBody != actualBody {
			return fmt.Errorf("body does not match expectation %s != %s", expectedBody, actualBody)
		}
		return nil
	}
	if rule, ok := strings.CutPrefix(expectedBody, IsPrefix); ok {
		if _, _, compound, _ := cutCompound(rule); compound {
			if strings.TrimSpace(actualBody) == "" {
				if strings.HasPrefix(rule, optionalRule) {
					return nil
				}
				return errors.New("body: missing")
			}
			var act any = actualBody
			var decoded any
			if json.Unmarshal([]byte(actualBody), &decoded) == nil {
				act = decoded
			}
			return matchIsRule("body", rule, act, opts.Additional)
		}
		return matchIsRule("body", rule, actualBody, opts.Additional)
	}

	var expJSON, actJSON any
	expErr := json.Unmarshal([]byte(expectedBody), &expJSON)
	actErr := json.Unmarshal([]byte(actualBody), &actJSON)
	if expErr == nil && actErr == nil {
		return matchJSONValue("body", expJSON, actJSON, opts.Additional)
	}
	if expErr == nil {
		if strings.TrimSpace(actualBody) == "" {
			return errors.New("body: missing")
		}
		return fmt.Errorf("body: expected JSON, got %q", truncate(actualBody, 64))
	}

	if expectedBody != actualBody {
		return fmt.Errorf("body does not match expectation %s != %s", expectedBody, actualBody)
	}
	return nil
}

func matchJSONValue(path string, expected, actual any, additional bool) error {
	if expStr, ok := expected.(string); ok {
		if rule, isMatcher := strings.CutPrefix(expStr, IsPrefix); isMatcher {
			return matchIsRule(path, rule, actual, additional)
		}
	}

	switch exp := expected.(type) {
	case map[string]any:
		actMap, ok := actual.(map[string]any)
		if !ok {
			return fmt.Errorf("%s: expected object, got %s (%v)", path, jsonTypeName(actual), formatJSONVal(actual))
		}
		expKeys := make([]string, 0, len(exp))
		for k := range exp {
			expKeys = append(expKeys, k)
		}
		sort.Strings(expKeys)
		for _, k := range expKeys {
			actVal, exists := actMap[k]
			if !exists {
				if isOptional(exp[k]) {
					continue
				}
				return fmt.Errorf("%s.%s: missing required field", path, k)
			}
			if err := matchJSONValue(path+"."+k, exp[k], actVal, additional); err != nil {
				return err
			}
		}
		if additional {
			return nil
		}
		actKeys := make([]string, 0, len(actMap))
		for k := range actMap {
			if _, exists := exp[k]; !exists {
				actKeys = append(actKeys, k)
			}
		}
		if len(actKeys) > 0 {
			sort.Strings(actKeys)
			return fmt.Errorf("%s.%s: unexpected field", path, actKeys[0])
		}
		return nil

	case []any:
		actArr, ok := actual.([]any)
		if !ok {
			return fmt.Errorf("%s: expected array, got %s (%v)", path, jsonTypeName(actual), formatJSONVal(actual))
		}
		if len(exp) != len(actArr) {
			return fmt.Errorf("%s: expected array length %d, got %d", path, len(exp), len(actArr))
		}
		for i := range exp {
			if err := matchJSONValue(fmt.Sprintf("%s[%d]", path, i), exp[i], actArr[i], additional); err != nil {
				return err
			}
		}
		return nil

	default:
		if expected != actual {
			return fmt.Errorf("%s: expected %s, got %s", path, formatJSONVal(expected), formatJSONVal(actual))
		}
		return nil
	}
}

func matchIsRule(path, rule string, actual any, additional bool) error {
	if _, inner, ok := CutNamed(IsPrefix + rule); ok {
		return matchJSONValue(path, inner, actual, additional)
	}
	if kind, inner, ok, err := cutCompound(rule); ok {
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if kind == optionalRule {
			return matchJSONValue(path, inner, actual, additional)
		}
		arr, isArr := actual.([]any)
		if !isArr {
			return fmt.Errorf("%s: expected array, got %s (%s)", path, jsonTypeName(actual), formatJSONVal(actual))
		}
		for i, item := range arr {
			if err := matchJSONValue(fmt.Sprintf("%s[%d]", path, i), inner, item, additional); err != nil {
				return err
			}
		}
		return nil
	}
	if pat, ok := cutRegexRule(rule); ok {
		s, ok := actual.(string)
		if !ok {
			return fmt.Errorf("%s: expected string matching /%s/, got %s (%s)", path, pat, jsonTypeName(actual), formatJSONVal(actual))
		}
		matched, err := regexp.MatchString(pat, s)
		if err != nil || !matched {
			return fmt.Errorf("%s: value %q does not match regex /%s/", path, s, pat)
		}
		return nil
	}

	switch strings.ToLower(strings.TrimSpace(rule)) {
	case "any":
		return nil
	case "null", "nil":
		if actual != nil {
			return fmt.Errorf("%s: expected null, got %s", path, formatJSONVal(actual))
		}
		return nil
	case "string":
		if _, ok := actual.(string); !ok {
			return fmt.Errorf("%s: expected string, got %s (%s)", path, jsonTypeName(actual), formatJSONVal(actual))
		}
		return nil
	case "number":
		if _, ok := actual.(float64); !ok {
			return fmt.Errorf("%s: expected number, got %s (%s)", path, jsonTypeName(actual), formatJSONVal(actual))
		}
		return nil
	case "int", "integer":
		f, ok := actual.(float64)
		if !ok || math.Trunc(f) != f {
			return fmt.Errorf("%s: expected integer, got %s (%s)", path, jsonTypeName(actual), formatJSONVal(actual))
		}
		return nil
	case "bool", "boolean":
		if _, ok := actual.(bool); !ok {
			return fmt.Errorf("%s: expected bool, got %s (%s)", path, jsonTypeName(actual), formatJSONVal(actual))
		}
		return nil
	case "array", "list":
		if _, ok := actual.([]any); !ok {
			return fmt.Errorf("%s: expected array, got %s (%s)", path, jsonTypeName(actual), formatJSONVal(actual))
		}
		return nil
	case "object", "map":
		if _, ok := actual.(map[string]any); !ok {
			return fmt.Errorf("%s: expected object, got %s (%s)", path, jsonTypeName(actual), formatJSONVal(actual))
		}
		return nil
	case "not_empty", "nonempty":
		switch v := actual.(type) {
		case string:
			if v != "" {
				return nil
			}
		case []any:
			if len(v) > 0 {
				return nil
			}
		case map[string]any:
			if len(v) > 0 {
				return nil
			}
		}
		return fmt.Errorf("%s: expected non-empty value, got %s", path, formatJSONVal(actual))
	case "uuid":
		s, ok := actual.(string)
		if !ok || uuid.Validate(s) != nil {
			return fmt.Errorf("%s: expected uuid, got %s", path, formatJSONVal(actual))
		}
		return nil
	case "uuidv4":
		s, ok := actual.(string)
		if !ok {
			return fmt.Errorf("%s: expected uuidv4, got %s", path, formatJSONVal(actual))
		}
		u, err := uuid.Parse(s)
		if err != nil || u.Version() != 4 {
			return fmt.Errorf("%s: expected uuidv4, got %s", path, formatJSONVal(actual))
		}
		return nil
	case "ulid":
		s, ok := actual.(string)
		if !ok || !restlua.IsULID(s) {
			return fmt.Errorf("%s: expected ulid, got %s", path, formatJSONVal(actual))
		}
		return nil
	case "nanoid":
		s, ok := actual.(string)
		if !ok || !nanoidRegex.MatchString(s) {
			return fmt.Errorf("%s: expected nanoid, got %s", path, formatJSONVal(actual))
		}
		return nil
	case "jwt":
		s, ok := actual.(string)
		if !ok || !restlua.IsJWT(s) {
			return fmt.Errorf("%s: expected jwt, got %s", path, formatJSONVal(actual))
		}
		return nil
	case "email":
		s, ok := actual.(string)
		if !ok {
			return fmt.Errorf("%s: expected email, got %s", path, formatJSONVal(actual))
		}
		if _, err := mail.ParseAddress(s); err != nil {
			return fmt.Errorf("%s: expected email, got %s", path, formatJSONVal(actual))
		}
		return nil
	case "url", "uri":
		s, ok := actual.(string)
		if !ok {
			return fmt.Errorf("%s: expected url, got %s", path, formatJSONVal(actual))
		}
		u, err := url.Parse(s)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return fmt.Errorf("%s: expected url, got %s", path, formatJSONVal(actual))
		}
		return nil
	default:
		return fmt.Errorf("%s: unknown expectation matcher %q", path, rule)
	}
}

func jsonTypeName(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case string:
		return "string"
	case float64, int:
		return "number"
	case bool:
		return "bool"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	default:
		return fmt.Sprintf("%T", v)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// describeRule renders a matcher rule the way it is written in a .rest file.
func describeRule(rule string) string {
	if name, _, ok := CutNamed(IsPrefix + rule); ok {
		return "schemas." + name
	}
	if kind, inner, ok, err := cutCompound(rule); ok && err == nil {
		return "is::" + strings.TrimSuffix(kind, ":") + "(" + formatJSONVal(inner) + ")"
	}
	if pat, ok := cutRegexRule(rule); ok {
		return fmt.Sprintf("is(%q)", "regex:"+pat)
	}
	return "is." + rule
}

func formatJSONVal(v any) string {
	if s, ok := v.(string); ok {
		if rule, ok := strings.CutPrefix(s, IsPrefix); ok {
			return describeRule(rule)
		}
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}
