# Pages split (temp design notes)

Goal: use rest's `page`s (interactive midnight markup) from other Go apps, so any
router (std mux, gin, fiber, ...) can mount an `.mn` page as an
`http.Handler`. Not final; moves to `doc/` once the design is settled.

## Status of the decisions

| Topic | Decision |
|---|---|
| Handler type | `http.Handler` (gin `WrapH`, fiber `adaptor.HTTPHandler`) |
| Route | not passed in: actions POST to the current document URL |
| Package | `github.com/taybart/rest/pages`, same module |
| Sources | `File(path)` and `Source(string)` now; `fs.FS` / embed **later** |
| Go functions | reflection, adapted once at registration, plus a raw `LGFunction` escape hatch (see below) |

## Package layout

`pages` imports only `rest/lua` and midnight (so no hcl, cty, cors, websocket,
`rest/request`). The module stays the same, and Go only links the packages
that are imported.

Moves out of `server`:

- `state.go` (state store, `X-Rest-State`, 410 on a gone state)
- `page.go` (mount / dispatch / `isInteractive`)
- the page parts of `cache.go` and `gzip_handler.go` (as handler options)

`server` then builds its page handlers with `pages`, adding `rest.request`,
`kv`, middleware, etc. on top.

## API sketch

```go
eng := pages.New(pages.Options{
    MaxStates: 1000,             // shared by every page of the engine
    StateTTL:  30 * time.Minute,
    Path:      "./ui/?.mn",    // package.path for require
})
defer eng.Close()

h := eng.File("ui/app.mn",
    pages.Props(func(r *http.Request) (any, error) {
        return map[string]any{"name": r.URL.Query().Get("name")}, nil
    }),
    pages.Funcs("app.db", map[string]any{
        "add_item":   db.AddItem,
        "list_items": db.ListItems,
    }),
)

mux.Handle("/app", h)                     // std: GET renders, POST runs actions
r.Any("/app", gin.WrapH(h))               // gin
app.All("/app", adaptor.HTTPHandler(h))   // fiber
```

- The engine owns the state store, so `MaxStates` and the TTL cleanup
  goroutine apply to all of its pages.
- The entry (file or source) returns a component, `(props) -> <html>…</html>`,
  (like `test/pages/app.mn`) or an element. The handler calls it with what
  `Props(r)` returned. It can also return a response table (ex. a redirect).
- GET renders and POST dispatches an action, so the handler has to be mounted
  for both methods, and the app can't have its own POST on that path.
- The `rest` global stays (`rest.req`, `rest.state`, `rest.exports`), so the
  same `.mn` files work under `rest` and in other apps. `rest.request` only
  exists when a Requester is set (the `server` case).
- The app's router middleware wraps the handler, so it runs for actions too
  (ex. auth).
- `File` keeps today's behavior: required files are re-checked (stat cache)
  and changes show up on the next page load.

### Action URL

Today the rpc url is `req.URL.Path` (`server/lua.go`). Behind
`http.StripPrefix` or a gin group, that's the stripped path, not the one the
browser sees. Change: an empty `rpc` in `markup.mount` means "POST to the
current document" (`location.pathname + location.search`) on the client,
and `pages.ActionURL(string)` overrides it.

## Go functions

Actions need to call back into the app (DBs, services, ...), and props only
cover the first render.

### Request context

Actions run in later requests than the page load, in the page's kept state.
So functions must see the **current** request (the action's), not the one
the page was loaded with. The engine stores the current `*http.Request` on
the state before each run, and a function whose first param is
`context.Context` or `*http.Request` gets it (not a midnight argument).

```go
func (db *DB) AddItem(ctx context.Context, item string) error
```

```midnight
local db = require('app.db')
local AddItem = () -> <form onsubmit={e -> { db.add_item(e.form.item) }}>…</form>
```

### Reflection, adapted once

The concerns with reflection are speed and error messages. To address both,
all the reflection about a function's *signature* happens in `Funcs(...)`,
when the handler is built:

1. `reflect.TypeOf(fn)`: check it's a func, and that each param/return type
   is supported. Otherwise **panic at registration** w/ the module, name, and
   param (ex. `pages: app.db.add_item: param 2: chan int is not supported`),
   so a bad signature fails at startup, not on a click.
2. Build one converter per param (`func(lua.LValue) (reflect.Value, error)`)
   and per result, plus the context injector.
3. Store a closure that's an `lua.LGFunction`: check arg count, run the
   converters, `fn.Call(args)`, convert the results back.

Per call, that's a `reflect.Value.Call` plus the converters, with no type
inspection. `Call` costs about 100-300ns more than a direct call. That's
small next to a lua state run and a render, which is the cost of an action
anyway. If it shows up in profiles, use the escape hatch or a generic adapter
(below).

Supported types (to start):

| Go | midnight |
|---|---|
| `string`, `bool`, ints, floats | string, boolean, number (non-integer numbers into ints are an error) |
| `[]T` | array table |
| `map[string]T` | table |
| structs | table by field name (`json` tag if set), on return only to start |
| `any` | `restlua.LValueToAny` / `AnyToLValue` |
| `error` (last result) | non-nil → lua error w/ the message (`pcall`able) |

Returns: `()`, `(T)`, `(error)`, `(T, error)`. More than one non-error
result is rejected at registration, to keep the midnight side simple.

Arg errors name the function and position: `app.db.add_item: arg 1: expected
string, got nil`.

### Escape hatch

Raw gopher-lua functions, for full control or hot paths. These expose midnight
types, which is the trade-off:

```go
pages.LuaFuncs("app.fast", map[string]lua.LGFunction{
    "sum": func(l *lua.LState) int { ... },
})
```

The current request is available from a helper: `pages.Request(l)`.

### Maybe: generic adapters

Typed wrappers that don't use `reflect.Value.Call`, for common shapes:

```go
pages.Func1(db.GetItem)   // func(context.Context, A) (R, error)
pages.Func2(db.Move)      // func(context.Context, A, B) (R, error)
```

Converters are still chosen by type once (at wrap time), but the call is
direct. Usable as values in the same `Funcs` map (the map checks for an
already-adapted function first). Only worth adding if reflection shows up in
profiles.

## Later

- **`fs.FS` / embed**: a loader in `package.loaders` that resolves `require`
  against an `fs.FS` (probably in midnight), with compiled chunks cached for
  good (embedded files don't change). Or precompile the `.mn` files to
  bytecode at build time and embed that. Until then, files live on disk like
  they do today.
- Cache / gzip as handler options (`pages.Cache(ttl, vary...)`).
- Whether `kv` / `setup` have an equivalent outside `server`, or apps just use
  Go functions.

## Steps

1. ~~Create `pages`: move the state store and mount/dispatch, `Engine`,
   `File`, `Source`, `Props`. `server` uses it, and its tests pass unchanged.~~
   Done. `pages` deps: midnight, taybart/log, uuid, nanoid (via `rest/lua`).
   Not yet: cache/gzip options.
   - Exports: `Options.Requester` (enables `rest.request`, ex.
     `rest.NewFile("api.rest")` → `.Requester(client)`; only apps that use it
     import the root package), `Options.Exports` (shared start), and
     `pages.Exports(func(r) ...)` (per page load, merged on top). Kept states
     keep their whole `rest` table (`restlua.SetRestGlobal` only replaces
     req / path / ctx), so exports survive between actions, in `server` too.
2. ~~Empty `rpc` → client posts to the current document (midnight `markup.lua`).~~
   Done (midnight change, used via an untracked `go.work` until midnight is
   bumped). Engine pages default to it, `pages.ActionURL` overrides; `server`
   pages use it too (actions now also see the page's query). Also fixed in
   the client: idiomorph's `ignoreActiveValue` skipped a focused element's
   children, so a clicked button's own text never updated; it's now only on
   while an input / textarea / select / contenteditable has focus.
3. ~~`LuaFuncs` + the current-request plumbing.~~ Done.
   `pages.LuaFuncs(module, map[string]lua.LGFunction)` preloads a module per
   page state; `pages.Request(l)` is the request being handled (the action's
   for actions). It's kept in the lua registry (so coroutines see it too) and
   cleared after each request. Not via `l.SetContext`: that switches the VM
   to a loop that checks the context on every instruction (slower renders).
4. ~~Reflection `Funcs`, with tests for registration errors and conversions,
   and a benchmark vs `LuaFuncs`.~~ Done (`pages/funcs.go`, `pages/convert.go`).
   - Differences from the design above: structs work as params too (not only
     results), and so do `time.Time` (RFC 3339 strings), `[]byte` (strings),
     and recursive types. `Props` uses the same converters (it used
     `restlua.AnyToLValue`, which turned structs into `fmt.Sprint` strings).
   - Benchmarks (M-series, per call, incl. the lua loop):
     `LuaFuncs` add 34ns / 1 alloc, `Funcs` add 156ns / 6 allocs;
     `(ctx, string) (string, error)` 53ns vs 302ns; a struct result 340ns.
     A whole action on `testdata/app.mn` is ~42µs (`BenchmarkAction`), so
     a `Funcs` call is well under 1% of one. Generic adapters (below) aren't
     worth it for now.
5. ~~Example app with std mux and gin (`test/pages/embed/`?).~~ Done, w/
   fiber v3 instead of gin: `examples/pages`, its own module (fiber stays out
   of rest's go.mod; a
   `replace` builds it against this checkout). A todo list w/ `pages.Funcs`
   (struct results, Go errors shown via pcall), mounted under a prefix on
   both routers; `todo/todotest` runs the same flow against each. Neither
   binary links hcl / cty / cors / websocket.
   - Found: submitting a form w/ Enter keeps the input's text (the focused
     input is "being typed in", so its value is preserved). A fix would be in
     midnight's client: don't preserve the active value for submit events.
