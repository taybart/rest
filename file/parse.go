// Package file provides a parser for the hcl configuration file format.
package file

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/taybart/rest/request"
	"github.com/taybart/rest/server"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	ctyjson "github.com/zclconf/go-cty/cty/json"
)

type HCLRequest struct {
	shouldSkip bool
	Label      string   `hcl:"label,label"`
	Body       hcl.Body `hcl:",remain"`
	BlockIndex int
}

type Root struct {
	filename string

	Imports *[]string `hcl:"imports"`
	Exports *[]string `hcl:"exports"`

	CLI *string `hcl:"cli,optional"`

	Locals []*struct {
		Body hcl.Body `hcl:",remain"`
	} `hcl:"locals,block"`

	Config *struct {
		Body hcl.Body `hcl:",remain"`
	} `hcl:"config,block"`

	Requests []*HCLRequest `hcl:"request,block"`

	Server *struct {
		Body hcl.Body `hcl:",remain"`
	} `hcl:"server,block"`

	Socket *struct {
		Body hcl.Body `hcl:",remain"`
	} `hcl:"socket,block"`
}

func (r *Root) Add(root *Root, config request.Config) {
	r.Locals = append(r.Locals, root.Locals...)
	for _, req := range root.Requests {
		if config.SkipImported {
			req.shouldSkip = true
		}
		if !config.NamespaceImports {
			r.Requests = append(r.Requests, req)
			continue
		}
		for _, req := range root.Requests {
			base := filepath.Base(root.filename)
			namespace := strings.TrimSuffix(base, filepath.Ext(base))
			req.Label = fmt.Sprintf("%s::%s", namespace, req.Label)
			r.Requests = append(r.Requests, req)
		}
	}
}

type Parser struct {
	Ctx     *hcl.EvalContext
	Files   map[string]*hcl.File
	Root    *Root
	Locals  map[string]cty.Value
	Exports map[string]cty.Value
	Config  request.Config
}

func NewParser(filename string) (*Parser, error) {
	p := &Parser{
		Root: &Root{
			filename: filename,
		},
		Config:  request.DefaultConfig(),
		Files:   map[string]*hcl.File{},
		Locals:  map[string]cty.Value{},
		Exports: map[string]cty.Value{},
	}

	if err := p.read(filename, p.Root); err != nil {
		return p, err
	}

	if p.Root.Config != nil {
		if err := p.decode(p.Root.Config.Body, &p.Config); err != nil {
			return p, errors.New("error decoding config block")
		}
	}

	if p.Root.Imports != nil {
		for _, i := range *p.Root.Imports {
			importedRest := &Root{filename: i}
			fp := path.Join(path.Dir(filename), i)
			if err := p.read(fp, importedRest); err != nil {
				return p, err
			}
			// get settings from imported file
			config := p.Config
			if importedRest.Config != nil {
				if err := p.decode(importedRest.Config.Body, &config); err != nil {
					return p, errors.New("error decoding config block")
				}
			}
			p.Root.Add(importedRest, config)
		}
	}
	if err := p.decodeLocals(); err != nil {
		return p, err
	}

	return p, nil
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
	p.Files[filename], diags = hclsyntax.ParseConfig(
		src, filename,
		hcl.Pos{Line: 1, Column: 1},
	)
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

func (p *Parser) Socket() (request.Socket, error) {
	var sock request.Socket
	if p.Root.Socket == nil {
		return sock, errors.New("socket block not found")
	}
	if err := p.decode(p.Root.Socket.Body, &sock); err != nil {
		return sock, errors.New("error decoding socket block")
	}
	if err := sock.ParseExtras(p.Ctx); err != nil {
		return sock, err
	}
	return sock, nil
}

func (p *Parser) Server() (server.Config, error) {
	var serv server.Config
	if p.Root.Server == nil {
		return serv, errors.New("server block not found")
	}
	if err := p.decode(p.Root.Server.Body, &serv); err != nil {
		return serv, errors.New("error decoding server block")
	}
	if serv.Response != nil {
		b, err := p.marshalBody(serv.Response.BodyHCL)
		if err != nil {
			return serv, err
		}
		serv.Response.Body = json.RawMessage(b)
	}
	if len(serv.Handlers) != 0 {
		for k, handler := range serv.Handlers {
			if handler.Response != nil {
				b, err := p.marshalBody(handler.Response.BodyHCL)
				if err != nil {
					return serv, err
				}
				serv.Handlers[k].Response.Body = json.RawMessage(b)
			}
		}
	}
	return serv, nil
}

func (p *Parser) Request(hreq *HCLRequest) (request.Request, error) {
	if hreq == nil {
		return request.Request{}, fmt.Errorf("request not found")
	}

	req := request.Request{Label: hreq.Label, Block: &hreq.Body}
	if err := p.decode(hreq.Body, &req); err != nil {
		return req, fmt.Errorf("error decoding request hreq(%s)", hreq.Label)
	}
	if hreq.shouldSkip {
		req.Skip = true
	}

	if req.CopyFrom != "" {
		var copyFromBody *HCLRequest
		for _, h := range p.Root.Requests {
			if h.Label == req.CopyFrom {
				copyFromBody = h
			}
		}
		if copyFromBody == nil {
			return req, fmt.Errorf("request (%s) copy_from not found: %s", hreq.Label, req.CopyFrom)
		}
		copyFrom, err := p.Request(copyFromBody)
		if err != nil {
			return req, err
		}
		req.CombineFrom(copyFrom)
	}

	var err error
	req.Body, err = p.marshalBody(req.BodyHCL)
	if err != nil {
		return req, err
	}
	if req.Expect != nil {
		req.Expect.Body, err = p.marshalBody(req.Expect.BodyHCL)
		if err != nil {
			return req, err
		}
	}
	if err := req.SetDefaults(p.Ctx); err != nil {
		return req, err
	}
	// make body look nice if its json
	if json.Valid([]byte(req.Body)) {
		var buf bytes.Buffer
		err := json.Compact(&buf, []byte(req.Body))
		if err != nil {
			return req, err
		}
		req.Body = buf.String()
		// requests[label] = req
	}
	return req, nil
}

func (p *Parser) Requests() (map[string]request.Request, error) {
	requests := map[string]request.Request{}
	labels := []string{}

	for i, block := range p.Root.Requests {
		if slices.Contains(labels, block.Label) {
			return requests, fmt.Errorf(`labels must be unique: "%s" already exists`, block.Label)
		}

		// TODO: this needs a lot of information, maybe its best to break
		// up parsing and pass back the parser to the rest object
		req, err := p.Request(block)
		if err != nil {
			return requests, err
		}
		// req.BlockIndex = i
		p.Root.Requests[i].BlockIndex = i

		requests[req.Label] = req
		labels = append(labels, req.Label)
	}
	// process copy_froms
	for label, req := range requests {
		if requests[label].CopyFrom != "" {
			if _, ok := requests[req.CopyFrom]; !ok {
				return requests, fmt.Errorf("request (%s) copy_from not found: %s", label, req.CopyFrom)
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

func (p *Parser) updateLocalsContext() {
	p.Ctx.Variables["locals"] = cty.ObjectVal(p.Locals)
}

func valueToCty(v any) cty.Value {
	switch v := v.(type) {
	case string:
		return cty.StringVal(v)
	case float64:
		return cty.NumberVal(big.NewFloat(v))
	case int:
		return cty.NumberVal(big.NewFloat(float64(v)))
	case bool:
		return cty.BoolVal(v)
	case []any:
		ctyList := make([]cty.Value, len(v))
		for i, item := range v {
			ctyList[i] = valueToCty(item)
		}
		return cty.ListVal(ctyList)
	case map[string]any:
		return cty.ObjectVal(exportsToCty(v))
	default:
		return cty.NullVal(cty.DynamicPseudoType)
	}
}
func exportsToCty(exports map[string]any) map[string]cty.Value {
	ret := map[string]cty.Value{}
	for k, v := range exports {
		ret[k] = valueToCty(v)
	}
	return ret
}
func (p *Parser) AddExportsCtx(exports map[string]any) {
	p.Exports = exportsToCty(exports)
	p.Ctx.Variables["exports"] = cty.ObjectVal(p.Exports)
}

func (p *Parser) makeContext() {
	p.Ctx = &hcl.EvalContext{
		Variables: map[string]cty.Value{
			"locals":  cty.ObjectVal(p.Locals),
			"exports": cty.ObjectVal(p.Exports),
		},
		Functions: map[string]function.Function{
			"b64_dec":  makeBase64DecodeFunc(),
			"b64_enc":  makeBase64EncodeFunc(),
			"btmpl":    makeTemplateFunc(),
			"env":      makeEnvFunc(),
			"form":     makeFormFunc(),
			"json_dec": makeJSONDecodeFunc(),
			"json_enc": makeJSONEncodeFunc(),
			"nanoid":   makeNanoIDFunc(),
			"read":     makeFileReadFunc(),
			"tmpl":     makeGoTemplateFunc(),
			"trim":     makeTrimFunc(),
			"uuid":     makeUUIDFunc(),
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
