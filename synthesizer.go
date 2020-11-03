package rest

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"text/template"

	"github.com/taybart/log"
	"github.com/taybart/rest/lexer"
	"github.com/taybart/rest/templates"
)

// SynthisizeClient : Holy grail
func (r Rest) SynthesizeClient(lang string) (string, error) {
	code, requests, err := r.SynthesizeRequests(lang)
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
			Label:  requests[i].Label,
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
func (r Rest) SynthesizeRequests(lang string) ([]string, []lexer.Request, error) {
	if t := templates.Get(lang); t != nil {
		generated := []string{}
		requests := []lexer.Request{}
		for _, metaReq := range r.lexed {
			req, err := lexer.BuildRequest(metaReq, r.vars)
			if err != nil {
				return nil, nil, err
			}
			if req.Skip {
				continue
			}
			var body []byte
			if req.R.Body != nil {
				var err error
				body, err = ioutil.ReadAll(req.R.Body)
				if err != nil {
					log.Error(err)
				}
			}
			templReq := struct {
				URL     string
				Method  string
				Headers map[string][]string
				Body    string
			}{
				URL:     req.R.URL.String(),
				Method:  req.R.Method,
				Headers: req.R.Header,
				Body:    string(body),
			}

			var buf bytes.Buffer
			err = t.Request.Execute(&buf, templReq)
			if err != nil {
				log.Error(err)
			}
			requests = append(requests, req)
			generated = append(generated, buf.String())
		}
		return generated, requests, nil
	}
	return nil, nil, fmt.Errorf("Unknown template")
}

func buildTemplate(lang, templ string) *template.Template {
	return template.Must(template.New(lang).Parse(templ))
}
