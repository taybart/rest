package rest

import (
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/taybart/log"
)

const (
	jsonSep = ",\n"
)

// Holy grail
func SynthisizeClient() {
}

// SynthisizeRequest : output request code
func (r Rest) SynthisizeRequest(lang string) ([]string, error) {
	name := fmt.Sprintf("./templates/%s", lang)
	template, err := ioutil.ReadFile(name)
	if err != nil {
		return []string{}, fmt.Errorf("asdf %w", err)
	}
	requests := make([]string, len(r.requests))
	for i, req := range r.requests {
		builder := strings.ReplaceAll(string(template), "_METHOD", req.Method)
		builder = strings.ReplaceAll(builder, "_URL", req.URL.String())

		headers := ""
		for h, v := range req.Header {
			headers += fmt.Sprintf(`"%s": "%s"%s`, h, v[0], jsonSep) // TODO file based seps
		}
		builder = strings.ReplaceAll(builder, "_HEADERS", headers)

		body, err := ioutil.ReadAll(req.Body)
		if err != nil {
			log.Error(err)
		}
		builder = strings.ReplaceAll(builder, "_BODY", string(body))
		requests[i] = builder
	}
	return requests, nil
}
