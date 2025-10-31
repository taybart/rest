# Rest Client

Rest uses HCL to define requests and run them.
It also has a lua interpreter to post-process responses.

1. [Cli](#client-cli)
1. [Config/Locals](#configlocals)
1. [Request Blocks](#request-blocks)
1. [Functions](#functions)
1. [Hooks](#hooks)
1. [Export](#export-to-a-different-language)
1. [Sockets](#sockets)

## Client cli

```sh
# run file
rest -f FILE_NAME
# run block 0-based
rest -f FILE_NAME -b BLOCK_NUMBER
# run by label (request "LABEL_NAME" {)
rest -f FILE_NAME -l LABLE_NAME

```

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
    }
    body = { # response must have this body
      "test": "response"
    }
  }

  # is a string, heredoc (<<IDENT ... IDENT) is a good way to set it
  # using LUA as the ident can make some editors highlight the code better
  post_hook = "see hooks below"
}
```

## Functions

There are a few functions that can be used in a rest file:

- `env("VALUE")` - grab an environment variable
- `read("./filepath")` - read a file into the rest file, this will just be read into a string so it can be used anywhere (ex. request body,
- `json("{\"string\": \"json\"}")` - turn string value into a json object, there are some caveats with this function
- `form({key = "value"}")` - turn map value into a url-encoded form string
- `btmpl("{\"string\": \"{{named}}\"}", {named = "world"})` - execute a basic template replacing named or indexed values if second argument is an array
- `tmpl("{{{if .named}}\"string\": \"{{.named}}\"{{end}}}", {named = "world"})` - execute a go template with a map (currently only map[string]strings are supported)

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
    test: json("${locals.partial_body}")
  }
  # or just used directly
  body = json(locals.partial_body)
  # or url encoded form string
  body = form({ hello = "world" })
}
```

## Hooks

Hooks are defined in the `post_hook` field. They are lua functions that are executed after the response is received. They can be used to do fancy lua stuff.

There are a couple of global libraries available to you:

- `json` - encode and decode json
- `colors` - adds terminal color escape codes and formatting functions for extra points
- `inspect` - used to inspect lua values
- `base64` - encode and decode base64
- `tools` - various helper functions, check out the [tools](https://github.com/taybart/rest/blob/main/request/lua/tools.lua) module for commented functions 

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

### exports

You can grab values from responses and put them in the `exports` table. This is available in requests below when the value is set.
It can be used in the HCL of rest files or in post_hooks (`rest.exports` in lua land). One use case is to authenticate in the first request block and use the returned auth token in the next request block.

```hcl
request "auth" {
    // ...
    post_hook = <<LUA
        rest.exports.auth = json.decode(rest.res.body).token
    LUA
}
request "one" {
    // ...
    bearer_token = exports.auth
    post_hook = <<LUA
        rest.exports.one_status = json.decode(rest.res.body).status
    LUA
}
request "two" {
    // ...
    body = { "status": rest.exports.one_status }
}
```

There is also a special `fail` function that can be used to fail the request. It takes a string argument and returns an error back up to the rest cli. This is different than a lua `error` which will be caught by the lua runtime. The main difference is that `fail` will cause the cli to just print the error you want without lua stack info.

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

