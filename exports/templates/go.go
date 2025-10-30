package templates

import (
	_ "embed"
)

//go:embed go/client.tmpl
var goClient string

//go:embed go/request.tmpl
var goRequest string

// Go : template
var Go = RequestTemplate{
	Name:       "go",
	ClientStr:  goClient,
	RequestStr: goRequest,
}
