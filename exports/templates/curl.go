package templates

import (
	_ "embed"
)

//go:embed curl/request.tmpl
var curlRequest string

// Curl : template
var Curl = RequestTemplate{
	Name:       "curl",
	RequestStr: curlRequest,
}
