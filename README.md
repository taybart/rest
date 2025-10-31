# Rest

In need of a quick rest client or server? Easily done with rest using HCL configs. 

Goes well with [rest.nvim](https://github.com/taybart/rest.nvim)

Example (see [doc](doc) for more info):

**Client**

```hcl
locals {
  url = "http://localhost:8080"
  name = "world"
}

# basic GET request
request "hello rest" {
  url = "${locals.url}/get"
}

request "httpbin post" {
  url = "https://httpbin.org/post"
  method = "POST"
  headers = { "Content-Type" = "application/json" }
  body = {
    hello: "${locals.name}"
  }
  # has lua interpreter to post process check docs/CLIENT.md for more
  post_hook = <<LUA
      local body = json.decode(rest.res.body)
      local ret = json.decode(body.data) -- what_we_sent_to_httpbin
      print(inspect(ret)) -- { hello = "world" }
      return ret.hello -- cli prints -> world
  LUA
}
```

**Server**
```hcl
server {
	address = "localhost:18080"
	handler "GET" "/hello" { response { status = 226 } }
	handler "POST" "/upload/{id}" {
		fn = <<LUA
			print(s.path_value("id")) -- get path values
			print(rest.req.body)
			return {
				status = 200
				body = {
					msg = "success"
				}
			}
		LUA
	}
	
	response { status = 418	}
}
```

```
rest -h
		=== Rest Easy ===
CLI:
    --no-color, -nc:
	No colors
Server:
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
Client:
    --file, -f:
	File to run
    --block, -b:
	Request block to run, 0-indexed
    --label, -l:
	Request label to run
    --socket, -S:
	Run the socket block (ignores requests)
	if set like "--socket/-S run", rest will run socket.run.order and exit
    --export, -e:
	Export file to specified language
    --client, -c:
	Export full client instead of individual requests
    --verbose, -v:
	More client logging
```

