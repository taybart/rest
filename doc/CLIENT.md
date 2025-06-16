# Rest Client

Rest uses HCL to define requests and run them.
It also has a lua interpreter to post-process responses.

1. [Cli](#client-cli)
1. [Config/Locals](#configlocals)
1. [Request Blocks](#request-blocks)
1. [Functions](#functions)
1. [Hooks](#hooks)
1. [Export](#export-to-a-different-language)

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
  # url must include protocol (http(s) for now)
  url = "http://localhost:8080/"

  # if not specified, defaults to GET
  method = "GET"

  # array of strings, they are directly included so ensure they are formatted correctly
  headers = [ "X-Test: test" ]

  # only single level query params are allowed so maps must be explicitly defined
  query = {
    b = "2"
    "config[key]" = "value"
  }

  # body can look like a json object or a regular hcl map
  body = { test: "body" } # or body = { test = "body" }

  # cookies can be set for a single request
  cookies = { a = "1" }

  # if a hook is not defined response code will be checked agains this value
  # and fail if it doesn't match (ie return 1)
  expect = 200

  # is a string, heredoc (<<IDENT ... IDENT) is a good way to set it,
  # if you are using treesitter, using LUA as your ident will highlight
  #the hook well
  post_hook = "see hooks below"
}
```

## Functions

There are a few functions that can be used in a rest file:

- `env("VALUE")` - grab an environment variable
- `read("./filepath")` - read a file into the rest file, this will just be read into a string so it can be used anywhere (ex. request body,

For example:

```hcl
locals {
    parial_body = read("./body.json")
}

request "my request" {
  url = "http://localhost:8080/"
  headers = [
    "Authorization: Bearer ${env("token")}"
  ]
  # will be {"test": {"hello": "from a file"}}
  body = {
    test: "${locals.partial_body}"
  }
}
```

## Hooks

Hooks are defined in the `post_hook` block. They are lua functions that are executed after the response is received. They can be used to do fancy lua stuff.

There are a couple of global libraries available to you:

- `json` - encode and decode json
- `inspect` - used to inspect lua values
- `base64` - encode and decode base64

Hooks are also passed a `rest` table that contains the following:

`req` - the request object

```lua
{
  body = "" -- string representation of the request body (can be parsed with json.decode())
  headers = {}, -- headers table of the request
  cookies = {}, -- cookies table that was sent during the request
  method = "GET", -- method used for the request
  query = {}, -- query table of the request
  url = "https://httpbin.org/get" -- url used for the request
}
```

`res` - the response object

```lua
{
  body = "" -- string representation of the response body (can also be parsed with json module)
  cookies = {}, -- cookies that were set during the request
  headers = {}, -- headers table returned by the server
  status = 200 -- status code returned by the server
}
```

It is possible to return a string from a hook, this will be returned to the client (printed out for now)

There is also a special `fail` function that can be used to fail the request. It takes a string argument and returns an error. This is different than a lua `error` and will just return the error to the client.

## Export to a different language

`todo!()`
