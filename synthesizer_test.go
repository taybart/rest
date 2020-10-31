package rest

import (
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/matryer/is"
)

func TestSynthesizeRequests(t *testing.T) {
	is := is.New(t)

	// Read example request
	r := New()
	err := r.Read("./test/post.rest")
	is.NoErr(err)

	table := []struct {
		lang string
		ft   string
	}{{"javascript", "js"}, {"go", "go"}, {"curl", "curl"}}

	for _, tt := range table {
		tt := tt
		t.Run(tt.lang, func(t *testing.T) {
			// Gen requests
			requests, err := r.SynthesizeRequests(tt.lang)
			is.NoErr(err)

			// Get answer
			ans, err := ioutil.ReadFile(fmt.Sprintf("./test/template_request.%s", tt.ft))
			is.NoErr(err)

			// Check answer
			for i, c := range requests[0] {
				is.Equal(rune(ans[i]), c)
			}
		})
	}
}

func TestSynthesizeClient(t *testing.T) {
	is := is.New(t)

	// Get all requests
	r := New()
	err := r.Read("./test/client.rest")
	is.NoErr(err)

	table := []struct {
		lang string
		ft   string
	}{{"javascript", "js"}, {"go", "go"}, {"curl", "curl"}}

	for _, tt := range table {
		tt := tt
		t.Run(tt.lang, func(t *testing.T) {
			_, err := r.SynthesizeClient(tt.lang)
			is.NoErr(err)
		})
	}
}
