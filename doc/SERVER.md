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

### Examples

```sh
# run server with default config (localhost:8080, 200 on any request, no cors)
rest -s

# serve a directory with cors and tls
rest -s -a 0.0.0.0:18080 -t ./tls/example.com -d ./dist --cors

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
The return must be a table (its the same type as the response block in HCL):
ex:
```lua
return {
    status = 200,
    headers = { ["Set-Cookie"] = "test=1" }
    body = json.encode({msg = "hello world"}),
}
```

**Modules**

There are some global modules available in the lua runtime.

- `json` - encode and decode json
- `colors` - adds terminal color escape codes and formatting functions for extra points
- `inspect` - used to inspect lua values
- `base64` - encode and decode base64
- `tools` - various helper functions, check out the [tools](https://github.com/taybart/rest/blob/main/lua/modules/tools.lua) module for commented functions
    - one call out is `tools.get_req_header`, but you can read the file to see the rest of the fuctions
- `kv` - in-memory key/value store that persists between requests
    - `kv.get(key)` - get value from kv cache, returns nil if key doesn't exist
    - `kv.set(key, value)` - set a key's value, can be anything

See [examples/server](./examples/server) for a more detailed examples
