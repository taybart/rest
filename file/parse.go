// Package file provides a parser for the hcl configuration file format.
package file

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/taybart/rest/request"
	"github.com/taybart/rest/server"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	ctyjson "github.com/zclconf/go-cty/cty/json"
)

type Rest struct {
	Config   request.Config
	Socket   request.Socket
	Server   server.Config
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

	Server *struct {
		Body hcl.Body `hcl:",remain"`
	} `hcl:"server,block"`

	Socket *struct {
		Body hcl.Body `hcl:",remain"`
	} `hcl:"socket,block"`
}

type Parser struct {
	Ctx    *hcl.EvalContext
	Files  map[string]*hcl.File
	Root   *Root
	Locals map[string]cty.Value
}

func (r *Root) Add(root *Root) {
	r.Locals = append(r.Locals, root.Locals...)
	// TODO: namespace labels AND mark added to not be executed
	r.Requests = append(r.Requests, root.Requests...)
}

func (p *Parser) read(filename string, root *Root) error {
	src, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("file %s does not exist", filename)
		}
		return fmt.Errorf("failed to read %s: %w", filename, err)
	}
	var diags hcl.Diagnostics
	p.Files[filename], diags = hclsyntax.ParseConfig(src, filename,
		hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		p.writeDiags(diags)
		return errors.New("failed to read rest file")
	}

	if diags := gohcl.DecodeBody(p.Files[filename].Body, nil, root); diags.HasErrors() {
		p.writeDiags(diags)
		return errors.New("failed to decode rest file")
	}
	return nil
}

func newParser(filename string) (Parser, error) {
	p := Parser{
		Root:  &Root{},
		Files: map[string]*hcl.File{},
	}

	var root Root
	if err := p.read(filename, &root); err != nil {
		return p, err
	}
	p.Root = &root

	if p.Root.Imports != nil {
		for _, i := range *p.Root.Imports {
			newFile := &Root{}
			fp := path.Join(path.Dir(filename), i)
			if err := p.read(fp, newFile); err != nil {
				return p, err
			}
			p.Root.Add(newFile)
		}
	}

	if err := p.decodeLocals(); err != nil {
		return p, err
	}
	p.makeContext()

	return p, nil
}

func Parse(filename string) (Rest, error) {
	ret := Rest{
		Config: request.DefaultConfig(),
	}

	p, err := newParser(filename)
	if err != nil {
		return ret, err
	}

	if p.Root.Config != nil {
		if err := p.decode(p.Root.Config.Body, &ret.Config); err != nil {
			return ret, errors.New("error decoding config block")
		}
	}
	if p.Root.Socket != nil {
		if err := p.decode(p.Root.Socket.Body, &ret.Socket); err != nil {
			return ret, errors.New("error decoding socket block")
		}
		if err := ret.Socket.ParseExtras(p.Ctx); err != nil {
			return ret, err
		}
	}
	if p.Root.Server != nil {
		if err := p.decode(p.Root.Server.Body, &ret.Server); err != nil {
			return ret, errors.New("error decoding server block")
		}
		b, err := p.marshalBody(ret.Server.Response.BodyHCL)
		if err != nil {
			return ret, err
		}
		ret.Server.Response.Body = json.RawMessage(b)
	}

	ret.Requests, err = p.parseRequests()
	if err != nil {
		return ret, err
	}

	return ret, nil
}
func (p *Parser) parseRequests() (map[string]request.Request, error) {
	requests := map[string]request.Request{}
	labels := []string{}
	for i, block := range p.Root.Requests {
		req := request.Request{Label: block.Label, Block: &block.Body}
		if err := p.decode(block.Body, &req); err != nil {
			return requests, fmt.Errorf("error decoding request block(%s)", block.Label)
		}
		for _, l := range labels {
			if l == req.Label {
				return requests, fmt.Errorf("labels must be unique: %s", l)
			}
		}
		var err error
		req.Body, err = p.marshalBody(req.BodyHCL)
		if err != nil {
			return requests, err
		}
		if err := req.SetDefaults(p.Ctx); err != nil {
			return requests, err
		}
		req.BlockIndex = i
		// make body look nice if its json
		if json.Valid([]byte(req.Body)) {
			var buf bytes.Buffer
			err := json.Compact(&buf, []byte(req.Body))
			if err != nil {
				return requests, err
			}
			req.Body = buf.String()
			// requests[label] = req
		}
		requests[req.Label] = req
		labels = append(labels, req.Label)
	}
	// process copy_froms
	for label, req := range requests {
		if requests[label].CopyFrom != "" {
			if _, ok := requests[req.CopyFrom]; !ok {
				return requests, fmt.Errorf("request copy_from not found: %s", req.CopyFrom)
			}
			req.CombineFrom(requests[req.CopyFrom])
			requests[label] = req
		}
		// check that required fields are set
		if requests[label].URL == "" {
			return requests, fmt.Errorf("url is required for request: %s", req.Label)
		}
	}
	return requests, nil
}

func (p *Parser) makeContext() {
	p.Ctx = &hcl.EvalContext{
		Variables: map[string]cty.Value{
			"locals": cty.ObjectVal(p.Locals),
		},
		Functions: map[string]function.Function{
			"env":   makeEnvFunc(),
			"read":  makeFileReadFunc(),
			"trim":  makeTrimFunc(),
			"json":  makeJSONFunc(),
			"btmpl": makeTemplateFunc(),
			"tmpl":  makeGoTemplateFunc(),
			"form":  makeFormFunc(),
		},
	}
}

func (p Parser) writeDiags(diags hcl.Diagnostics) {
	wr := hcl.NewDiagnosticTextWriter(
		os.Stdout,
		p.Files, // the parser's file cache, for source snippets
		80,      // wrapping width
		false,   // generate colored/highlighted output
	)
	wr.WriteDiagnostics(diags)
}

func (p *Parser) decode(body hcl.Body, to any) error {
	if diags := gohcl.DecodeBody(body, p.Ctx, to); diags.HasErrors() {
		p.writeDiags(diags)
		return errors.New("error decoding hcl body")
	}
	return nil
}

// marshalBody turns hcl expressions into a formatted json blob or go string
func (p *Parser) marshalBody(bodyHCL hcl.Expression) (string, error) {
	bodyVal, diags := bodyHCL.Value(p.Ctx)
	if diags.HasErrors() {
		p.writeDiags(diags)
		return "", errors.New("could not decode body")
	}

	// Handle different value types
	switch bodyVal.Type() {
	case cty.String:
		body := bodyVal.AsString()
		if json.Valid([]byte(body)) {
			return body, nil
		}
		// Not valid JSON, treat as plain string and marshal it
		return body, nil

	case cty.DynamicPseudoType:
		if bodyVal.IsNull() {
			return "", nil
		}

	default:
		// For objects, lists, maps, etc.
		// Check if it's already been converted to a JSON string somehow
		if bodyVal.Type().IsPrimitiveType() && bodyVal.Type().FriendlyName() == "string" {
			strVal := bodyVal.AsString()
			if json.Valid([]byte(strVal)) {
				return strVal, nil
			}
		}
	}

	simple := ctyjson.SimpleJSONValue{Value: bodyVal}
	jsonBytes, err := simple.MarshalJSON()
	if err != nil {
		return "", err
	}

	if string(jsonBytes) != "null" {
		return string(jsonBytes), nil
	}

	return "", nil
}
