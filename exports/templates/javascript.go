package templates

import (
	_ "embed"
)

//go:embed javascript/client.tmpl
var jsClient string

//go:embed javascript/function.tmpl
var jsFunction string

//go:embed javascript/request.tmpl
var jsRequest string

// Javascript : template
var Javascript = RequestTemplate{
	Name:        "js",
	ClientStr:   jsClient,
	FunctionStr: jsFunction,
	RequestStr:  jsRequest,
}
