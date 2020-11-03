package templates

import (
	"text/template"
)

var stdFns = template.FuncMap{
	"json": func(headers map[string][]string, body string) string {
		/* if headers["Content-Type"][0] == "application/json" {
			fmt.Println(body)
			var b bytes.Buffer
			err := json.Compact(&b, []byte(body))
			fmt.Println(err)
			fmt.Println(b.String())
			return b.String()
		} */
		return body
	},
}
