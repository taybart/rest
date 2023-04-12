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
