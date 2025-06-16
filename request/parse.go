package request

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/taybart/log"
)

func parseFile(filename string) (Config, []Request, error) {
	config := DefaultConfig()

	file, diags := readFile(filename)
	if diags.HasErrors() {
		log.Infoln("parse")
		writeDiags(map[string]*hcl.File{filename: file}, diags)
		return config, nil, diags
	}

	var root Root
	diags = gohcl.DecodeBody(file.Body, nil, &root)
	if diags.HasErrors() {
		log.Infoln("decode")
		writeDiags(map[string]*hcl.File{filename: file}, diags)
		return config, nil, diags
	}

	locals, err := decodeLocals(root)
	if err != nil {
		return config, nil, err
	}

	ctx := createContext(locals)

	if root.Config != nil {
		if diags = gohcl.DecodeBody(root.Config.Body, ctx, &config); diags.HasErrors() {
			writeDiags(map[string]*hcl.File{filename: file}, diags)
			return config, nil, fmt.Errorf("error decoding HCL configuration: %w", diags)
		}
	}

	requests := []Request{}
	labels := []string{}
	for _, block := range root.Requests {
		req := Request{Label: block.Label}
		if diags = gohcl.DecodeBody(block.Body, ctx, &req); diags.HasErrors() {
			writeDiags(map[string]*hcl.File{filename: file}, diags)
			return config, nil, fmt.Errorf("error decoding HCL configuration: %w", diags)
		}
		for _, l := range labels {
			if l == req.Label {
				return config, nil, fmt.Errorf("labels must be unique: %s", l)
			}
		}
		if err := req.ParseBody(ctx); err != nil {
			return config, nil, err
		}
		if err := req.SetDefaults(ctx); err != nil {
			return config, nil, err
		}

		requests = append(requests, req)
		labels = append(labels, req.Label)
	}
	return config, requests, nil
}
