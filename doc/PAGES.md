# Pages

rest's server can also serve HTML apps written in [erde](https://erde-lang.github.io)
w/ go-erde's markup (HTML-like elements, components, and scoped styles):
handlers that return HTML, interactive `page`s whose events run on the server,
and data fetched w/ the file's own `request` blocks.

See [SERVER.md](SERVER.md) for the server itself (handlers, middleware, `kv`,
`setup`, ...), and go-erde's [markup docs](https://github.com/taybart/go-erde/blob/main/docs/MARKUP.md)
for the syntax and runtime.

- [Returning HTML](#returning-html)
- [Pages (`page`)](#pages-page)
- [Fetching data (`rest.request`)](#fetching-data-restrequest)
- [Caching pages (`cache`)](#caching-pages-cache)
- [Stateful handlers](#stateful-handlers)

## Returning HTML

A handler (or middleware) can return a markup element instead of a response table:
it's rendered as an HTML document (`text/html`). Or use it as the body of a
response table, to also set the status, headers, or cookies.

```hcl
handler "GET" "/hello" {
  fn = <<-ERDE
    local name = rest.req:query('name') || 'stranger'
    return <html><body><h1>hello {name}</h1></body></html>
  ERDE
}
handler "GET" "/missing" {
  fn = <<-ERDE
    return { status = 404, body = <p>nothing here</p> }
  ERDE
}
```

See go-erde's [markup docs](https://github.com/taybart/go-erde/blob/main/docs/MARKUP.md)
for the syntax, components, `state`, and scoped styles (`css`).

## Pages (`page`)

A `page` is like `fn`, but returns a markup element that's served as an
interactive page: functions in `on*` attributes (`onclick`, `on:input`, ...)
run on the server when the event happens in the browser, and only the
components that changed are sent back (and morphed into the page, so inputs
keep focus).

```hcl
handler "GET" "/{$}" {
  page = <<-ERDE
    local App = require('app')   -- ./app.erde
    return <App name={rest.req:query('name')}/>
  ERDE
}
```

```erde
-- app.erde
local { state, css } = require('erde.markup')

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
- Static files (css, images, ...) can be served next to pages w/ the
  server's `directory`: it serves whatever no handler matches (so use
  `"/{$}"` for a page at the root, `"/"` matches everything).
- Changes to required `.erde` files show up on the next page load (the
  `.rest` file itself needs a restart).

## Fetching data (`rest.request`)

Pages get data by running the file's `request` blocks (including imported
ones) w/ `rest.request(label [, exports])` (or a block index), so the requests
an app makes are defined, run, tested, and exported like any others (see
[CLIENT.md](CLIENT.md#running-requests-from-lua-restrequest) for what it does
and returns). A block's `after` hook can post process its response: what it
returns is what `rest.request` returns.

```hcl
# hn.rest: runs on its own too (rest -f hn.rest -l item)
request "item" {
  url    = "https://api.hackerwebapp.com/item/${exports::try("id", 8863)}"
  expect = 200
  after  = [lib.expected, <<-ERDE
    local item = rest.res:json()
    return { title = item.title, url = item.url || "/post/{item.id}" }
  ERDE
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
    page = <<-ERDE
      local Post = require('post')
      local item = assert(rest.request('hn::item', { id = rest.req:path_value('id') }))
      return <Post item={item}/>
    ERDE
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
served from memory, w/ `cache` on its handler:

```hcl
handler "GET" "/hn/post/{id}" {
  page  = read("src/pages/post.erde")
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
- It stacks w/ `cache` on request blocks: a page cached for 1m showing data
  cached for 5m can be up to 6m old.
- Only for `GET` handlers w/ `fn` or `page`, and not `stateful` ones.

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
