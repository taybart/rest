package request

import "github.com/hashicorp/hcl/v2"

type Root struct {
	Locals []*struct {
		Body hcl.Body `hcl:",remain"`
	} `hcl:"locals,block"`

	Requests []*struct {
		Body hcl.Body `hcl:",remain"`
	} `hcl:"request,block"`
}
