# rest

Goes well with [vim-rest](https://github.com/taybart/rest-vim)

Build rest requests for testing. The format is similar to [vim-rest-console](https://github.com/diepm/vim-rest-console):

<img width="271" alt="image" src="https://user-images.githubusercontent.com/3513897/69470865-ca43a200-0d56-11ea-9ebb-b68ecd7ced57.png">

## Command line execution

```shell
$ go install github.com/taybart/rest/cmd/rest
$ cat <<EOF > post.rest
http://localhost:8080
Content-Type: application/json
POST /
{
  "data": "Yeah!"
}
EOF
$ rest -f post.rest
{ "status": "ok" }

# STDIN
$ rest -i
http://localhost:8080
GET /
# <C-D>
[http://localhost:8080/
HTTP/1.1 200 OK
Content-Length: 16
Content-Type: text/plain; charset=utf-8
Date: Tue, 26 Nov 2019 01:31:49 GMT

{"status":"ok"}
] []
```

## Programatic parsing

```go
import ("github.com/taybart/rest")
func main {
  r := rest.New()
  err := r.Read("./post.rest")
  if err != nil {
    panic("HOLY SHIT!")
  }
  r.Exec() // Execute dem requests
}
```


### Create other language requests!

```go
import (
  "fmt"

  "github.com/taybart/rest"
)

func main() {
  r := rest.New()
  err := r.Read("./post.rest")
  if err != nil {
  fmt.Println(err)
    panic("HOLY SHIT!")
  }
  requests, err := r.SynthisizeRequest("javascript")
  if err != nil {
    fmt.Println(err)
    panic("HOLY SHIT!")
  }
  fmt.Println(requests[0])
}
```
```javascript
// output
fetch('http://localhost:8080/post-test', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json', },
  body: JSON.stringify({  "data": "Yeah!"}),
}).then((res) => { if (res.status == 200) { /* woohoo! */ } })
```
