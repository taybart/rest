package templates

// Curl : template
var Curl = RequestTemplate{
	String: `curl -X {{.Method}} {{.URL}} \
{{range $name, $value := .Headers}} -H {{$name}} {{range $internal := $value}}{{$internal}}{{end}} \
{{end}}
-d '{{.Body}}'`,
}
