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
func (r Rest) SynthisizeClient(lang string) (string, error) {
	code, err := r.SynthisizeRequests(lang)
	if err != nil {
		return "", err
	}
	templ := templates.Get(lang)
	var client string
	for i, c := range code {
		templReq := struct {
			Label  string
			Code   string
			Client bool
		}{
			Label:  r.requests[i].label,
			Code:   c,
			Client: true,
		}
		var buf bytes.Buffer
		err = templ.Function.Execute(&buf, templReq)
		if err != nil {
			log.Error(err)
		}
		client = fmt.Sprintf("%s\n%s\n", client, buf.String())
	}
	var buf bytes.Buffer
	err = templ.Client.Execute(&buf, struct{ Code string }{Code: client})
	if err != nil {
		log.Error(err)
	}
	client = buf.String()
	return client, nil
}

// SynthisizeRequests : output request code
func (r Rest) SynthisizeRequests(lang string) ([]string, error) {
	if t := templates.Get(lang); t != nil {
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
			err = t.Request.Execute(&buf, templReq)
			if err != nil {
				log.Error(err)
			}
			requests[i] = buf.String()
		}
		return requests, nil
	}
	return nil, fmt.Errorf("Unknown template")
}

func buildTemplate(lang, templ string) *template.Template {
	return template.Must(template.New(lang).Parse(templ))
}
