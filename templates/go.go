package templates

// Go : template
var Go = RequestTemplate{
	String: `req, err := http.NewRequest("{{.Method}}", "{{.URL}}", {{if .Body}}strings.NewReader(` + "`" + `{{.Body}}` + "`" + `){{else}}nil{{end}})
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
