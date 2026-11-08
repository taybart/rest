# pages example: todos

A todo list served by `github.com/taybart/rest/pages` on two routers: net/http's
`ServeMux` (`cmd/std`) and fiber v3 (`cmd/fiber`). It's its own module, so fiber
isn't a dependency of rest; it builds against this checkout (`replace` in
go.mod).

```sh
cd doc/examples/pages
go run ./cmd/std   # http://localhost:8090/todos/
go run ./cmd/fiber # http://localhost:8091/todos/
go test ./...
```

- `todo/ui/`: the page, a component per file, each w/ its own scoped styles
  (midnight's `css`): `app.mn` (the `<html>` shell, layout, theme),
  `header.mn`, `list.mn` (form, error, counts), and `item.mn` (one
  todo). `require('list')` etc. find them: file pages look in their own
  directory. Their actions (`onsubmit`, `onchange`, `onclick`) run on the
  server, and only what changed is sent back.
- `todo/ui/tb.css`: [tb.css](https://github.com/taybart/tb.css) (vendored),
  the base styles (dark / light theme, buttons, inputs, lists). The page links
  it relatively, and each router serves it next to the page (`todo.Styles`
  on the std mux, `c.SendFile` on fiber). The app's own styles are scoped
  (midnight's `css`), and the theme toggle is page state (`data-theme` on body).
- `todo/page.go`: the page handler, w/ the store's methods as the
  `app.todos` module (`pages.Funcs`): plain Go methods, w/ the request's
  context, converted args / results, and errors as lua errors.
- `cmd/std`: mounted under `/todos/` w/ `http.StripPrefix`; `cmd/fiber`: in a
  `/todos` group. The handler is mounted for GET (render) and POST (actions):
  actions POST to the url the browser sees, so prefixes just work.
- fiber runs on fasthttp, not net/http, so the page (an `http.Handler`) goes
  through `adaptor.HTTPHandler`, which converts every request and response.
  Its app isn't an `http.Handler` either: its test serves it on a listener.
- `todo/todotest`: drives the page like its client script does; both routers
  are tested w/ it.

The page's files (`todo/ui`) are embedded in the binary and served w/
`eng.FS`: compiled once, nothing read from disk. To edit them while it runs,
point `-ui` at them (`go run ./cmd/std -ui todo/ui`): that serves them w/
`eng.File`, and changes show up on the next page load.
