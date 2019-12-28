# rest

Goes well with [rest.vim](https://github.com/taybart/rest.vim)

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
# <C-d>
http://localhost:8080/
HTTP/1.1 200 OK
Content-Length: 16
Content-Type: text/plain; charset=utf-8
Date: Fri, 06 Dec 2019 18:55:01 GMT

{"status":"ok"}

---
```

## Programatic parsing

```go
import (
  "fmt"

  "github.com/taybart/rest"
)

func main {
  r := rest.New()
  err := r.Read("./test/post.rest")
  if err != nil {
    panic("HOLY SHIT!")
  }
  success, failed := r.Exec() // Execute dem requests
  fmt.Println(success, failed)
}
```
### System tests!

```http
set HOST real.url.net
set ID 123

https://${HOST}
Content-Type: application/json
POST /user
{ "id": ${ID}, "name": "taybart" }

expect 200

---

delay 3s

https://${HOST}
Content-Type: application/json
GET /user?id=${ID}

expect 200 { "id": ${ID}, "name": "taybart" }

```

```
~ » rest -i
https://httpbin.org
GET /status/401
expect 200
Failed requests
Incorrect status code returned 200 != 401
body:
```

### Create other language requests!

```go
import (
  "fmt"

  "github.com/taybart/rest"
)

func main() {
  r := rest.New()
  err := r.Read("./test/post.rest")
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

### Create full clients


```bash
$ rest -f ./test/client.rest -c -o go > client.go
```
```go
// client.go
package main
import (
  "fmt"
  "io/ioutil"
  "net/http"
  "strings"
)
// Client : my client
type Client struct { }

func (c Client) GetThing() {
  req, err := http.NewRequest("GET", "http://localhost:8080/", nil)

  res, err := http.DefaultClient.Do(req)
  if err != nil {
    fmt.Println(err)
  }
  defer res.Body.Close()
    body, err := ioutil.ReadAll(res.Body)
    if err != nil {
      fmt.Println(err)
    }
  fmt.Println(string(body))
}


func (c Client) PostThing() {
  req, err := http.NewRequest("POST", "http://localhost:8080/user", strings.NewReader(`{  "user": "taybart"}`))

  res, err := http.DefaultClient.Do(req)
  if err != nil {
    fmt.Println(err)
  }
  defer res.Body.Close()
  body, err := ioutil.ReadAll(res.Body)
  if err != nil {
    fmt.Println(err)
  }
  fmt.Println(string(body))
}

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

**But ports!!!**

Chill...

```bash
$ rest -s -p 1234
2267-11-25 18:38:02 [INFO] Running at localhost:1234
```
