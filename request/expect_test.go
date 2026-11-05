package request_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/taybart/rest/request"
)

func tok(rule string) string { return request.IsToken(rule) }

func optional(inner any) string {
	b, _ := json.Marshal(inner)
	return request.OptionalToken(string(b))
}

func arrayOf(inner any) string {
	b, _ := json.Marshal(inner)
	return request.ArrayOfToken(string(b))
}

func body(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestMatchOptionalAndArrayOf(t *testing.T) {
	expected := body(t, map[string]any{
		"name":     tok("string"),
		"remember": optional(tok("bool")),
		"tags":     arrayOf(tok("string")),
		"items":    optional(arrayOf(map[string]any{"id": tok("int"), "note": optional(tok("string"))})),
	})

	cases := []struct {
		name   string
		actual string
		errSub string // empty means expect pass
	}{
		{"all present", `{"name":"a","remember":true,"tags":["x"],"items":[{"id":1,"note":"n"},{"id":2}]}`, ""},
		{"optional absent", `{"name":"a","tags":[]}`, ""},
		{"optional wrong type", `{"name":"a","tags":[],"remember":"yes"}`, "body.remember: expected bool"},
		{"optional null is not absent", `{"name":"a","tags":[],"remember":null}`, "body.remember: expected bool"},
		{"required missing", `{"tags":[]}`, "body.name: missing required field"},
		{"array_of bad element", `{"name":"a","tags":["x",2]}`, "body.tags[1]: expected string"},
		{"array_of not array", `{"name":"a","tags":"x"}`, "body.tags: expected array"},
		{"nested array object", `{"name":"a","tags":[],"items":[{"id":1.5}]}`, "body.items[0].id: expected integer"},
		{"unexpected field still strict", `{"name":"a","tags":[],"extra":1}`, "body.extra: unexpected field"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := request.MatchExpectedBody(expected, c.actual, request.BodyOptions{})
			if c.errSub == "" {
				if err != nil {
					t.Fatalf("expected pass, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.errSub) {
				t.Fatalf("expected error containing %q, got %v", c.errSub, err)
			}
		})
	}
}

func TestMatchTopLevelCompoundBody(t *testing.T) {
	opt := optional(map[string]any{"q": tok("string")})
	if err := request.MatchExpectedBody(opt, "", request.BodyOptions{}); err != nil {
		t.Fatalf("optional body should allow empty: %v", err)
	}
	if err := request.MatchExpectedBody(opt, `{"q":"x"}`, request.BodyOptions{}); err != nil {
		t.Fatalf("optional body present: %v", err)
	}
	arr := arrayOf(tok("int"))
	if err := request.MatchExpectedBody(arr, `[1,2,3]`, request.BodyOptions{}); err != nil {
		t.Fatalf("top-level array_of: %v", err)
	}
	if err := request.MatchExpectedBody(arr, "", request.BodyOptions{}); err == nil || !strings.Contains(err.Error(), "body: missing") {
		t.Fatalf("expected missing body error, got %v", err)
	}
}

func TestMatchBodyNotJSON(t *testing.T) {
	expected := body(t, map[string]any{"a": tok("string")})
	err := request.MatchExpectedBody(expected, "a=1&b=2", request.BodyOptions{})
	if err == nil || !strings.Contains(err.Error(), "body: expected JSON") {
		t.Fatalf("expected JSON error, got %v", err)
	}
	err = request.MatchExpectedBody(expected, "", request.BodyOptions{})
	if err == nil || !strings.Contains(err.Error(), "body: missing") {
		t.Fatalf("expected missing error, got %v", err)
	}
}

func TestMatchStringValues(t *testing.T) {
	cases := []struct {
		name     string
		expected string
		values   []string
		errSub   string
	}{
		{"int coerced", tok("int"), []string{"4"}, ""},
		{"int rejects text", tok("int"), []string{"abc"}, "expected integer"},
		{"int rejects float", tok("int"), []string{"1.5"}, "expected integer"},
		{"number", tok("number"), []string{"1.5"}, ""},
		{"bool", tok("bool"), []string{"false"}, ""},
		{"bool rejects 1", tok("bool"), []string{"1"}, "expected bool"},
		{"uuid", tok("uuid"), []string{"c0a80121-7ac0-4e1c-9b8a-2f6d6e0e2f6a"}, ""},
		{"literal", "json", []string{"json"}, ""},
		{"literal mismatch", "json", []string{"xml"}, `expected "json", got "xml"`},
		{"missing", tok("string"), nil, "missing required value"},
		{"optional missing", optional(tok("int")), nil, ""},
		{"optional present bad", optional(tok("int")), []string{"x"}, "expected integer"},
		{"array_of all", arrayOf(tok("int")), []string{"1", "2"}, ""},
		{"array_of one bad", arrayOf(tok("int")), []string{"1", "x"}, "q[1]: expected integer"},
		{"regex", tok("regex:^[a-z]+$"), []string{"abc"}, ""},
		{"structured inner", optional(map[string]any{"a": 1}), []string{"x"}, "cannot match structured value"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := request.MatchStringValues("q", c.expected, c.values)
			if c.errSub == "" {
				if err != nil {
					t.Fatalf("expected pass, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.errSub) {
				t.Fatalf("expected error containing %q, got %v", c.errSub, err)
			}
		})
	}
}

func TestIsArrayOf(t *testing.T) {
	if !request.IsArrayOf(arrayOf(tok("int"))) || !request.IsArrayOf(optional(arrayOf(tok("int")))) {
		t.Fatal("expected array_of to be detected")
	}
	if request.IsArrayOf(tok("array")) || request.IsArrayOf(optional(tok("int"))) || request.IsArrayOf("x") {
		t.Fatal("false positive")
	}
}

func TestCheckExpectationHeaders(t *testing.T) {
	exp := &request.Expect{Headers: map[string]string{
		"X-Id":       tok("uuid"),
		"X-Optional": optional(tok("int")),
	}}
	h := map[string][]string{"X-Id": {"c0a80121-7ac0-4e1c-9b8a-2f6d6e0e2f6a"}}
	if err := request.CheckExpectation("t", exp, 0, 200, h, ""); err != nil {
		t.Fatalf("expected pass, got %v", err)
	}
	h["X-Optional"] = []string{"nope"}
	if err := request.CheckExpectation("t", exp, 0, 200, h, ""); err == nil || !strings.Contains(err.Error(), "header X-Optional: expected integer") {
		t.Fatalf("got %v", err)
	}
	delete(h, "X-Id")
	if err := request.CheckExpectation("t", exp, 0, 200, h, ""); err == nil || !strings.Contains(err.Error(), "header X-Id: missing required value") {
		t.Fatalf("got %v", err)
	}
}

func TestMatchAdditional(t *testing.T) {
	expected := body(t, map[string]any{
		"user":  map[string]any{"id": tok("int")},
		"items": arrayOf(map[string]any{"id": tok("int")}),
	})
	actual := `{"user":{"id":1,"name":"x"},"items":[{"id":1,"extra":true}],"meta":{}}`

	if err := request.MatchExpectedBody(expected, actual, request.BodyOptions{}); err == nil || !strings.Contains(err.Error(), "unexpected field") {
		t.Fatalf("strict: expected unexpected field error, got %v", err)
	}
	if err := request.MatchExpectedBody(expected, actual, request.BodyOptions{Additional: true}); err != nil {
		t.Fatalf("additional: expected pass at every depth, got %v", err)
	}
	// additional still enforces listed fields
	if err := request.MatchExpectedBody(expected, `{"user":{"name":"x"},"items":[]}`, request.BodyOptions{Additional: true}); err == nil || !strings.Contains(err.Error(), "body.user.id: missing required field") {
		t.Fatalf("additional: expected missing field error, got %v", err)
	}
	if err := request.CheckExpectation("t", &request.Expect{Body: expected, Additional: true}, 0, 200, nil, actual); err != nil {
		t.Fatalf("client expect additional: %v", err)
	}
}
