package parser

import (
	"testing"

	"github.com/matryer/is"
)

func TestParseFiles(t *testing.T) {
	files := []struct {
		name string
		fn   string
		res  bool
	}{
		{name: "example", fn: "../example.rest", res: true},
		// {name: "get", fn: "../test/get.rest", res: true},
		// {name: "var", fn: "../test/var.rest", res: true},
		// {name: "post", fn: "../test/post.rest", res: true},
		// {name: "multi", fn: "../test/multi.rest", res: true},
		// {name: "delay", fn: "../test/delay.rest", res: true},
		// {name: "expect", fn: "../test/expect.rest", res: true},
		// {name: "skip", fn: "../test/skip.rest", res: true},
		// {name: "runtime", fn: "../test/runtime.rest", res: true},
		// {name: "invalid", fn: "../test/invalid.rest", res: false}, // TODO add individual failures
	}
	for _, tt := range files {
		t.Run(tt.name, func(t *testing.T) {
			is := is.New(t)
			p := New(tt.fn)
			err := p.Parse()
			is.NoErr(err)
		})
	}
}
