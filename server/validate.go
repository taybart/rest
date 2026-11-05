package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"sort"

	"github.com/taybart/rest/request"
)

// ValidateRequest checks r against the handler's expect block.
func ValidateRequest(e *Expect, r *http.Request) error {
	if e == nil {
		return nil
	}
	for _, k := range sortedKeys(e.Path) {
		var vals []string
		if v := r.PathValue(k); v != "" {
			vals = []string{v}
		}
		if err := request.MatchStringValues("path."+k, e.Path[k], vals); err != nil {
			return err
		}
	}
	query := r.URL.Query()
	for _, k := range sortedKeys(e.Query) {
		if err := request.MatchStringValues("query."+k, e.Query[k], firstUnlessArray(e.Query[k], query[k])); err != nil {
			return err
		}
	}
	for _, k := range sortedKeys(e.Headers) {
		if err := request.MatchStringValues("headers."+k, e.Headers[k], firstUnlessArray(e.Headers[k], r.Header.Values(k))); err != nil {
			return err
		}
	}
	if e.Body != "" {
		var body []byte
		if r.Body != nil {
			var err error
			body, err = io.ReadAll(r.Body)
			r.Body.Close()
			if err != nil {
				return err
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
		}
		if err := request.MatchExpectedBody(e.Body, string(body), request.BodyOptions{Additional: e.Additional}); err != nil {
			return err
		}
	}
	return nil
}

// firstUnlessArray mirrors what handlers read: rest.req:query()/header()
// return the first value, so scalar rules only check that one.
func firstUnlessArray(expected string, vals []string) []string {
	if len(vals) > 1 && !request.IsArrayOf(expected) {
		return vals[:1]
	}
	return vals
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func validationResponse(err error) Response {
	body, _ := json.Marshal(map[string]string{"error": err.Error()})
	return Response{
		Status:  http.StatusBadRequest,
		Headers: map[string]string{"Content-Type": "application/json"},
		Body:    body,
	}
}
