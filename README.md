# rest

Build rest requests for testing. The format is similar to vim-rest-console:

```http
http://localhost:8080
Content-Type: application/json
POST /post-ep
{
  "data": "Yeah!"
}
```

## Programatic parsing

rest file `post_test.rest`
```http
http://localhost:8080
Content-Type: application/json
POST /post-test
{
  "data": "Yeah!"
}
```

```go
import ("github.com/taybart/rest")
func main {
	r := rest.New()
	err := r.Read("./post_test.rest")
	if err != nil {
    panic("HOLY SHIT!")
  }
  r.Exec() // Execute dem requests
}
```


# Create other language requests!

```go
import ("github.com/taybart/rest")

func main() {
	r := rest.New()
	err := r.Read("./post_test.rest")
	if err != nil {
    panic("HOLY SHIT!")
  }
	requests, err := r.SynthisizeRequest("javascript")
	if err != nil {
    panic("HOLY SHIT!")
  }
	js, err := ioutil.ReadFile("./test/template_request.js")
	if err != nil {
    panic("HOLY SHIT!")
  }
  fmt.Println(js) 
  
}
```

### Output

```javascript
fetch('http://localhost:8080/post-test', {
  method: 'POST',
  headers: {
    "Content-Type": "application/json",
  },
  body: {  "data": "Yeah!"}
}).then((res) => {
  if (res.status == 200) {
    // woohoo!
  }
})
```
