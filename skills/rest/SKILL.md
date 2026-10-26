---
name: rest
description: Write, run, test, debug, and export `.rest` files using the `rest` HCL-based HTTP/WebSocket client, mock server, and interactive CLI builder. Use this skill whenever the user mentions `.rest` files, asks to make or run a `rest` file/script/server/client, uses `rest` HCL blocks (`request`, `server`, `socket`, `cli`, `config`, `locals`, `expect`, `handler`, `middleware`), or wants to quickly mock an HTTP/WebSocket server or test HTTP endpoints using `rest`.
---

# `rest` (`.rest` HCL Files)

`rest` is an HCL-based HTTP/WebSocket client, mock/dev server, and interactive CLI builder with an embedded Lua runtime.

For full reference documentation and working examples, read:
- `references/CLIENT.md` — Detailed client config, functions, Lua `after` hooks, `expect` matchers, WebSockets, exports, and `cli` blocks.
- `references/SERVER.md` — Detailed server config, routes, `handler` functions, `middleware`, `kv` store, and `setup` hooks.
- `references/examples/` — Complete client (`references/examples/client/`) and server (`references/examples/server/`) `.rest` files.

---

## Running `rest` from the CLI

### Client & CLI Tools
```sh
# Run all non-skipped requests in a file in order
rest -f api.rest

# Run a single request by label or 0-based index
rest -f api.rest -l "login"
rest -f api.rest -b 0

# List request labels in a file
rest -f api.rest --list

# Run from stdin / heredoc without creating a file
rest <<'EOF'
request "check" {
  url   = "https://httpbin.org/get"
  after = lib.inspect_res
}
EOF

# Ignore failures and run all blocks
rest -f api.rest --ignore-fail

# Run WebSocket block: interactive REPL, full playbook order, or a single entry
rest -f api.rest -S
rest -f api.rest -S run
rest -f api.rest -S ping

# Export requests to another format (curl, go, js, postman; use `ls` to list)
rest -f api.rest -e curl
rest -f api.rest -l "login" -e curl
```

### Server
```sh
# Run the `server {}` block in a .rest file
rest -s -f server.rest

# Quick ad-hoc server without a .rest file (default localhost:8080)
rest -s -a localhost:18080 -d ./dist --spa --cors
rest -s -r '{"status": 200, "body": {"ok": true}}'
rest -s -d . --tailscale
```

---

## Core File Structure & Client Syntax

A `.rest` file can contain `imports`, `config`, `locals`, `request`, `socket`, `cli`, and `server` blocks. You can even put `server {}` and test `request` blocks in the same `.rest` file (`rest -s -f file.rest` runs the server; `rest -f file.rest` runs the client requests against it).

### `config`, `locals`, and `imports`
```hcl
imports = ["./base.rest"]

config {
  no_cookies             = false
  no_follow_redirect     = false
  user_agent             = "rest-client/2.0"
  insecure_no_verify_tls = false
  namespace_imports      = true  # prefixes imported request labels with `filename::label`
  skip_imported          = false # set true when importing a shared library of base requests
  no_flags               = false # set true in executable `cli` scripts so custom flags can use any name
}

# Note: locals are preprocessed across the file; the last value assigned to a key wins globally.
locals {
  base_url = env("API_URL") != "" ? env("API_URL") : "http://localhost:18080"
}
```

### `request` Blocks, `expect`, and `after` Hooks
```hcl
request "base" {
  skip    = true # template request; won't run during `rest -f`, can be inherited via copy_from
  headers = { "X-App" = "demo" }
}

request "login" {
  copy_from = "base" # or "imported_file::label" when namespace_imports = true
  url       = "${locals.base_url}/login"
  method    = "POST"
  body      = { username = "root", password = "secret" }

  expect {
    status  = 200
    headers = { "content-type" = "application/json" }
    body    = {
      token = is.uuidv4
      id    = is.int
      code  = is("regex:^[A-Z]{3}-[0-9]+$")
    }
  }

  # `after` accepts a pre-canned `lib.*` hook, helper call, `<<LUA ... LUA` string, or a list of hooks
  after = [
    lib.expected,
    inspect_res("id", "token"),
    export_res({ auth_token = "token" }),
  ]
}

request "get profile" {
  url          = "${locals.base_url}/me"
  bearer_token = exports.auth_token # or try_exports("auth_token", "fallback")
  query        = { expand = "roles" }
  expect       = 200
  after        = [lib.expected, lib.inspect_res]
}
```

### Built-in HCL Functions & Matchers
- **Environment & Files:** `env("VAR")`, `read("./path/to/file")`
- **Encoding & Templates:** `json_enc(map)`, `json_dec(str)`, `form(map)`, `btmpl(str, map_or_list)`, `tmpl(str, map)`
- **Exports:** `exports.<key>` (or `rest.exports.<key>`), `try_exports("key", "default")`
- **Pre-canned `after` hooks (`lib`):**
  - `lib.expected` — checks the `expect` status/block and fails the run on mismatch (always include in `after` when both `expect` and `after` are set, as custom `after` hooks override automatic `expect` checking unless `lib.expected` or `tools.expected()` is called)
  - `lib.inspect_res`, `lib.inspect_req` — JSON-decode and pretty-print response/request body
  - `lib.print_req_res`, `lib.print_req`, `lib.print_res`, `lib.print_body`, `lib.print_label`
- **Hook helper functions (usable directly in `after = [...]`):**
  - `inspect_res("id", "nested.field")` or `inspect_res({ alias = "nested.field" })`
  - `inspect_req("field")`
  - `export_res("id", "token")` or `export_res({ auth_token = "auth.token" })`
  - `copy_res("token")` (copies field or whole body to clipboard)
- **`expect` matchers (`is`):**
  - `is.uuid`, `is.uuidv4`, `is.nanoid`, `is.email`, `is.url`
  - `is.string`, `is.number`, `is.int`, `is.bool`, `is.array`, `is.object`, `is.null`, `is.any`, `is.not_empty`
  - `is("regex:<pattern>")` or `is("regex", "<pattern>")`

---

## Custom Lua in `after` Hooks

When writing `after = <<LUA ... LUA`:
- **Globals:** `json` (`encode`/`decode`), `form` (`encode`/`decode`), `inspect` (`inspect(v)`, `inspect.print(v)`), `colors`, `base64`, `uuid` (`newv4`, `is_uuid`, `is_uuidv4`), `tools` (`tools.expected()`), `copy(val)`, `fail(msg)`.
- **Request & Response (`rest.req`, `rest.res`):**
  - Fields: `rest.req.body`, `rest.req.headers`, `rest.req.cookies`, `rest.req.method`, `rest.req.query`, `rest.req.url`, `rest.req.dump`
  - Fields: `rest.res.body`, `rest.res.headers`, `rest.res.cookies`, `rest.res.status`, `rest.res.dump`
  - Methods (use `:`): `rest.req:header("name")`, `rest.req:query("key")`, `rest.req:cookie("name")`, `rest.req:bearer_token()`, `rest.res:header("name")`, `rest.res:cookie("name")`
- **Sharing state to subsequent requests:** `rest.exports.my_key = "value"`

---

## Building Executable CLI Tools (`cli` Block)

Add `#!/usr/bin/env rest` and `config { no_flags = true }` to create standalone CLI scripts (`chmod +x tool.rest && ./tool.rest`):

```hcl
#!/usr/bin/env rest

config { no_flags = true }

cli {
  flags = {
    user = {
      desc     = "Username to look up"
      short    = "u"
      required = true
    }
    loud = {
      desc  = "Verbose output"
      short = "l"
      bool  = true
    }
  }

  fn = <<LUA
    -- Flags are on `cli.<name>` (or `cli["flag-name"]`); positional args on `cli.rest` (`cli.rest:string()`)
    rest.exports.user = cli.user
    rest.label("fetch user")
    if cli.loud then
      print(rest.exports.result)
    end
    -- Optional: call `rest.loop()` to enter the interactive `loop` block if defined
  LUA
}

request "fetch user" {
  url   = "https://api.example.com/users/${try_exports("user", "default")}"
  after = export_res({ result = "name" })
}
```
For interactive REPL loops (`loop = <<LUA ... LUA`, `PROMPT`, `input`, `goto continue`), consult `references/CLIENT.md`.

---

## WebSockets (`socket` Block)

```hcl
socket {
  url = "ws://localhost:8080/ws"
  run = {
    delay = "100ms"
    order = ["connect", "sub", "noop"] # "noop" waits one delay interval for server messages
  }
  playbook = {
    connect = raw("CONNECT ${env("TOKEN")}")       # sends raw text frame without JSON quotes
    sub     = { msg = "subscribe", chan = "news" } # JSON-encoded automatically
  }
}
```

---

## Mock & Dev Server (`server` Block)

Run with `rest -s -f server.rest`. Built-in debug routes (unless overridden): `/__echo__`, `/__ws__`, `/__quit__`.

```hcl
server {
  address   = "localhost:18080"
  cors      = true
  quiet     = false
  # directory = "./public"
  # spa       = true
  # tls       = "./keys/site.com" # expects .key and .crt
  # tailscale = true

  # Seed in-memory concurrency-safe `kv` store
  kv = {
    users = {
      "1" = { name = "Alice" }
    }
  }

  # Optional one-time startup Lua hook
  setup = <<LUA
    kv.set("boot_token", uuid.newv4())
  LUA

  middleware "auth" {
    fn = <<LUA
      local token = rest.req:cookie("token") or rest.req:bearer_token()
      if token ~= kv.get("boot_token") then
        return { status = 401, body = json.encode({ error = "unauthorized" }) }
      end
      rest.ctx.token = token -- pass data to downstream handlers via `rest.ctx`
    LUA
  }

  handler "GET" "/health" {
    response {
      status = 200
      body   = { ok = true }
    }
  }

  handler "GET" "/users/{id}" {
    middleware = ["auth"]
    fn = <<LUA
      local id = rest.req:path_value("id")
      local users = kv.get("users")
      local user = users and users[id]
      if not user then
        return { status = 404, body = json.encode({ error = "not found" }) }
      end
      return {
        status  = 200,
        headers = { ["X-Req-Token"] = rest.ctx.token },
        body    = json.encode(user),
      }
    LUA
  }
}
```
For cookie options in server responses, WebSocket echo handlers (`ws = true`), proxy handlers, and full `kv` methods (`kv.get`, `kv.set`, `kv.del`, `kv.all`), consult `references/SERVER.md`.
