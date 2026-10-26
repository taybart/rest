# Rest Server

## CLI

```sh
    --addr, -a:
	Address to listen on
    --serve, -s:
	Run a server
    --dir, -d:
	Directory to serve
    --spa:
	Serve index.html in directory instead of 404
    --file, -f:
	File to run
    --cors:
	Add cors headers
    --response, -r:
	Response to send, json file path or inline in the format {"status": 200, "body": {"hello": "world"}}
    --tls, -t:
	TLS path name to be used for tls key/cert (defaults to no TLS)
	ex: '-t ./keys/site.com' where the files ./keys/site.com.{key,crt} exist
    --tailscale:
	Serve https using this machine's tailscale cert (needs the tailscale cli and HTTPS enabled on the tailnet)
	listens on :8080 unless --addr is set so other tailnet devices can connect
    --quiet, -q:
	Don't log server requests
```

**response.json**

```json
{
  "status": 200,
  "headers": {
    "Content-Type": "application/json"
  },
  "body": {
    "custom": "response"
  }
}
```


### Routes

There are some build in routes to help with testing and development:

- `/__echo__` - returns the request headers (only ones that begin with `x-`) and body
- `/__ws__` - echo websocket messages back
- `/__quit__` - exit the server process

NOTE: these routes will be overridden if you provide a custom handler


### Examples

```sh
# run server with default config (localhost:8080, 200 on any request, no cors)
rest -s

# serve a directory with cors and tls
rest -s -a 0.0.0.0:18080 -t ./tls/example.com -d ./dist --cors

# serve a directory over https to your tailnet at https://<machine>.<tailnet>.ts.net:8085
rest -s -a :8085 -d . --tailscale

# serve with a custom response
rest -s -r response.json
```


## Server rest file

```hcl
server {
    address = "localhost:18080"
    # TLS path name to be used for tls key/cert (defaults to no TLS)
    # ex: './keys/site.com' where the files ./keys/site.com.{key,crt} exist
    tls = "test/keys/example.com"
    # or serve https with this machine's tailscale cert instead (default false)
    # tailscale = true
    # don't dump requests (default false)
    quiet = true
    # add cors headers (default false)
    cors = true
    # serve a directory, this will override response if provided (except headers)
    directory = "./test"
    # should this be treated as a single page application (ie frontend routing) by returning index.html instead of 404 (default false)
    spa = true
    # if you need a more complicated test server you can add specific handlers
    handler "GET" "/path" {
        # override responses and just serve a websocket echo path
        ws = true
        # either use lua to create a more complex response
        fn = "similar concept to the after hook in the client files (see hander fns below)"
        # or use a response object to just have different responses per path
        response {
            status = 200
            headers = { "x-custom-header": "custom-header" }
            body = { "custom": "response" }
        }
    }
    # not required, override the default response if needed
    response {
        status = 200
        # add headers to response
        headers = {
            "x-custom-header": "custom-header",
            "x-env-header": env("HEADER_VALUE")
        }
        body = { "custom": "response" }
    }
}
```

### Handler functions

The handler function has access to the same lua tools as the client after hook.
The return must be a table (its the same type as the response block in HCL, plus an optional `cookies` table):
ex:
```lua
return {
    status = 200,
    headers = { ["X-Custom"] = "test" },
    cookies = {
        token = {
            value = "secret",
            path = "/",
            http_only = true,
            same_site = "strict", -- "lax", "strict", "none", "default"
            secure = true,
            max_age = 3600,
        },
        theme = "dark", -- shorthand for simple key=value cookies
    },
    body = json.encode({msg = "hello world"}),
}
```

Handler functions are passed a global `rest` table containing `rest.path`, `rest.ctx`, and `rest.req`:

```lua
rest.req = {
    url = "http://localhost:18080/user/5",
    method = "GET",
    query = {},
    headers = {},
    cookies = {},
    body = "",
}
```

### Seeding State (`kv` and `setup`)

You can pre-populate the in-memory `kv` cache declaratively in HCL with `kv = { ... }`, or run one-time startup logic with `setup = <<LUA ... LUA` (or both—`kv` is loaded first, then `setup` runs):

```hcl
server {
    address = "localhost:18080"

    kv = {
        db = {
            users = {
                1 = { name = "Bryan Cranston", role = "dad" },
                4 = { name = "Frankie Muniz", role = "middle" },
            }
        }
    }

    setup = <<LUA
        kv.set("boot_token", uuid.newv4())
    LUA
}
```

### Middleware

You can define reusable `middleware` blocks in `server` and reference them on any `handler` via `middleware = ["name"]` (including `fn`, `response`, and `proxy` handlers).

- If a middleware returns `nil` (or nothing), execution proceeds to the next middleware or handler.
- If a middleware returns a response table (e.g. `return { status = 401, body = ... }`), it short-circuits the request and sends that response immediately.
- Middleware and handler `fn` scripts share `rest.ctx` (a table) for passing values downstream within the same request.

```hcl
server {
    address = "localhost:18080"

    middleware "auth" {
        fn = <<LUA
            local token = rest.req:cookie("token") or rest.req:bearer_token()
            if token ~= kv.get("last_token") then
                return {
                    status = 401,
                    body = json.encode({ error = "unauthorized" }),
                }
            end
            rest.ctx.token = token
        LUA
    }

    handler "GET" "/user/{id}" {
        middleware = ["auth"]
        fn = <<LUA
            return {
                status = 200,
                body = json.encode({
                    id = rest.req:path_value("id"),
                    token = rest.ctx.token,
                }),
            }
        LUA
    }
}
```

`rest.req` also provides helper methods (called with `:`):

- `rest.req:path_value(key)` - get a route path parameter (e.g. `{id}` from `/user/{id}`)
- `rest.req:query(key)` - get the first value of a query parameter (or `nil`)
- `rest.req:query_values(key)` - get all values of a query parameter as a table (or `nil`)
- `rest.req:header(key)` - case-insensitive lookup returning the first header value and matched key (or `nil`)
- `rest.req:header_values(key)` - case-insensitive lookup returning all values for the header as a table and matched key (or `nil`)
- `rest.req:cookie(name)` - get the value of a request cookie by name (or `nil`)
- `rest.req:bearer_token()` - extract the token from the `Authorization: Bearer <token>` header (or `nil`)

**Modules**

There are some global modules available in the lua runtime.

- `json` - encode and decode json (`json.encode(tbl)`, `json.decode(str)`)
- `form` - encode and decode url-encoded form/query strings (`form.encode(tbl)`, `form.decode(str)`)
- `colors` - adds terminal color escape codes and formatting functions for extra points
- `inspect` - used to inspect lua values
- `base64` - encode and decode base64
- `uuid` - generate and validate UUIDs
- `tools` - various helper functions, check out the [tools](https://github.com/taybart/rest/blob/main/lua/modules/tools.lua) module for commented functions
- `kv` - in-memory key/value store that persists between requests (concurrency-safe)
    - `kv.get(key)` - get value from kv cache, returns nil if key doesn't exist
    - `kv.set(key, value)` - set a key's value, can be any Lua value or nested table (passing `nil` deletes the key)
    - `kv.del(key)` - delete a key from the kv cache
    - `kv.all()` - return a table of all key/value pairs in the kv cache

See [examples/server](./examples/server) for a more detailed examples
