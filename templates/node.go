package templates

// Node : template
var Node = RequestTemplate{
	ClientStr: `const fetch = require('node-fetch')
	class Client {
	{{- .Code -}}
	}
	`,
	FunctionStr: `{{if not .Client}}function {{end}}{{.Label}}() {
	{{.Code}}
	}`,
	RequestStr: `fetch('{{.URL}}', {
  method: '{{.Method}}',
{{- if .Headers}}
  headers: {
{{range $name, $value := .Headers}}    '{{$name}}': '{{range $internal := $value}}{{$internal}}{{end}}',
{{end}}  },
{{- end}}
{{- if .Body}}
  body: JSON.stringify({{.Body}}),
{{- end}}
})
  .then((res) => res.json().then((data) => ({ status: res.status, data })))
  .then(({ status, data }) => console.log(status, data))`,
}
