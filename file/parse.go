package file

import (
	"fmt"
	"os"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/taybart/log"
	"github.com/taybart/rest/request"
)

type Root struct {
	// Imports []*struct {
	// 	Body hcl.Body `hcl:",remain"`
	// } `hcl:"imports,block"`

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

func read(filename string) (*hcl.File, hcl.Diagnostics) {
	src, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, hcl.Diagnostics{
				{
					Severity: hcl.DiagError,
					Summary:  "Configuration file not found",
					Detail:   fmt.Sprintf("The configuration file %s does not exist.", filename),
				},
			}
		}
		return nil, hcl.Diagnostics{
			{
				Severity: hcl.DiagError,
				Summary:  "Failed to read configuration",
				Detail:   fmt.Sprintf("Can't read %s: %s.", filename, err),
			},
		}
	}
	return hclsyntax.ParseConfig(src, filename, hcl.Pos{Line: 1, Column: 1})

}

func Parse(filename string) (request.Config, map[string]request.Request, *request.Socket, error) {
	config := request.DefaultConfig()

	file, diags := read(filename)
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

	// if err := importFiles(root); err != nil {
	// 	return config, nil, nil, err
	// }

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
	var socket *request.Socket // allow nil value for later
	if root.Socket != nil {
		// move to concrete pointer, to ensure decode doesn't panic
		socket = &request.Socket{}
		if diags = gohcl.DecodeBody(root.Socket.Body, ctx, socket); diags.HasErrors() {
			writeDiags(map[string]*hcl.File{filename: file}, diags)
			return config, nil, nil, fmt.Errorf("error decoding HCL configuration: %w", diags)
		}
		if err := socket.ParseExtras(ctx); err != nil {
			return config, nil, nil, err
		}
	}

	requests := map[string]request.Request{}
	labels := []string{}
	for i, block := range root.Requests {
		req := request.Request{Label: block.Label}
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
		req.BlockIndex = i
		requests[req.Label] = req
		labels = append(labels, req.Label)
	}
	// process copy_froms
	for label, req := range requests {
		if requests[label].CopyFrom != "" {
			if _, ok := requests[req.CopyFrom]; !ok {
				return config, nil, nil, fmt.Errorf("request copy_from not found: %s", req.CopyFrom)
			}
			req.CombineFrom(requests[req.CopyFrom])
			requests[label] = req
		}
		// check that required fields are set
		if requests[label].URL == "" {
			return config, nil, nil, fmt.Errorf("url is required for request: %s", req.Label)
		}
	}
	return config, requests, socket, nil
}
