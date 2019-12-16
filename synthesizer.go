package rest

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"text/template"

	"github.com/taybart/log"
	"github.com/taybart/rest/templates"
)

// SynthisizeClient : Holy grail
func SynthisizeClient() {
}

// SynthisizeRequests : output request code
func (r Rest) SynthisizeRequests(lang string) ([]string, error) {
	if templ, ok := getTemplate(lang); templ != nil && ok {
		requests := make([]string, len(r.requests))
		for i, req := range r.requests {
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

			var buf bytes.Buffer
			err = templ.Execute(&buf, templReq)
			if err != nil {
				log.Error(err)
			}
			requests[i] = buf.String()
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
		templ = templates.Go.String
	case "curl":
		templ = templates.Curl.String
	case "javascript", "js":
		templ = templates.Javascript.String
	default:
		exists = false
		return
	}
	t = template.Must(template.New(name).Parse(templ))
	return
}
