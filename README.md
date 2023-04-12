# Rest

Goes well with [rest.nvim](https://github.com/taybart/rest.nvim)

Example:
```hcl
locals {
  url = "http://localhost:8080"
  asdf = "world"
}

// test
request {
  label = "hell_yeah"
  method = "GET"
  headers = [
    "X-TEST: you:😄",
  ]
  url = "${locals.url}/get"
}

request {
  method = "POST"
  url = "${locals.url}/post"
  headers = [
    "Content-Type: application/json",
  ]

  body = <<END
  {
    "hello": "${locals.asdf}"
  }
  END
  expect = 200
}
```

Server/Client:

<img width="721" alt="image" src="https://user-images.githubusercontent.com/3513897/231360482-d54f6e43-b1e9-45ba-883c-7e1d044da2df.png">


```sh
rest -h
    --no-color, -nc:
        No colors
    --quiet, -q:
        Minimize logging
    --addr, -a:
        Address to listen on
    --serve, -s:
        Run a server
    --dir, -d:
        Directory to serve
    --file, -f:
        File to run
    --block, -b:
        Request block to run
    --label, -l:
        Request label to run
```

