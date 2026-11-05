package request_test

import (
	"encoding/json"
	"testing"

	"github.com/taybart/rest/request"
)

func schemaJSON(t *testing.T, expected any, opts request.SchemaOptions) string {
	t.Helper()
	b, err := json.Marshal(request.MatcherSchema(expected, opts))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestMatcherSchema(t *testing.T) {
	inner := func(rule string) string { b, _ := json.Marshal(request.IsToken(rule)); return string(b) }
	cases := []struct {
		name     string
		expected any
		opts     request.SchemaOptions
		want     string
	}{
		{"literal", "user", request.SchemaOptions{}, `{"const":"user"}`},
		{"literal number", 5.0, request.SchemaOptions{}, `{"const":5}`},
		{"null", nil, request.SchemaOptions{}, `{"type":"null"}`},
		{"int", request.IsToken("int"), request.SchemaOptions{}, `{"type":"integer"}`},
		{"uuid", request.IsToken("uuid"), request.SchemaOptions{}, `{"format":"uuid","type":"string"}`},
		{"regex", request.IsToken("regex:^a+$"), request.SchemaOptions{}, `{"pattern":"^a+$","type":"string"}`},
		{"any", request.IsToken("any"), request.SchemaOptions{}, `{}`},
		{"not_empty param", request.IsToken("not_empty"), request.SchemaOptions{Param: true}, `{"minLength":1,"type":"string"}`},
		{"optional unwraps", request.OptionalToken(inner("bool")), request.SchemaOptions{}, `{"type":"boolean"}`},
		{"array_of", request.ArrayOfToken(inner("string")), request.SchemaOptions{}, `{"items":{"type":"string"},"type":"array"}`},
		{"tuple", []any{request.IsToken("string"), 1.0}, request.SchemaOptions{},
			`{"maxItems":2,"minItems":2,"prefixItems":[{"type":"string"},{"const":1}],"type":"array"}`},
		{"object", map[string]any{"a": request.IsToken("int"), "b": request.OptionalToken(inner("string"))}, request.SchemaOptions{},
			`{"additionalProperties":false,"properties":{"a":{"type":"integer"},"b":{"type":"string"}},"required":["a"],"type":"object"}`},
		{"object additional", map[string]any{"a": 1.0}, request.SchemaOptions{Additional: true},
			`{"properties":{"a":{"const":1}},"required":["a"],"type":"object"}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := schemaJSON(t, c.expected, c.opts); got != c.want {
				t.Errorf("got  %s\nwant %s", got, c.want)
			}
		})
	}
}
