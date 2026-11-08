package main

import (
	"net/http/httptest"
	"testing"

	"github.com/taybart/rest/doc/examples/pages/todo"
	"github.com/taybart/rest/doc/examples/pages/todo/todotest"
	"github.com/taybart/rest/pages"
)

func TestTodos(t *testing.T) {
	eng := pages.New(pages.Options{})
	defer eng.Close()
	// the embedded files
	ts := httptest.NewServer(routes(eng, todo.NewStore(), ""))
	defer ts.Close()
	todotest.Run(t, ts.URL+"/todos/", "net/http")
}
