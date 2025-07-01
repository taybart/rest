package templates

// Javascript : template
var Javascript = RequestTemplate{
	Name: "js",
	ClientStr: `
	class Client {
	{{- .Code -}}
	}
	`,
	FunctionStr: `function {{camelcase .Label}}() {
	{{.Code}}
	}`,
	RequestStr: `fetch('{{.URL}}', {
  method: '{{.Method}}',
{{- if .Headers}}
  headers: {
	{{range $key, $value := .Headers}}'{{$key}}', '{{$value}}',{{end}}
},{{- end}}
{{- if .Body}}
  body: JSON.stringify({{.Body}}),
{{- end}}
})
  .then((res) => res.json().then((data) => ({ status: res.status, data })))
  .then(({ status, data }) => console.log(status, data))`,
}
