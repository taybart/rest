# Pages

rest's server can also serve HTML apps written in [Midnight](https://github.com/taybart/midnight)
with midnight's markup (HTML-like elements, components, and scoped styles):
handlers that return HTML, interactive `page`s whose events run on the server,
and data fetched with the file's own `request` blocks.

The same pages can be served by any Go app, on any router, with the
[`pages`](#pages-in-go-apps-restpages) package.

See [SERVER.md](SERVER.md) for the server itself (handlers, middleware, `kv`,
`setup`, ...), and midnight's [markup docs](https://github.com/taybart/midnight/blob/main/docs/MARKUP.md)
for the syntax and runtime.

- [Returning HTML](#returning-html)
- [Pages (`page`)](#pages-page)
- [Fetching data (`rest.request`)](#fetching-data-restrequest)
- [Caching pages (`cache`)](#caching-pages-cache)
- [Stateful handlers](#stateful-handlers)
- [Pages in Go apps (`rest/pages`)](#pages-in-go-apps-restpages)
  - [Mounting](#mounting)
  - [Where pages come from](#where-pages-come-from)
  - [Props](#props)
  - [Go functions (`Funcs`)](#go-functions-funcs)
  - [Raw lua functions (`LuaFuncs`)](#raw-lua-functions-luafuncs)
  - [Exports and `rest.request`](#exports-and-restrequest)
  - [Caching (`Cache`)](#caching-cache)
  - [Engine options](#engine-options)

## Returning HTML

A handler (or middleware) can return a markup element instead of a response table:
it's rendered as an HTML document (`text/html`). Or use it as the body of a
response table, to also set the status, headers, or cookies.

```hcl
handler "GET" "/hello" {
  fn = <<-MIDNIGHT
    local name = rest.req:query('name') || 'stranger'
    return <html><body><h1>hello {name}</h1></body></html>
  MIDNIGHT
}
handler "GET" "/missing" {
  fn = <<-MIDNIGHT
    return { status = 404, body = <p>nothing here</p> }
  MIDNIGHT
}
```

See midnight's [markup docs](https://github.com/taybart/midnight/blob/main/docs/MARKUP.md)
for the syntax, components, `state`, and scoped styles (`css`).

## Pages (`page`)

A `page` is like `fn`, but returns a markup element that's served as an
interactive page: functions in `on*` attributes (`onclick`, `on:input`, ...)
run on the server when the event happens in the browser, and only the
components that changed are sent back (and morphed into the page, so inputs
keep focus).

```hcl
handler "GET" "/{$}" {
  page = <<-MIDNIGHT
    local App = require('app')   -- ./app.mn
    return <App name={rest.req:query('name')}/>
  MIDNIGHT
}
```

```midnight
-- app.mn
local { state, css } = require('midnight.markup')

local s = css({ count = { font_weight = 'bold' } })

local Counter = () -> {
  local st = state({ count = 0 })
  return <button onclick={() -> { st.count += 1 }}>
    clicked <span class={s.count}>{st.count}</span> times
  </button>
}

return (props) -> <html>
  <body>
    <h1>hello {props.name || 'stranger'}</h1>
    <Counter/>
    <form onsubmit={e -> { print(e.form.item) }}>
      <input name="item"/>
    </form>
  </body>
</html>
```

- Actions are POSTs to the page's own path (rest adds the route), so a page
  can't have its own `POST` handler on the same path. They run the page's
  middleware too.
- An action gets a summary of the event: `{ type, value, checked, key, form }`.
- Each page load gets its own lua state, kept for its actions (see [Stateful handlers](#stateful-handlers)). Module level variables are per page, `state` is per component.
- Pages w/o actions are static: no client script, and no state is kept.
- A page can also return a response table (ex. a redirect).
- Static files (css, images, ...) can be served next to pages with the
  server's `directory`: it serves whatever no handler matches (so use
  `"/{$}"` for a page at the root, `"/"` matches everything).
- Changes to required `.mn` files show up on the next page load (the
  `.rest` file itself needs a restart).

## Fetching data (`rest.request`)

Pages get data by running the file's `request` blocks (including imported
ones) with `rest.request(label [, exports])` (or a block index), so the requests
an app makes are defined, run, tested, and exported like any others (see
[CLIENT.md](CLIENT.md#running-requests-from-lua-restrequest) for what it does
and returns). A block's `after` hook can post process its response: what it
returns is what `rest.request` returns.

```hcl
# hn.rest: runs on its own too (rest -f hn.rest -l item)
request "item" {
  url    = "https://api.hackerwebapp.com/item/${exports::try("id", 8863)}"
  expect = 200
  after  = [lib.expected, <<-MIDNIGHT
    local item = rest.res:json()
    return { title = item.title, url = item.url || "/post/{item.id}" }
  MIDNIGHT
  ]
}
```

```hcl
# server.rest
imports = ["./hn.rest"]
config {
  skip_imported     = true
  namespace_imports = true
}

server {
  handler "GET" "/post/{id}" {
    page = <<-MIDNIGHT
      local Post = require('post')
      local item = assert(rest.request('hn::item', { id = rest.req:path_value('id') }))
      return <Post item={item}/>
    MIDNIGHT
  }
}
```

- **Where to fetch**: a component runs on every render (after every action),
  so fetch in the page script (once per page load) or in actions (to refresh),
  and pass what was fetched to components as props. If a component has to
  fetch, keep it in its `state`: `if !s.item { s.item = rest.request(...) }`.
- **Exports**: `rest.exports` is what request blocks see as `exports`. Each
  request has its own (so requests running at the same time don't see each
  other's), starting from what `setup` exported (ex. a token it got at
  startup, see [SERVER.md](SERVER.md#seeding-state-kv-and-setup)). A page's
  (or `stateful` handler's) exports are kept in its state, between actions.
- **Cookies**: the server doesn't keep cookies from responses (they'd be
  shared between its users).
- **Caching**: `cache = "5m"` on a request block keeps its responses (by
  url, headers, etc.) for every page that requests them, so a popular page
  doesn't make a request per view (see
  [CLIENT.md](CLIENT.md#running-requests-from-lua-restrequest)). A cached
  response's `:json()` is decoded once and shared (read-only) by every page,
  instead of each page keeping its own copy.

## Caching pages (`cache`)

A page (or `fn`) that's the same for every visitor can be rendered once and
served from memory, with `cache` on its handler:

```hcl
handler "GET" "/hn/post/{id}" {
  page  = read("src/pages/post.mn")
  cache = "1m"
}
```

- Responses are kept by path and query (and the request headers listed in
  `vary = ["Accept-Language"]`), so `cache` is a promise that the page only
  depends on those. rest can't tell: a script can read cookies, `kv`, the
  time, ... Don't cache pages that show something per visitor.
- Only `200`s are kept, not responses that set cookies, and not interactive
  pages (each one is for one visitor's state; a warning is logged).
- Middleware still runs for cached responses (ex. auth).
- Responses get `Cache-Control: public, max-age=<ttl>` (unless the handler
  sets its own), so a CDN or proxy in front of rest can cache them too.
- It stacks with `cache` on request blocks: a page cached for 1m showing data
  cached for 5m can be up to 6m old.
- Only for `GET` handlers with `fn` or `page`, and not `stateful` ones.

## Stateful handlers

`stateful = true` keeps a handler's lua state between requests (globals,
required modules, closures). A request w/o a state id gets a new state, whose
id is `rest.state`; requests that send it back in the `X-Rest-State` header
reuse it (one request at a time). Pages are stateful automatically.

States cost memory (roughly 0.3-2MB each, depending on what's loaded), so the
server keeps at most `max_states` (default 1000, the least recently used is
closed to make room), and closes states unused for `state_ttl` (default 30m).
A request for a closed (or unknown) state gets a `410`, which makes a page
reload itself.

## Pages in Go apps (`rest/pages`)

`github.com/taybart/rest/pages` serves pages as `http.Handler`s, so any Go app
can mount them on its router (net/http, gin, fiber, chi, ...). A page works
like a `page` handler: it renders on `GET`, its actions run on the server,
and only the components that changed are sent back. The `.mn` files are the
same, so they work in both.

It only imports midnight and `rest/lua`, not the rest of rest (hcl, the
client, the server, ...).

```go
import "github.com/taybart/rest/pages"

eng := pages.New(pages.Options{})
defer eng.Close()

mux := http.NewServeMux()
mux.Handle("/app", eng.File("ui/app.mn",
    pages.Props(func(r *http.Request) (any, error) {
        return map[string]any{"name": r.URL.Query().Get("name")}, nil
    }),
    pages.Funcs("app.db", map[string]any{
        "list": db.List, // func(ctx context.Context) ([]Item, error)
        "add":  db.Add,  // func(ctx context.Context, name string) error
    }),
))
```

```midnight
-- ui/app.mn
local { state, map } = require('midnight.markup')
local db = require('app.db')

local Items = () -> {
  local st = state({ error = nil })
  local add = (e) -> {
    local ok, err = pcall(db.add, e.form.name)
    st.error = if ok { nil } else { err }
  }
  return <div>
    <form onsubmit={add}><input type="text" name="name"/></form>
    {st.error && <p>{st.error}</p>}
    <ul>{map(db.list(), (it) -> <li>{it.name}</li>)}</ul>
  </div>
}

return (props) -> <html>
  <body>
    <h1>hello {props.name || 'stranger'}</h1>
    <Items/>
  </body>
</html>
```

A page returns a component (called with the [props](#props)), a markup element,
or a response table (ex. a redirect), like a `page` handler. The element it
returns is rendered once: actions re-render components, so anything that
changes (ex. `data-theme` on `<body>`) has to be in one.

The [example](examples/pages) is a todo app on net/http and fiber, with
embedded files, Go functions, and tests that drive the page like a browser.

### Mounting

A page handler renders on `GET` (and `HEAD`) and runs actions on `POST`, so
mount it for both methods (other methods get a `405`). The app can't have its
own `POST` on the same path.

```go
mux.Handle("/app", h)                              // net/http (no method in the pattern)
mux.Handle("/app/", http.StripPrefix("/app", h))   // under a prefix
r.Any("/app", gin.WrapH(h))                        // gin
app.All("/app", adaptor.HTTPHandler(h))            // fiber (github.com/gofiber/fiber/v3/middleware/adaptor)
```

- Actions POST to the url the browser shows (path and query), so the page
  doesn't need to know its route: prefixes, router groups and proxies just
  work. `pages.ActionURL("/rpc/app")` sends them somewhere else (which has to
  reach the same handler).
- The router's middleware wraps the handler, so it runs for actions too (ex.
  auth, logging, compression). Use the router's own for gzip etc., pages
  doesn't have its own.
- Fiber runs on fasthttp, so `adaptor.HTTPHandler` converts each request and
  response.
- Errors in a page's scripts are a `500` (the error isn't sent, it can have
  source in it), and are passed to `Options.OnError` (logged by default).
- Like `page` handlers, each interactive page load gets its own lua state,
  kept for its actions, and a `410` for a closed one makes the page reload
  (see [Stateful handlers](#stateful-handlers)). Pages w/o actions are
  static: no client script, and no state is kept.

### Where pages come from

| | |
|---|---|
| `eng.File(path, opts...)` | a file on disk. `require` looks next to it first (`require('header')` is `header.mn`, `.midnight`, or `.lua` in its directory), then in `Options.Path`. Changes show up on the next page load. |
| `eng.FS(fsys, name, opts...)` | a file in an `fs.FS`, ex. an `embed.FS`. Compiled once and kept. |
| `eng.Source(src, opts...)` | midnight source. `require` uses `Options.Path`. |

`FS` is for files built into the binary:

```go
//go:embed ui
var ui embed.FS

mux.Handle("/app/", http.StripPrefix("/app", eng.FS(ui, "ui/app.mn")))
```

- `require` only looks in the fs, next to the page: `require('header')` is
  `ui/header.mn` (or `.midnight`, `.lua`, or `ui/header/init.*`), and
  `require('parts.nav')` is `ui/parts/nav.mn`. Not on disk, and not in
  `Options.Path`, so the binary runs the files it was built with, from any
  directory. The `Funcs` / `LuaFuncs` modules and midnight's own still work.
- Files are found and compiled on first use, and kept: later page loads
  don't read the fs at all. So changes to the fs aren't seen; use `File`
  for files that change.
- Paths are `fs.FS` paths (`ui/app.mn`, not `./ui/app.mn`). A bad path, a
  missing file, or a directory panics, so a typo fails at startup.

To edit embedded files while the app runs, serve them with `File` in
development (the example's `-ui` flag does this):

```go
if dev {
    h = eng.File("ui/app.mn", opts...)
} else {
    h = eng.FS(ui, "ui/app.mn", opts...)
}
```

Static files next to a page (css, images, ...) are the router's: ex.
`http.FileServerFS(ui)` / `http.ServeFileFS` for net/http, or `c.SendFile`
with `fiber.SendFile{FS: ui}` on fiber.

### Props

`pages.Props(fn)` gives a page props, by the request that loads it. A page
that returns a component (`return (props) -> <html>...</html>`) is called with
them. They're converted to lua like `Funcs` results (structs become tables,
see [the types](#types)).

```go
pages.Props(func(r *http.Request) (any, error) {
    user, err := auth.User(r)
    if err != nil {
        return nil, err // a 500
    }
    return PageProps{User: user, Tab: r.URL.Query().Get("tab")}, nil
})
```

Props are only for the page load. Actions are later requests, so to get
data then (or to change it), use Go functions.

#### Data for the page's js (`pages.JSON`)

`pages.JSON(v)` encodes `v` with `encoding/json` for the page's own js. In the
page it's opaque (it can be rendered, not read): it goes from Go to the
browser as is, w/o becoming lua tables, so its types are Go's (an empty map
is `{}`, not `[]`) and big data costs the page nothing.

```go
pages.Props(func(r *http.Request) (any, error) {
    return map[string]any{"repos": pages.JSON(repos)}, nil
})
```

Render it in a JSON `<script>` (or `<Data/>`, the same thing), or declare it
in a script with `define:vars`:

```midnight
<Data id="repos" value={props.repos}/>
-- JSON.parse(document.getElementById('repos').textContent)

<script define:vars={{ repos = props.repos }}>
  const names = repos.map(r => r.name)
</script>
```

The JSON never has a `<`, `>`, or `&`, so nothing in the data can close the
`<script>`. An encoding error (ex. a channel in `v`) is the page's error (a
500). Lua values work too: midnight's `json(v)` encodes them, and a JSON
`<script>`'s children are encoded when it's rendered.

### Go functions (`Funcs`)

`pages.Funcs(module, fns)` makes plain Go functions a module of the page, so
it can call into the app (databases, services, ...) when it renders and in
its actions:

```go
pages.Funcs("app.todos", map[string]any{
    "list":   store.List,   // func(ctx context.Context) ([]Todo, error)
    "add":    store.Add,    // func(ctx context.Context, title string) (Todo, error)
    "toggle": store.Toggle, // func(ctx context.Context, id int) error
})
```

```midnight
local todos = require('app.todos')

local list = todos.list()               -- a table of Todos (by json tag)
local ok, err = pcall(todos.add, title) -- Go errors are lua errors
```

- A first param of `context.Context` or `*http.Request` isn't an argument:
  it gets the request being handled. That's the action's request for code
  that runs in an action (so its context, cookies, auth headers, ...), not
  the page load's.
- Results can be `()`, `(T)`, `(error)`, or `(T, error)`. A non-nil error is
  raised as a lua error, `pcall`-able, with the module and function in the
  message: `app.todos.add: title is required`.
- A missing arg is `nil` (an error for a non-pointer type), too many args
  is an error.
- Bad args are lua errors that say where: `app.todos.save: arg 1[2].name:
  expected string, got number`.
- Methods work (`store.List`), so do closures.

The function's signature is checked when the handler is built: an
unsupported one (ex. a `chan` param, or two non-error results) panics, with the
module, function, and param, so it fails at startup, not on a click.

All the reflection on a signature (and its types) happens then too. A call
converts its args, calls the function (`reflect.Value.Call`), and converts
its result: about 150-350ns, vs 35-55ns for a [`LuaFuncs`](#raw-lua-functions-luafuncs)
function. A whole action (dispatch, render, diff) is about 40µs, so it's
rarely worth the difference.

#### Types

| Go | midnight |
|---|---|
| `string`, `[]byte` | string |
| `bool` | boolean |
| ints, uints, floats | number (into an int: only integers in range) |
| `[]T` | array table (a nil slice is `{}`) |
| `map[string]T` | table (a nil map is `{}`) |
| `struct`, `*struct` | table by field name, or its `json` tag (`json:"-"` and unexported fields skipped, embedded structs flattened, `omitempty` / `omitzero` fields left out when empty, like `encoding/json`) |
| `time.Time` | string (RFC 3339) |
| `any` | by the value's type (into Go: `string`, `float64`, `bool`, `[]any`, `map[string]any`, or `nil`) |
| `*T` | `nil` or `T` |

Recursive types work. Channels, funcs, complex numbers, non-string map
keys, and interfaces with methods aren't supported.

Midnight's defaults (`local { q = '?' } = props`) only apply to `nil`, and a
Go zero value (`""`, `0`, `false`) isn't one. Tag a field whose zero value
means "not set" `omitempty` (or `omitzero`), and it's `nil` in lua when it's
empty, so the default applies.

### Raw lua functions (`LuaFuncs`)

For full control, or a hot path, `pages.LuaFuncs(module, fns)` makes plain
gopher-lua functions (`func(l *lua.LState) int`, `lua` being
`github.com/taybart/midnight`) a module. They get their args from the stack,
push their results, and raise errors with `l.RaiseError` / `l.ArgError`.
`pages.Request(l)` is the request being handled (the action's, in an
action):

```go
pages.LuaFuncs("app.user", map[string]lua.LGFunction{
    "name": func(l *lua.LState) int {
        user := auth.FromContext(pages.Request(l).Context())
        l.Push(lua.LString(user.Name))
        return 1
    },
})
```

An `LGFunction` in a `Funcs` map is used as is too, to mix them in one
module.

### Exports and `rest.request`

Pages see the same `rest` global as in the server: `rest.req` (the request
being handled), `rest.state` (the page's state id), and `rest.exports`.
`rest.exports` starts from `Options.Exports`, then what
`pages.Exports(fn)` returns for the page load. It's kept for the page's
actions (`fn` isn't called again for them).

`rest.request(label [, exports])` runs a `.rest` file's request blocks (see
[Fetching data](#fetching-data-restrequest)) when the engine has a
`Requester`. That needs the root `rest` package (and so hcl etc.), so only
apps that use it import it:

```go
f, err := rest.NewFile("api.rest")
// ...
c, err := client.New(request.Config{})
// ...
eng := pages.New(pages.Options{
    Requester: f.Requester(c),
    Exports:   map[string]any{"base_url": os.Getenv("API_URL")},
})
```

W/o a `Requester`, `rest.request` raises an error. There's no `kv` or
`setup`: apps use Go functions for their data.

### Caching (`Cache`)

`pages.Cache(ttl, vary...)` works like [`cache`](#caching-pages-cache) on a
handler: a page that's the same for every visitor is rendered once per
`ttl`, and served from memory.

```go
eng.FS(ui, "ui/about.mn", pages.Cache(5*time.Minute, "Accept-Language"))
```

- Responses are kept by path, query, and the `vary` request headers: a
  promise that the page only depends on those (pages can't tell what a
  page's scripts or Go functions read).
- Only `200`s that don't set cookies are kept. Interactive pages are never
  cached (each one has its visitor's state id); a warning is logged once.
- Responses get `Cache-Control: public, max-age=<ttl>` unless the page sets
  its own.
- At most 1000 responses / 64MB per page, the oldest are dropped first.
- Router middleware runs for cached responses too (it's outside of the
  handler).

### Engine options

```go
eng := pages.New(pages.Options{
    MaxStates: 1000,             // kept page states, shared by every page (default 1000)
    StateTTL:  30 * time.Minute, // close states unused this long (default 30m)
    Path:      "./lib/?.mn;./lib/?.lua", // more places for require (not used by FS pages)
    Requester: f.Requester(c),   // for rest.request (see above)
    Exports:   map[string]any{}, // where rest.exports starts
    OnError: func(r *http.Request, err error) { // errors from page scripts (default: log)
        slog.Error("page", "path", r.URL.Path, "err", err)
    },
})
defer eng.Close() // closes the kept states
```

An app usually has one engine: its pages share the state store, so
`MaxStates` limits the memory of all of them (states are roughly 0.3-2MB
each).

| Page option | |
|---|---|
| `Props(fn)` | the page's props, by request |
| `Funcs(module, fns)` | Go functions as a module |
| `LuaFuncs(module, fns)` | gopher-lua functions as a module |
| `Exports(fn)` | adds to `rest.exports`, by request |
| `ActionURL(url)` | where actions POST (default: the page's url) |
| `Cache(ttl, vary...)` | cache rendered responses |
