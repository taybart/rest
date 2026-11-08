package main

import (
	"net/http/httptest"
	"testing"

	"github.com/taybart/rest/examples/pages/todo"
	"github.com/taybart/rest/examples/pages/todo/todotest"
	"github.com/taybart/rest/pages"
)

func TestTodos(t *testing.T) {
	eng := pages.New(pages.Options{})
	defer eng.Close()
	ts := httptest.NewServer(routes(eng, todo.NewStore(), "../../todo/ui"))
	defer ts.Close()
	todotest.Run(t, ts.URL+"/todos/", "net/http")
}
