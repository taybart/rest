package request_test

import (
	"testing"

	"github.com/taybart/rest/request"
)

func TestBuildKeepsInlineQuery(t *testing.T) {
	cases := []struct {
		query map[string]string
		want  string
	}{
		{nil, "http://x/p?a=1&b=2"},
		{map[string]string{"c": "3"}, "http://x/p?a=1&b=2&c=3"},
		{map[string]string{"a": "override"}, "http://x/p?a=override&b=2"},
	}
	for _, c := range cases {
		r := request.Request{Method: "GET", URL: "http://x/p?a=1&b=2", Query: c.query}
		built, err := r.Build()
		if err != nil {
			t.Fatal(err)
		}
		if got := built.URL.String(); got != c.want {
			t.Errorf("query %v: got %s want %s", c.query, got, c.want)
		}
	}
}
