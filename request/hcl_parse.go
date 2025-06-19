package request

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/taybart/log"
)

type Root struct {
	Locals []*struct {
		Body hcl.Body `hcl:",remain"`
	} `hcl:"locals,block"`

	Config *struct {
		Body hcl.Body `hcl:",remain"`
	} `hcl:"config,block"`

	Requests []*struct {
		Label string   `hcl:"label,label"`
		Body  hcl.Body `hcl:",remain"`
	} `hcl:"request,block"`

	Socket *struct {
		Body hcl.Body `hcl:",remain"`
	} `hcl:"socket,block"`
}

func parseFile(filename string) (Config, []Request, *Socket, error) {
	config := DefaultConfig()

	file, diags := readFile(filename)
	if diags.HasErrors() {
		writeDiags(map[string]*hcl.File{filename: file}, diags)
		return config, nil, nil, diags
	}

	var root Root
	diags = gohcl.DecodeBody(file.Body, nil, &root)
	if diags.HasErrors() {
		log.Infoln("decode")
		writeDiags(map[string]*hcl.File{filename: file}, diags)
		return config, nil, nil, diags
	}

	locals, err := decodeLocals(root)
	if err != nil {
		return config, nil, nil, err
	}

	ctx := makeContext(locals)

	if root.Config != nil {
		if diags = gohcl.DecodeBody(root.Config.Body, ctx, &config); diags.HasErrors() {
			writeDiags(map[string]*hcl.File{filename: file}, diags)
			return config, nil, nil, fmt.Errorf("error decoding HCL configuration: %w", diags)
		}
	}
	var socket *Socket // allow nil value for later
	if root.Socket != nil {
		// move to concrete pointer, to ensure decode doesn't panic
		socket = &Socket{}
		if diags = gohcl.DecodeBody(root.Socket.Body, ctx, socket); diags.HasErrors() {
			writeDiags(map[string]*hcl.File{filename: file}, diags)
			return config, nil, nil, fmt.Errorf("error decoding HCL configuration: %w", diags)
		}
		if err := socket.ParseExtras(ctx); err != nil {
			return config, nil, nil, err
		}
	}

	requests := []Request{}
	labels := []string{}
	for _, block := range root.Requests {
		req := Request{Label: block.Label}
		if diags = gohcl.DecodeBody(block.Body, ctx, &req); diags.HasErrors() {
			writeDiags(map[string]*hcl.File{filename: file}, diags)
			return config, nil, nil, fmt.Errorf("error decoding HCL configuration: %w", diags)
		}
		for _, l := range labels {
			if l == req.Label {
				return config, nil, nil, fmt.Errorf("labels must be unique: %s", l)
			}
		}
		if err := req.ParseBody(ctx); err != nil {
			return config, nil, nil, err
		}
		if err := req.SetDefaults(ctx); err != nil {
			return config, nil, nil, err
		}

		requests = append(requests, req)
		labels = append(labels, req.Label)
	}
	return config, requests, socket, nil
}
