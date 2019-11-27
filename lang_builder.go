package rest

import (
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/taybart/log"
)

const (
	jsonSep = ","
)

// Holy grail
func SynthisizeClient() {
}

// SynthisizeRequest : output request code
func (r Rest) SynthisizeRequest(lang string) ([]string, error) {
	if template, ok := templates[lang]; ok {

		/* name := fmt.Sprintf("./templates/%s", lang)
		template, err := ioutil.ReadFile(name)
		if err != nil {
			return []string{}, fmt.Errorf("asdf %w", err)
		} */
		requests := make([]string, len(r.requests))
		for i, req := range r.requests {
			builder := strings.Replace(string(template), "_METHOD", req.r.Method, 1)
			builder = strings.Replace(builder, "_URL", req.r.URL.String(), 1)

			headers := ""
			for h, v := range req.r.Header {
				headers += fmt.Sprintf(`'%s': '%s'%s`, h, v[0], jsonSep) // TODO file based seps
			}
			builder = strings.Replace(builder, "_HEADERS", headers, 1)

			body, err := ioutil.ReadAll(req.r.Body)
			if err != nil {
				log.Error(err)
			}
			builder = strings.Replace(builder, "_BODY", string(body), 1)
			requests[i] = builder
		}
		return requests, nil
	}
	return nil, fmt.Errorf("Unknown template")
}

var templates = map[string]string{
	"javascript": `fetch('_URL', {
  method: '_METHOD',
  headers: { _HEADERS },
  body: JSON.stringify(_BODY),
}).then((res) => { if (res.status == 200) { /* woohoo! */ } })`,
}
