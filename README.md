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
  after = <<LUA
      local body = json.decode(rest.res.body)
      local ret = json.decode(body.data) -- what_we_sent_to_httpbin
      print(inspect(ret)) -- { hello = "world" }
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

**Create CLI tools**

```hcl
#!/usr/bin/env rest

# disable regular rest cli flags
config { no_flags = true }

locals {
  url = "http://localhost:8080"
  name = "world"
}

cli {
    flags = {
        name = { 
            desc = "Name to say hello to"
            short = "n" # flag can be --name or -n, short is optional
            # set default value (using local here can be anything)
            default = locals.name
        }
    }
    fn = <<LUA
        if not cli.name then
            error("name is required")
        end
        -- export the name to use in the request
        rest.exports.name = cli.name
        -- run the request
        rest.label('say hello')
    LUA
}

# basic GET request
request "hello rest" {
  url = "${locals.url}/get"
    # try to use rest.exports.name or default to "world"
  query = { name = try_exports("name", "world") }
}
```

make the file executable and run it

```sh
chmod +x hello.rest
./hello.rest --name "rest"
# GET http://localhost:8080/get?name=rest
```

there are other options like a "loop" see [docs/CLIENT.md](doc/CLIENT.md) for more

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
    --tailscale:
	Serve https using this machine's tailscale cert (needs the tailscale cli and HTTPS enabled on the tailnet)
	listens on :8080 unless --addr is set so other tailnet devices can connect
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

