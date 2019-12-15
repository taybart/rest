package rest

import (
	"fmt"
	"io/ioutil"
	"os"
	"text/template"

	"github.com/taybart/log"
)

// Holy grail
func SynthisizeClient() {
}

// SynthisizeRequest : output request code
func (r Rest) SynthisizeRequest(lang string) ([]string, error) {
	if templ, ok := getTemplate(lang); templ != nil && ok {
		requests := make([]string, len(r.requests))
		for _, req := range r.requests {
			body, err := ioutil.ReadAll(req.r.Body)
			if err != nil {
				log.Error(err)
			}
			templReq := struct {
				URL     string
				Method  string
				Headers map[string][]string
				Body    string
			}{
				URL:     req.r.URL.String(),
				Method:  req.r.Method,
				Headers: req.r.Header,
				Body:    string(body),
			}

			err = templ.Execute(os.Stdout, templReq)
			if err != nil {
				log.Error(err)
			}
		}
		return requests, nil
	}
	return nil, fmt.Errorf("Unknown template")
}

func getTemplate(name string) (t *template.Template, exists bool) {
	exists = true
	var templ string
	switch name {
	case "go":
		templ =
			`req, err := http.NewRequest("{{.Method}}", "{{.URL}}", strings.NewReader(` + "`" + `{{.Body}}` + "`" + `))
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
fmt.Println(string(body))
			`
	case "curl":
		templ =
			`curl -X {{.Method}} {{.URL}} \
{{range $name, $value := .Headers}} -H {{$name}} {{range $internal := $value}}{{$internal}}{{end}} \
{{end}}
-d '{{.Body}}'
			`
	case "javascript", "js":
		templ =
			`fetch('{{.URL}}', {
  method: '{{.Method}}',
  headers: {
{{range $name, $value := .Headers}}    '{{$name}}': '{{range $internal := $value}}{{$internal}}{{end}}',
{{end}}
},
	body: JSON.stringify({{.Body}}),
}).then((res) => { if (res.status == 200) { /* woohoo! */ } })`
	default:
		exists = false
		return
	}
	t = template.Must(template.New(name).Parse(templ))
	return
}
