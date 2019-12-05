# rest

Goes well with [vim-rest](https://github.com/taybart/rest-vim)

Build rest requests for testing. The format is similar to [vim-rest-console](https://github.com/diepm/vim-rest-console):

[Example file](https://raw.githubusercontent.com/taybart/rest/master/example.rest)

<img width="337" alt="image" src="https://user-images.githubusercontent.com/3513897/69688272-cda29a80-1082-11ea-92e5-d1139fb47fde.png">

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
import (
  "fmt"
  
  "github.com/taybart/rest"
)

func main {
  r := rest.New()
  err := r.Read("./post.rest")
  if err != nil {
    panic("HOLY SHIT!")
  }
  success, failed := r.Exec() // Execute dem requests
  fmt.Println(success, failed)
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

## Server?

I have had uses for having a server that just accepts requests...So I put it in here:

```bash
$ rest -s
2267-11-25 18:38:02 [INFO] Running at localhost:8080
```

I have also (like many) have used `python -m http.server`. Mine is not as cool but it does stuff:

```bash
$ rest -d
2019-11-25 18:40:01 [INFO] Serving $(pwd) at localhost:8080
```

## But ports!!! 

Chill...

```bash
$ rest -s -p 1234
2267-11-25 18:38:02 [INFO] Running at localhost:1234
```
