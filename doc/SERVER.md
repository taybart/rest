# Rest Server

## CLI

```sh
    --addr, -a:
	Address to listen on
    --serve, -s:
	Run a server
    --dir, -d:
	Directory to serve
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
    # if you need a more complicated test server you can add specific handlers
    handler "GET" "/path" {
        # either use lua to create a more complex response
        fn = "similar concept to the post_hook in the client files (see hander fns below)"
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

See [examples/server](./examples/server) for a more detailed examples
