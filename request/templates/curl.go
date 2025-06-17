package templates

// Curl : template
var Curl = RequestTemplate{
	Name: "curl",
	RequestStr: `curl{{if .Method}} -X {{.Method}}{{end}}{{range .Headers}} \
  --header '{{.}}'{{end}}{{if .UserAgent}} \
  --header 'User-Agent: {{.UserAgent}}'{{end}}{{range $key, $value := .Cookies}} \
  --cookie '{{$key}}={{$value}}'{{end}}{{if .Body}} \
  --data-raw '{{.Body}}'{{end}} \
  '{{.URLWithQuery}}'`,
}
