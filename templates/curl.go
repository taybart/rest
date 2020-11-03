package templates

// Curl : template
var Curl = RequestTemplate{
	RequestStr: `curl -X {{.Method}} {{.URL}}{{if .Headers}} \{{else if .Body}} \{{end}}
{{range $name, $value := .Headers}} -H {{$name}} {{range $internal := $value}}{{$internal}}{{end}} \{{end}}
{{if .Body}} -d '{{.Body}}'{{end}}`,
}
