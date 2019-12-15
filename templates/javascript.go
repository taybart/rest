package templates

// Javascript : template
var Javascript = RequestTemplate{
	String: `fetch('{{.URL}}', {
  method: '{{.Method}}',
  headers: {
{{range $name, $value := .Headers}}    '{{$name}}': '{{range $internal := $value}}{{$internal}}{{end}}',
{{end}}  },
  body: JSON.stringify({{.Body}}),
})
  .then((res) => res.json().then((data) => ({ status: res.status, data })))
  .then(({ status, data }) => console.log(status, data))`,
}
