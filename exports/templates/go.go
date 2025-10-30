package templates

// Go : template
var Go = RequestTemplate{
	Name: "go",
	ClientStr: `package main
import (
  "fmt"
  "io"
  "net/http"
	"net/url"
  "strings"
)
// Client : my client
type Client struct { }
{{.Code}}
	`,
	FunctionStr: `
	{{.Code}}`,
	RequestStr: `
	func (c Client) {{camelcase .Label}}() (*http.Response, error) {
	req, err := http.NewRequest("{{.Method}}", "{{.URL}}", {{if .Body}}
		strings.NewReader(` + "`" + `{{json .Headers .Body}}` + "`" + `){{else}}nil{{end}})
	if err != nil {
		return nil, err
	}
	{{range $key, $value := .Headers}}req.Header.Set("{{$key}}", "{{$value}}"){{end}}
	{{ if .Cookies }}
	{{range $key, $value := .Cookies}}
	req.AddCookie(&http.Cookie{
		Name:  "{{$key}}",
		Value: "{{$value}}",
	}){{end}} {{end}}
	{{ if .Query }}
	query := url.Values{}
	{{range $key, $value := .Query}} query.Add("{{$key}}", "{{$value}}") 
	{{end}} req.URL.RawQuery = query.Encode()
	{{end}}
res, err := http.DefaultClient.Do(req)
if err != nil {
	return nil, err
}
defer res.Body.Close()
body, err := io.ReadAll(res.Body)
if err != nil {
	return nil, err
}
fmt.Println(string(body))
{{ if .Expect }}
if res.StatusCode != {{.Expect}} {
	return nil, fmt.Errorf("status code %d != %d", res.StatusCode, {{.Expect}})
}
{{ end }}
return res, nil
}
`,
}
