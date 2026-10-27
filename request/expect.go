package request

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/mail"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/google/uuid"
	restlua "github.com/taybart/rest/lua"
)

const IsPrefix = "\x00__rest_is__:"

var nanoidRegex = regexp.MustCompile(`^[A-Za-z0-9_-]{21}$`)

func IsToken(rule string) string {
	return IsPrefix + rule
}

func ValidateIsRule(rule string) error {
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
			if err := MatchExpectedBody(expect.Body, body, expect.Exact); err != nil {
				return fmt.Errorf(`request(%s): %w`, label, err)
			}
		}
		if len(expect.Headers) != 0 {
			for k, expVal := range expect.Headers {
				values := headers.Values(k)
				if len(values) == 0 {
					return fmt.Errorf(`request(%s): required response header %q not present`, label, k)
				}
				matches := false
				lastValue := ""
				for _, value := range values {
					lastValue = value
					if matchHeaderValue(expVal, value) {
						matches = true
						break
					}
				}
				if !matches {
					return fmt.Errorf(`request(%s): unexpected response header [%s] %s != %s`, label, k, formatExpectedVal(expVal), lastValue)
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

func matchHeaderValue(expected, actual string) bool {
	if rule, ok := strings.CutPrefix(expected, IsPrefix); ok {
		return matchIsRule("header", rule, actual) == nil
	}
	return expected == actual
}

func MatchExpectedBody(expectedBody, actualBody string, exact bool) error {
	if exact {
		if expectedBody != actualBody {
			return fmt.Errorf("body does not match expectation %s != %s", expectedBody, actualBody)
		}
		return nil
	}
	if rule, ok := strings.CutPrefix(expectedBody, IsPrefix); ok {
		return matchIsRule("body", rule, actualBody)
	}

	var expJSON, actJSON any
	expErr := json.Unmarshal([]byte(expectedBody), &expJSON)
	actErr := json.Unmarshal([]byte(actualBody), &actJSON)
	if expErr == nil && actErr == nil {
		return matchJSONValue("body", expJSON, actJSON)
	}

	if expectedBody != actualBody {
		return fmt.Errorf("body does not match expectation %s != %s", expectedBody, actualBody)
	}
	return nil
}

func matchJSONValue(path string, expected, actual any) error {
	if expStr, ok := expected.(string); ok {
		if rule, isMatcher := strings.CutPrefix(expStr, IsPrefix); isMatcher {
			return matchIsRule(path, rule, actual)
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
				return fmt.Errorf("%s.%s: missing required field", path, k)
			}
			if err := matchJSONValue(path+"."+k, exp[k], actVal); err != nil {
				return err
			}
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
			if err := matchJSONValue(fmt.Sprintf("%s[%d]", path, i), exp[i], actArr[i]); err != nil {
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

func matchIsRule(path, rule string, actual any) error {
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

func formatExpectedVal(s string) string {
	if rule, ok := strings.CutPrefix(s, IsPrefix); ok {
		return "is." + rule
	}
	return s
}

func formatJSONVal(v any) string {
	if s, ok := v.(string); ok {
		if rule, ok := strings.CutPrefix(s, IsPrefix); ok {
			return "is." + rule
		}
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}
