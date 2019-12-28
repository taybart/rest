package rest

import (
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/matryer/is"
)

// TODO Fix the equal test
func TestSynthesizeRequests(t *testing.T) {
	is := is.New(t)
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
			requests, err := r.SynthisizeRequests(tt.lang)
			is.NoErr(err)
			ans, err := ioutil.ReadFile(fmt.Sprintf("./test/template_request.%s", tt.ft))
			is.NoErr(err)
			for i, c := range requests[0] {
				is.Equal(rune(ans[i]), c)
			}
		})
	}
}

func TestSynthesizeClient(t *testing.T) {
	is := is.New(t)
	r := New()
	err := r.Read("./test/client.rest")
	is.NoErr(err)
	table := []struct {
		lang string
		ft   string
	}{
		{"javascript", "js"},
		// {"go", "go"},
		// {"curl", "curl"},
	}
	for _, tt := range table {
		tt := tt
		t.Run(tt.lang, func(t *testing.T) {
			_, err := r.SynthisizeClient(tt.lang)
			is.NoErr(err)
		})
	}
}
