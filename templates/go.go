package templates

// Go : template
var Go = RequestTemplate{
	ClientStr: `package main
import (
  "fmt"
  "io/ioutil"
  "net/http"
  "strings"
)
// Client : my client
type Client struct { }
{{.Code}}
	`,
	FunctionStr: `func (c Client) {{.Label}}() {
{{.Code}}
}
		`,
	RequestStr: `req, err := http.NewRequest("{{.Method}}", "{{.URL}}", {{if .Body}}strings.NewReader(` + "`" + `{{json .Headers .Body}}` + "`" + `){{else}}nil{{end}})
{{range $name, $value := .Headers}}req.Header.Set("{{$name}}", "{{range $internal := $value}}{{$internal}}{{end}}")
{{end}}
res, err := http.DefaultClient.Do(req)
if err != nil {
  fmt.Println(err)
}
defer res.Body.Close()
body, err := ioutil.ReadAll(res.Body)
if err != nil {
  fmt.Println(err)
}
fmt.Println(string(body))`,
}
