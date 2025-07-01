// Package file provides a parser for the hcl configuration file format.
package file

import (
	"fmt"
	"os"
	"path"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/taybart/log"
	"github.com/taybart/rest/request"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

type Rest struct {
	Config   request.Config
	Socket   request.Socket
	Requests map[string]request.Request
}

type Root struct {
	Imports *[]string `hcl:"imports"`

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

func (r *Root) Add(root *Root) {
	r.Locals = append(r.Locals, root.Locals...)
	r.Requests = append(r.Requests, root.Requests...)
}

func read(filename string, root *Root) (*hcl.File, hcl.Diagnostics) {
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
	file, diags := hclsyntax.ParseConfig(src, filename, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		log.Infoln("decode")
		writeDiags(map[string]*hcl.File{filename: file}, diags)
		return nil, diags
	}

	diags = gohcl.DecodeBody(file.Body, nil, root)
	if diags.HasErrors() {
		log.Infoln("decode")
		writeDiags(map[string]*hcl.File{filename: file}, diags)
		return nil, diags
	}
	return file, diags

}

func Parse(filename string) (Rest, error) {
	ret := Rest{
		Config: request.DefaultConfig(),
	}

	root := &Root{}
	file, diags := read(filename, root)
	if diags.HasErrors() {
		return ret, diags
	}

	if root.Imports != nil {
		for _, i := range *root.Imports {
			newFile := &Root{}
			file, _ = read(path.Join(path.Dir(filename), i), newFile)
			root.Add(newFile)
		}
	}

	locals, _ := decodeLocals(root)

	ctx := makeContext(locals)

	if root.Config != nil {
		if diags = gohcl.DecodeBody(root.Config.Body, ctx, &ret.Config); diags.HasErrors() {
			writeDiags(map[string]*hcl.File{filename: file}, diags)
			return ret, fmt.Errorf("error decoding HCL configuration: %w", diags)
		}
	}
	if root.Socket != nil {
		if diags = gohcl.DecodeBody(root.Socket.Body, ctx, &ret.Socket); diags.HasErrors() {
			writeDiags(map[string]*hcl.File{filename: file}, diags)
			return ret, fmt.Errorf("error decoding HCL configuration: %w", diags)
		}
		if err := ret.Socket.ParseExtras(ctx); err != nil {
			return ret, err
		}
	}

	requests := map[string]request.Request{}
	labels := []string{}
	for i, block := range root.Requests {
		req := request.Request{Label: block.Label}
		if diags = gohcl.DecodeBody(block.Body, ctx, &req); diags.HasErrors() {
			writeDiags(map[string]*hcl.File{filename: file}, diags)
			return ret, fmt.Errorf("error decoding HCL configuration: %w", diags)
		}
		for _, l := range labels {
			if l == req.Label {
				return ret, fmt.Errorf("labels must be unique: %s", l)
			}
		}
		if err := req.ParseBody(ctx); err != nil {
			return ret, err
		}
		if err := req.SetDefaults(ctx); err != nil {
			return ret, err
		}
		req.BlockIndex = i
		requests[req.Label] = req
		labels = append(labels, req.Label)
	}
	// process copy_froms
	for label, req := range requests {
		if requests[label].CopyFrom != "" {
			if _, ok := requests[req.CopyFrom]; !ok {
				return ret, fmt.Errorf("request copy_from not found: %s", req.CopyFrom)
			}
			req.CombineFrom(requests[req.CopyFrom])
			requests[label] = req
		}
		// check that required fields are set
		if requests[label].URL == "" {
			return ret, fmt.Errorf("url is required for request: %s", req.Label)
		}
	}
	ret.Requests = requests

	return ret, nil
}

func makeContext(vars map[string]cty.Value) *hcl.EvalContext {
	return &hcl.EvalContext{
		Variables: map[string]cty.Value{
			"locals": cty.ObjectVal(vars),
		},
		Functions: map[string]function.Function{
			"env":  makeEnvFunc(),
			"read": makeFileReadFunc(),
			"json": makeJSONFunc(),
			"tmpl": makeTemplateFunc(),
		},
	}
}

func writeDiags(files map[string]*hcl.File, diags hcl.Diagnostics) {
	wr := hcl.NewDiagnosticTextWriter(
		os.Stdout,
		files, // the parser's file cache, for source snippets
		78,    // wrapping width
		false, // generate colored/highlighted output
	)
	wr.WriteDiagnostics(diags)
}
