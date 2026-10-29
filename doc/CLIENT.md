# Rest Client

Rest uses HCL to define requests and run them.
It also has a lua interpreter to post-process responses.

1. [Cli](#client-cli)
1. [Config/Locals](#configlocals)
1. [Request Blocks](#request-blocks)
1. [Functions](#functions)
1. [After Hooks](#after-hooks)
1. [Export](#export-to-a-different-language)
1. [Sockets](#sockets)
1. [CLI Block](#cli-block)

## Client cli

```sh
# run file
rest -f FILE_NAME
# run block 0-based
rest -f FILE_NAME -b BLOCK_NUMBER
# run by label (request "LABEL_NAME" {)
rest -f FILE_NAME -l LABLE_NAME
# run from stdin / heredoc
rest <<'EOF'
request "test" {
  url = "https://httpbin.org/get"
}
EOF
```

You can also turn a rest file into an executable script by adding a shebang at the top (see below):

```hcl
#!/usr/bin/env rest

cli {
  ...
}
```

Then make it executable and run it directly:

```sh
chmod +x mytool.rest
./mytool.rest --name Alice
```

When invoked via shebang, rest will automatically use the script file as `-f`, so you don't need to pass it explicitly.


## Config/Locals

The config block is used to set global options for the client, there can only be one config block

```hcl
# defaults
config {
  # when true, all cookies are ignored
  no_cookies = false
  # when true, all redirects are ignored
  no_follow_redirect = false
  # override default user agent
  user_agent = "rest-client/2.0"
  # don't verify tls certs
  insecure_no_verify_tls = false
  # add filename prefix to imported requests label -> filename::label
  namespace_imports = true
  # don't execute requests that were imported (library creation)
  skip_imported = false
  # disable built-in rest CLI flags so the cli {} block owns all flags
  no_flags = false
  # automatically set Content-Type = "application/json" when body is JSON and no Content-Type header is specified
  default_content_type = true
  # language for scripts in this file (after, cli fn/loop, server setup/middleware/handler fn)
  # "lua" or "erde" (https://erde-lang.github.io), imported files use their own config.
  # when unset, each script uses its heredoc marker (<<LUA / <<ERDE), otherwise lua
  # lang = "erde"
}
```

Locals just set variables that can be referenced in request blocks, there can be many locals blocks.

There is something to be aware of though, locals are preprocessed so the last value set for a particular local will be its value for the whole file.

```hcl
locals {
  name = "hello"
}

# url will be http://localhost:8080/world in this block,
# since the last value in the file is world
request "first" {
  url = "http://localhost:8080/${locals.name}"
}

locals {
  name = "world"
}

request "second" {
  url = "http://localhost:8080/${locals.name}"
}
```

## Request Blocks

Requests are defined in the `request` block (duh), they require some kind of label.

```hcl
request "my request" {
  # skip request, this is useful when creating a base request
  # skip is not copied over and not used when comparing requests
  skip = false

  # url must include protocol (http(s) for now)
  url = "http://localhost:8080/"

  # if not specified, defaults to GET
  method = "GET"

  # there are convenience keys to set auth headers
  # basic auth can be set
  basic_auth = "username:password"
  # or a bearer token can be used
  bearer_token = "token"

  # map of headers
  headers = { "X-Test" = "test" }

  # only single level query params are allowed so maps must be explicitly defined
  query = {
    b = "2"
    "config[key]" = "value"
  }

  # body can look like a json object or a regular hcl map or a string
  # maps/objects are marshalled to JSON by default (and Content-Type = "application/json"
  # is added automatically unless another Content-Type header is set or default_content_type = false),
  # or url-encoded automatically if headers includes Content-Type = "application/x-www-form-urlencoded"
  body = { test: "body" } # or body = { test = "body" }

  # cookies can be set for a single request
  cookies = { a = "1" }

  # if you only want to check the status code
  expect = 200
  # otherwise, use the expect block, this will take priority over the expect status above
  # if a hook is not defined response code will be checked agains this value and fail if it
  # doesn't match (ie return 1), fields are optional (you can just check one or more of them)
  expect {
    status = 200 # response must have status code
    headers = { # response must contain these headers (only checks provided headers)
      "x-custom-header" = "custom-header"
      "x-request-id"    = is.uuid
    }
    body = { # response must match this JSON shape and values
      "test": "response",
      "token": is.uuidv4,
      "count": is.int,
      "code": is("regex:^[A-Z]{3}-[0-9]+$")
    }
    # optional: set exact = true to require raw byte-for-byte body string equality
    # exact = true
  }

  # is a string (or list of strings), heredoc (<<IDENT ... IDENT) is a good way to set it
  # using LUA as the ident can make some editors highlight the code better
  after = "see after hooks below"
}
```

## Functions

There are a few functions that can be used in a rest file:

- `env("VALUE")` - grab an environment variable
- `read("./filepath")` - read a file into the rest file, this will just be read into a string so it can be used anywhere (ex. request body,
- `json_enc({key = "value"})` - turn an HCL table into a string value, useful for templates
- `json_dec("{\"string\": \"json\"}")` - turn an string into a table value
- `jwt_sign({sub = "123", role = "admin"}, "secret", {alg = "HS256", expires_in = 3600})` (or `jwt_enc(...)`) - sign a JWT token (supports `HS256`/`384`/`512`, `RS256`/`384`/`512`, `ES256`/`384`/`512`, `EdDSA`)
- `jwt_dec(token)` - decode a JWT's claims into an HCL object without verifying the signature
- `form({key = "value"}")` - turn map value into a url-encoded form string
- `btmpl("{\"string\": \"{{named}}\"}", {named = "world"})` - execute a basic template replacing named or indexed values if second argument is an array
- `tmpl("{{{if .named}}\"string\": \"{{.named}}\"{{end}}}", {named = "world"})` - execute a go template with a map (currently only map[string]strings are supported)
- `raw("CONNECT token")` - mark a [socket](#sockets) playbook entry to be sent as a raw text frame instead of being JSON encoded
- `lib::inspect_res("id", "name")` / `lib::inspect_res(["id", "name"])` / `lib::inspect_res({ user_id = "user.id" })` - after hook helper that decodes the JSON response body and prints selected fields (supports dot-paths)
- `lib::inspect_req("id", "name")` - after hook helper that decodes the JSON request body and prints selected fields
- `lib::inspect_jwt()` / `lib::inspect_jwt("auth.token", "sub", "role")` - after hook helper that decodes and pretty-prints a JWT from the response or request bearer token
- `lib::export_res("id", "name")` / `lib::export_res({ token = "auth.token" })` - after hook helper that exports fields from the JSON response body into `rest.exports`
- `lib::copy_res("token")` - after hook helper that copies a field from the JSON response body (or the whole body if no key is passed) to the clipboard
- `is.<type>` / `is("...")` - expectation matchers for `expect { body = ... }` and `expect { headers = ... }`:
  - `is.uuid`, `is.uuidv4`, `is.ulid`, `is.nanoid`, `is.jwt`, `is.email`, `is.url`
  - `is.string`, `is.number`, `is.int`, `is.bool`, `is.array`, `is.object`, `is.null`, `is.any`, `is.not_empty`
  - `is("regex:<pattern>")` or `is("regex", "<pattern>")` - match a string against a regular expression

For example (more examples in [examples/client](examples/client)):

```hcl
locals {
    # contents of body.json: { "hello": "from a file" }
    partial_body = read("./body.json")
}

request "my request" {
  url = "http://localhost:8080/"
  headers = {
    "Authorization" = "Bearer ${env("token")}"
  }
  # will be {"test": {"hello": "from a file"}}
  body = {
    test: json_dec("${locals.partial_body}")
  }
  # or just used directly
  body = json_dec(locals.partial_body)
  # or url encoded form string
  body = form({ hello = "world" })
}
```

## After Hooks

Hooks are defined in the `after` field. They are lua functions that are executed after the response is received. They can be used to do fancy lua stuff.

### Pre-canned Hooks (`lib`) and Hook Functions

For common tasks, you don't need to write a `<<LUA` block. You can use the built-in `lib` table, hook functions, or pass a list of hooks to run in sequence:

- `lib.dump` - print the full HTTP dump of both request and response
- `lib.print_req` - print the HTTP request dump
- `lib.print_res` - print the HTTP response dump
- `lib.print_body` - print the raw response body
- `lib.inspect_res` - JSON-decode and `inspect.print` the response body
- `lib.inspect_req` - JSON-decode and `inspect.print` the request body
- `lib.inspect_jwt` - decode and `inspect.print` the JWT header and claims from the response or request bearer token
- `lib.expected` - run `tools.expected()` to validate the `expect` block/status

```hcl
request "login" {
  url    = "http://localhost:8080/login"
  method = "POST"
  expect = 200
  after  = [
    lib.expected,
    lib::inspect_res("id", "name"),
    lib::export_res({ token = "auth.token" }),
  ]
}

request "debug" {
  url   = "http://localhost:8080/debug"
  after = lib.dump
}
```

### Lua Runtime

There are a couple of global libraries available to you:

- `json` - encode and decode json (`json.encode(tbl)`, `json.decode(str)`)
- `jwt` - sign, decode, and verify JWTs (`jwt.sign(claims, secret [, opts])`, `jwt.decode(token)`, `jwt.verify(token, secret [, opts])`, `jwt.is_valid(token [, secret, opts])`)
- `form` - encode and decode url-encoded form/query strings (`form.encode(tbl)`, `form.decode(str)`)
- `colors` - adds terminal color escape codes and formatting functions for extra points
- `inspect` - used to inspect lua values
- `base64` - encode and decode base64 (`base64.encode(str [, { url = true, raw = true }])`, `base64.decode(str)`)
- `uuid` - generate and validate UUIDs (`uuid.newv4()`, `uuid.is_uuid(str)`, `uuid.is_uuidv4(str)`)
- `ulid` - generate and validate ULIDs (`ulid.new([ms])` or `ulid([ms])`, `ulid.is_ulid(str)`, `ulid.timestamp(str)`)
- `nanoid` - generate and validate NanoIDs (`nanoid.new([size [, alphabet]])` or `nanoid([size [, alphabet]])`, `nanoid.is_nanoid(str [, size])`)
- `tools` - various helper functions (such as `tools.expected()`), check out the [tools](https://github.com/taybart/rest/blob/main/lua/modules/tools.lua) module for commented functions

Additionally there are a few global functions available to you:
- `copy("value")` - copy a value to the clipboard, if the value is a table it will be marshalled to json
- `fail("message")` - return a clean error to the rest cli so it can fail outside of the lua vm
                    (as opposed to `error("message")`, that does fail in the vm)

Hooks are also passed a `rest` table that contains the following:

`rest.label` - the label of the request block

`req` - the request object

```lua
{
  body = "" -- string representation of the request body (can be parsed with json.decode())
  headers = {}, -- headers table of the request
  cookies = {}, -- cookies table that was sent during the request
  method = "GET", -- method used for the request
  query = {}, -- query table of the request
  url = "https://httpbin.org/get" -- url used for the request
  dump = "" -- formatted full request in http format
}
```

`rest.req` methods:
- `rest.req:query(key)` - get the first value of a query parameter (or `nil`)
- `rest.req:query_values(key)` - get a table of values for a query parameter (or `nil`)
- `rest.req:header(key)` - case-insensitive lookup returning the first header value and matched key (or `nil`)
- `rest.req:header_values(key)` - case-insensitive lookup returning a table of values and matched key (or `nil`)
- `rest.req:cookie(name)` - get the value of a request cookie by name (or `nil`)
- `rest.req:bearer_token()` - extract the token from the `Authorization: Bearer <token>` header (or `nil`)

`res` - the response object

```lua
{
  body = "" -- string representation of the response body (can be parsed into a lua table with json.decode())
  cookies = {}, -- cookies that were set during the request
  headers = {}, -- headers table returned by the server
  status = 200 -- status code returned by the server
  dump = "" -- formatted full response in http format
}
```

`rest.res` methods:
- `rest.res:header(key)` - case-insensitive lookup returning the first header value and matched key (or `nil`)
- `rest.res:header_values(key)` - case-insensitive lookup returning a table of values and matched key (or `nil`)
- `rest.res:cookie(name)` - get the value of a response cookie by name (and a second return value with cookie attributes when set via `Set-Cookie`)

### exports

You can grab values from responses and put them in the `exports` table. This is available in requests below when the value is set.
It can be used in the HCL of rest files or in `after` hooks (`rest.exports` in lua land). One use case is to authenticate in the first request block and use the returned auth token in the next request block.

```hcl
request "auth" {
    // ...
    after = <<LUA
        rest.exports.auth = json.decode(rest.res.body).token
    LUA
}
request "get status" {
    // ...
    bearer_token = exports.auth
    after = <<LUA
        rest.exports.one_status = json.decode(rest.res.body).status
    LUA
}
request "forward status" {
    // ...
    body = { "status": rest.exports.one_status }
}
```

An example of using more complicated hooks can be found [here](https://github.com/taybart/search)

## Sockets

You can also run a websocket client with a "playbook" of messages either in a REPL or by setting a run order with an inter-message delay.

```sh
$ rest -f api.rest -S # REPL
> ping
< pong
>
$ rest -f api.rest -S run # run in order defined
running playbook ["ping", "subscribe", "post", "noop"]
...
$ rest -f api.rest -S ping # run one off playbook message
< pong
```

```hcl
locals {
    channel = "#general"
    msg = <<JSON
    {"msg": "{{$0}}"}
    JSON
    post = <<JSON
    {
        "msg": "post",
        "channel": "#general",
        "content": "{{.content}}"
    }
    JSON
}
socket {
  no_special_cmds = true # don't reserve "quit" and "exit" commands
  url = "ws://localhost:8080"
  run = {
    delay = "100ms"
    order = ["ping", "subscribe", "post", "noop"] # noop can be used to wait for an answer
  }
  playbook = {
    ping = btmpl(locals.msg, ["ping"]) // {"msg": "ping"}
    subscribe = {msg: "sub", channel: locals.channel }
    post = tmpl(locals.post, { content = "hello everyone!" }) // { "msg": "post", "channel": "#general", "content": "hello everyone!" }
    }
  }
}
```

### Raw text frames

Playbook entries are JSON encoded by default, so a plain string arrives on the wire
with its quotes (`hi = "hello"` sends `"hello"`). Protocols with a plaintext
handshake or line protocol need the bytes as written, wrap those in `raw()`:

```hcl
locals {
  token = env("TOKEN")
}
socket {
  url = "wss://host/ws"
  run = {
    delay = "2s"
    order = ["connect", "noop"]
  }
  playbook = {
    connect = raw("CONNECT ${locals.token}")        # sends: CONNECT eyJhbGciOi...
    sub     = { msg = "sub", channel = "#general" } # sends: {"channel":"#general","msg":"sub"}
  }
}
```

Interpolation and the template helpers still resolve first, `raw()` only changes how
the result is encoded. This works the same for `-S run`, `-S <entry>` and the REPL.

## Export to a different language

Rest files can be exported to a different language using the cli, either a single block or the whole file (as a "client").

```sh
$ rest -f api.rest -e ls # list supported languages
curl
go
js
postman
$ rest -f api.rest -e curl
curl -X ...
```

## CLI Block

The `cli` block turns a rest file into an interactive command-line tool. When a file contains a `cli` block and is run without `-b`, `-l`, `-e`, `-S`, or `--list`, rest will start a Lua REPL instead of executing requests directly.

```hcl
cli {
  # required without fn: lua code to run in a loop until it breaks or errors (more below)
  loop = <<LUA
    print("Hello from the CLI!")
    break
  LUA

  # required without loop: runs instead of loop (call rest.loop() from fn to enter the loop)
  fn = <<LUA
    if cli.loud then -- loud is a boolean defined below
        print(("Hello %s from the cli!"):format(cli.name or "world")) -- name is a flag defined below
    end
  LUA

  # optional: define custom flags that can be passed on the command line
  flags = {
    name = {
      desc = "A name to greet"
      short = "n"
      required = true
    }
    loud = {
      desc = "Enable loud mode"
      short = "l"
      bool = true
    }
    dir = {
      desc = "The directory to work in"
      short = "d"
      default = env("PWD")
    }
  }
}
```

### CLI Flags

Flags defined in the `cli` block can be passed on the command line just like built-in flags (using `github.com/taybart/args`). Passing `-h` or `--help` prints the description of all declared flags in the `cli` block.

Flags are exposed to the Lua loop/fn as fields on the global `cli` table, typed by kind: string flags are Lua strings, bool flags are real Lua booleans. Keys are the flags' literal HCL names, so hyphenated flags must be indexed as strings in Lua: a `list-dir` flag is `cli["list-dir"]` (writing `cli.list-dir` would parse as subtraction).

Each flag supports:
- `desc` - Help description shown on `-h` / `--help`
- `short` - Short flag alias (e.g. `short = "d"` for `-d`)
- `bool` - Boolean flag (`true` / `false`)
- `default` - Default string value when not passed
- `required` - Require the flag to be set (`true` / `false`)

```sh
# pass a string flag
rest -f mycli.rest --name Alice

# pass a bool flag
rest -f mycli.rest --loud

# print help for the cli block's flags
rest -f mycli.rest -h
```

Inside the Lua loop:

```lua
print("Hello, " .. (cli.name or "world") .. "!")
if cli.loud then
  print("LOUD MODE")
end

-- Remaining positional arguments are available on cli.rest (1-indexed table)
-- and can be joined into a string with cli.rest:string() (defaults to " " separator)
local query = cli.rest:string() -- or cli.rest:string(",") or tostring(cli.rest)
```

**Disabling built-in rest flags (`no_flags`):** By default, flag names and short flags must not conflict with any built-in rest flags (e.g., `file`/`f`, `block`/`b`, `label`/`l`, `dir`/`d`, `serve`/`s`, etc.). To disable rest's built-in CLI flags for a file so your `cli` block can freely use any flag names or short flags, set `no_flags = true` in the `config` block (pass `--flags` on the command line to override `no_flags` and use standard `rest` flags like `-l` or `-b`):

```hcl
config {
  no_flags = true
}
```

### Flag Defaults

A string flag can declare a `default`, which is used when the flag is not passed on the command line:

```hcl
flags = {
  dir = {
    desc = "The directory to operate on"
    default = env("PWD")
  }
}
```

- The default is evaluated like the rest of the file, so `env()` and interpolation (e.g. `"${locals.dir}"`) work.
- A defaulted flag is **always** present on the Lua `cli` table. Without a default, an unset flag is `nil`, so `if cli.dir then` distinguishes "user passed it" from "fell back" — a defaulted flag can't tell you that.
- Defaults are Lua-side only: they don't reach the HCL context. To use a flag value in a request with an HCL-side fallback, export it (`rest.exports`) and read it with `try_exports` — see [Exports](#exports).
- Bool flags ignore `default` and are exposed as real Lua booleans: `--loud` → `true`, `--loud=false` → `false`, not passed → `nil`.

### CLI Loop

The `loop` field contains Lua code that runs in a `while true do ... end` loop. For each iteration, `rest` reads a line into the `input` variable (stopping on EOF) and provides a `::continue::` label at the end of the loop.

When both `fn` and `loop` are defined, `fn` runs first as an override. From inside `fn`, you can run one-off setup or flag checks and call `rest.loop()` to enter the defined `loop`.

```hcl
cli {
  fn = <<LUA
    history = {}
    rest.loop()
  LUA
  loop = <<LUA
    if input == "quit" then
      break -- quit the loop
    end
    if input == "skip" then
      goto continue -- jump past the rest of the loop and restart
    end
    table.insert(history, input)
    print("You said: " .. input)
  LUA
}
```

#### Custom Prompt

You can customize the REPL prompt by setting the global `PROMPT` variable (for example in `fn` before calling `rest.loop()`, or dynamically inside `loop`). This is useful for showing state like the current mode or context.

```hcl
cli {
  fn = <<LUA
    PROMPT = "myapp> "
    rest.loop()
  LUA
  loop = <<LUA
    -- input is automatically read using the custom prompt
    print("got: " .. input)
  LUA
}
```

### Full Example

```hcl
cli {
  flags = {
    id = {
      desc = "User ID"
      bool = false
    }
  }
  fn = <<LUA
    conversation = {}
    rest.loop()
  LUA
  loop = <<LUA
    if input == 'echo' then
      inspect.print(conversation)
      goto continue
    end
    table.insert(conversation, { role = 'user', content = input })
    rest.exports.conversation = json.encode(conversation)
    rest.label('completions')
    table.insert(conversation, { role = 'assistant', content = rest.exports.response })
    print(rest.exports.response)
  LUA
}

request "completions" {
  url = "https://api.example.com/v1/chat/completions"
  method = "POST"
  headers = { "Content-Type" = "application/json" }
  body = {
    model = "gpt-4"
    messages: json_dec(try_exports("conversation", env("conversation")))
  }
}
```
