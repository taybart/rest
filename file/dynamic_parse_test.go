package file_test

import (
	"strings"
	"testing"
)

// dynamic blocks (hcl's ext/dynblock, like terraform's) make a handler per
// element of a collection, w/ labels from expressions
func TestParseDynamicHandlers(t *testing.T) {
	serv, err := parseServer(t, `
locals {
  pages = [
    { path = "/", file = "index.md" },
    { path = "/docs", file = "docs.md" },
    { path = "/play", file = "" },
  ]
}
server {
  address = ":0"

  dynamic "handler" {
    for_each = [for p in locals.pages : p if p.file != ""]
    labels   = ["GET", handler.value.path == "/" ? "/{$}" : handler.value.path]
    content {
      fn = <<-MIDNIGHT
        return { status = 200, body = '${handler.value.file}' }
      MIDNIGHT
    }
  }

  handler "GET" "/play" {
    fn = <<LUA
      return { status = 200, body = "play" }
    LUA
  }
}
`)
	if err != nil {
		t.Fatal(err)
	}
	type got struct{ path, lang, file string }
	var handlers []got
	for _, h := range serv.Handlers {
		file := ""
		if strings.Contains(h.Fn, "index.md") {
			file = "index.md"
		} else if strings.Contains(h.Fn, "docs.md") {
			file = "docs.md"
		}
		handlers = append(handlers, got{h.Path, h.Lang, file})
	}
	want := []got{
		{"/{$}", "midnight", "index.md"},
		{"/docs", "midnight", "docs.md"},
		{"/play", "lua", ""}, // (after the dynamic block: still lua)
	}
	if len(handlers) != len(want) {
		t.Fatalf("got %d handlers %+v, want %+v", len(handlers), handlers, want)
	}
	for i := range want {
		if handlers[i] != want[i] {
			t.Errorf("handler %d: got %+v, want %+v", i, handlers[i], want[i])
		}
	}
}
