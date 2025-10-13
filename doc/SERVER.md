# Rest Server

cli flags:

```sh
    --addr, -a:
	Address to listen on
    --serve, -s:
	Run a server
    --dir, -d:
	Directory to serve
    --file, -f:
	File to run
    --origins, -o:
	Add Access-Control-Allow-Origin header value
	ex: -o * or -o 'http://localhost:8080 http://localhost:3000'
    --tls, -t:
	TLS path name to be used for tls key/cert (defaults to no TLS)
	ex: '-t ./keys/site.com' where the files ./keys/site.com.{key,crt} exist
    --quiet, -q:
	Don't log server requests
```

```hcl
server {
  address = "localhost:18080"
  # TLS path name to be used for tls key/cert (defaults to no TLS)
  # ex: './keys/site.com' where the files ./keys/site.com.{key,crt} exist
  tls = "test/keys/example.com"
  # don't dump requests
  quiet = true
  # add Access-Control-Allow-Origin header values
  origins = ["*"]
  # serve a directory, this will override the default response
  directory = "./test"
  # not required, override the default response if needed
  response {
    status = 200
    # add headers to response
    headers = {
      "x-custom-header": "custom-header",
      "x-env-header": env("HEADER_VALUE")
    }
    body = {
      "custom": "response"
    }
  }
}
```
