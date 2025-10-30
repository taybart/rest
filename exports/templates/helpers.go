package templates

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
	"text/template"

	"github.com/taybart/log"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var stdFns = template.FuncMap{
	"split": func(str, sp string, idx int) string {
		return strings.Trim(strings.Split(str, sp)[idx], " ")
	},
	"camelcase": func(s string) string {
		s = regexp.MustCompile("[^a-zA-Z0-9_ ]+").ReplaceAllString(s, "")
		s = strings.ReplaceAll(s, "_", " ")
		s = cases.Title(language.AmericanEnglish, cases.NoLower).String(s)
		s = strings.ReplaceAll(s, " ", "")
		if len(s) > 0 {
			s = strings.ToLower(s[:1]) + s[1:]
		}
		return s
	},
	"json": func(headers map[string]string, body string) string {
		if headers["Content-Type"] == "application/json" {
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
