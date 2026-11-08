package server

import (
	"net/http"
	"testing"

	"github.com/taybart/rest/pages"
)

// A page's state is only kept if it has actions.
func TestPageStateKeptOnlyIfInteractive(t *testing.T) {
	s := &Server{Router: http.NewServeMux(), Config: Config{Quiet: true}}
	s.states = pages.NewStateStore(0, 0)
	for _, c := range []struct {
		page string
		kept int
	}{
		{`return <p>static</p>`, 0},
		{`return { status = 302, headers = { Location = '/' } }`, 0},
		{`error('boom')`, 0},
		{`return <button onclick={() -> {}}>live</button>`, 1},
	} {
		before := s.states.Len()
		s.RunLuaPipeline(&Handler{Method: "GET", Path: "/", Page: c.page, Lang: "midnight"}, pageRequest(), nil)
		if kept := s.states.Len() - before; kept != c.kept {
			t.Errorf("%s: kept %d states, want %d", c.page, kept, c.kept)
		}
	}
}

func pageRequest() *http.Request {
	req, _ := http.NewRequest("GET", "/", nil)
	return req
}
