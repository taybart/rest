package templates

import (
	"bytes"
	"encoding/json"
	"text/template"

	"github.com/taybart/log"
)

var stdFns = template.FuncMap{
	"json": func(headers map[string][]string, body string) string {
		if headers["Content-Type"][0] == "application/json" {
			var b bytes.Buffer
			err := json.Compact(&b, []byte(body))
			if err != nil {
				log.Error(err)
				return body
			}
			return b.String()
		}
		return body
	},
}
