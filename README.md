# Rest

Goes well with [rest.nvim](https://github.com/taybart/rest.nvim)

Example (see [doc](doc/CLIENT.md) for more info):

```hcl
locals {
  url = "http://localhost:8080"
  name = "world"
}

// test
request "hell_yeah" {
  url = "${locals.url}/get"
  method = "GET"
  headers = [
    "X-TEST: you:😄",
  ]
}

request "httpbin post" {
  method = "POST"
  url = "https://httpbin.org/post"
  headers = { "Content-Type" = "application/json" }

  body = {
    hello: "${locals.name}"
  }
  # has lua interpreter to post process check docs for more
  post_hook = <<LUA
      local body = json.decode(rest.res.body)
      local ret = json.decode(body.data) -- what_we_sent_to_httpbin
      print(inspect(ret)) -- { hello = "world" }
      return ret.hello -- cli prints -> world
  LUA
}
```

Server/Client:

<img width="721" alt="image" src="https://user-images.githubusercontent.com/3513897/231360482-d54f6e43-b1e9-45ba-883c-7e1d044da2df.png">

Testing:

<img width="716" alt="image" src="https://user-images.githubusercontent.com/3513897/231361047-0a539866-e289-4905-b089-b93753e50e89.png">

```sh
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
    --origins, -o:
	Add Access-Control-Allow-Origin header value
	ex: -o * or -o 'http://localhost:8080 http://localhost:3000'
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

### TODO

- [ ] namespace imports => import "name.rest" -> copy_from "name:basic"
