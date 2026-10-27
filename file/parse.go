// Package file provides a parser for the hcl configuration file format.
package file

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

type CLIFlag struct {
	Desc     string `hcl:"desc"`
	Short    string `hcl:"short"`
	Default  string `hcl:"default"`
	Bool     bool   `hcl:"bool"`
	Required bool   `hcl:"required"`
}

type CLI struct {
	Fn        *string `hcl:"fn,optional"`
	Loop      *string `hcl:"loop,optional"`
	Flags     map[string]CLIFlag
	FlagsBody hcl.Body `hcl:",remain"`
}

type Root struct {
	filename string

	Imports *[]string `hcl:"imports"`
	Exports *[]string `hcl:"exports"`

	// TODO: allow for read() in this context
	CLI *CLI `hcl:"cli,block"`

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
	Ctx        *hcl.EvalContext
	Files      map[string]*hcl.File
	Root       *Root
	Locals     map[string]cty.Value
	Exports    map[string]cty.Value
	RawExports map[string]any
	Config     request.Config
}

func NewParser(filename string) (*Parser, error) {
	p := &Parser{
		Root: &Root{
			filename: filename,
		},
		Config:     request.DefaultConfig(),
		Files:      map[string]*hcl.File{},
		Locals:     map[string]cty.Value{},
		Exports:    map[string]cty.Value{},
		RawExports: map[string]any{},
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

func stripShebang(src []byte) []byte {
	if len(src) >= 2 && src[0] == '#' && src[1] == '!' {
		if idx := bytes.IndexByte(src, '\n'); idx != -1 {
			return src[idx+1:]
		}
		return []byte{}
	}
	return src
}

func (p *Parser) read(filename string, root *Root) error {
	var src []byte
	var err error
	if filename == "-" {
		src, err = io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("failed to read stdin: %w", err)
		}
	} else {
		src, err = os.ReadFile(filename)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("file %s does not exist", filename)
			}
			return fmt.Errorf("failed to read %s: %w", filename, err)
		}
	}
	src = stripShebang(src)
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

func (p *Parser) CLI() (CLI, error) {
	if p.Root.CLI == nil {
		return CLI{}, errors.New("cli block not found")
	}

	cli := p.Root.CLI
	if cli.FlagsBody != nil {
		cli.Flags = make(map[string]CLIFlag)
		attrs, diags := cli.FlagsBody.JustAttributes()
		if diags.HasErrors() {
			p.writeDiags(diags)
			return *cli, errors.New("error parsing cli flags")
		}
		flagsAttr, ok := attrs["flags"]
		if !ok {
			return *cli, nil // no flags defined
		}

		val, diags := flagsAttr.Expr.Value(p.Ctx)
		if diags.HasErrors() {
			p.writeDiags(diags)
			return *cli, errors.New("error evaluating flags")
		}

		if val.Type().IsObjectType() || val.Type().IsMapType() {
			for name, flagVal := range val.AsValueMap() {
				var flag CLIFlag
				if flagVal.Type().HasAttribute("desc") {
					flag.Desc = flagVal.GetAttr("desc").AsString()
				}
				if flagVal.Type().HasAttribute("short") {
					flag.Short = flagVal.GetAttr("short").AsString()
				}
				if flagVal.Type().HasAttribute("bool") {
					flag.Bool = flagVal.GetAttr("bool").True()
				}
				if flagVal.Type().HasAttribute("default") {
					flag.Default = flagVal.GetAttr("default").AsString()
				}
				if flagVal.Type().HasAttribute("required") {
					flag.Required = flagVal.GetAttr("required").True()
				}
				cli.Flags[name] = flag
			}
		}
	}
	return *cli, nil
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
	if serv.KVHCL != nil {
		kv, err := p.marshalKV(serv.KVHCL)
		if err != nil {
			return serv, err
		}
		serv.KV = kv
	}
	mwNames := make(map[string]bool, len(serv.Middlewares))
	for _, mw := range serv.Middlewares {
		if mwNames[mw.Name] {
			return serv, fmt.Errorf("duplicate middleware %q", mw.Name)
		}
		mwNames[mw.Name] = true
	}
	if len(serv.Handlers) != 0 {
		for k, handler := range serv.Handlers {
			for _, mwName := range handler.Middleware {
				if !mwNames[mwName] {
					return serv, fmt.Errorf("handler %s %s: middleware %q not found", handler.Method, handler.Path, mwName)
				}
			}
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
	req, err := p.request(hreq)
	if err != nil {
		return req, err
	}
	if p.Config.DefaultContentType && json.Valid([]byte(req.Body)) && !hasContentTypeHeader(req.Headers) {
		if req.Headers == nil {
			req.Headers = make(map[string]string)
		}
		req.Headers["Content-Type"] = "application/json"
	}
	return req, nil
}

func (p *Parser) request(hreq *HCLRequest) (request.Request, error) {
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

	after, err := p.marshalAfter(req.AfterHCL)
	if err != nil {
		return req, err
	}
	req.After = after

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
		copyFrom, err := p.request(copyFromBody)
		if err != nil {
			return req, err
		}
		req.CombineFrom(copyFrom)
	}

	body, err := p.marshalRequestBody(req.BodyHCL, req.Headers)
	if err != nil {
		return req, err
	}
	if body != "" {
		req.Body = body
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
	req.Exports = p.RawExports
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
		if len(v) == 0 {
			return cty.EmptyTupleVal
		}
		ctyList := make([]cty.Value, len(v))
		for i, item := range v {
			ctyList[i] = valueToCty(item)
		}
		return cty.TupleVal(ctyList)
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
	for k, v := range exports {
		p.RawExports[k] = v
		p.Exports[k] = valueToCty(v)
	}
	p.Ctx.Variables["exports"] = cty.ObjectVal(p.Exports)
	p.Ctx.Functions["try_exports"] = makeTryExportsFunc(p.Exports)
}

func (p *Parser) makeContext() {
	p.Ctx = &hcl.EvalContext{
		Variables: map[string]cty.Value{
			"locals":  cty.ObjectVal(p.Locals),
			"exports": cty.ObjectVal(p.Exports),
			"lib":     makeLibObject(),
			"is":      makeIsObject(),
		},
		Functions: map[string]function.Function{
			"b64_dec":          makeBase64DecodeFunc(),
			"b64_enc":          makeBase64EncodeFunc(),
			"btmpl":            makeTemplateFunc(),
			"env":              makeEnvFunc(),
			"form":             makeFormFunc(),
			"is":               makeIsFunc(),
			"json_dec":         makeJSONDecodeFunc(),
			"json_enc":         makeJSONEncodeFunc(),
			"jwt_dec":          makeJWTDecodeFunc(),
			"jwt_enc":          makeJWTSignFunc(),
			"jwt_sign":         makeJWTSignFunc(),
			"lib::copy_res":    makeLuaHookFunc("tools.copy_res"),
			"lib::export_res":  makeLuaHookFunc("tools.export_res"),
			"lib::inspect_jwt": makeLuaHookFunc("tools.inspect_jwt"),
			"lib::inspect_req": makeLuaHookFunc("tools.inspect_req"),
			"lib::inspect_res": makeLuaHookFunc("tools.inspect_res"),
			"nanoid":           makeNanoIDFunc(),
			"raw":              makeRawFunc(),
			"read":             makeFileReadFunc(),
			"tmpl":             makeGoTemplateFunc(),
			"try_exports":      makeTryExportsFunc(p.Exports),
			"trim":             makeTrimFunc(),
			"ulid":             makeULIDFunc(),
			"uuid":             makeUUIDFunc(),
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

func hasContentTypeHeader(headers map[string]string) bool {
	for k := range headers {
		if strings.EqualFold(k, "Content-Type") {
			return true
		}
	}
	return false
}

func isFormContentType(headers map[string]string) bool {
	for k, v := range headers {
		if strings.EqualFold(k, "Content-Type") {
			ct := strings.ToLower(strings.TrimSpace(v))
			if strings.HasPrefix(ct, "application/x-www-form-urlencoded") {
				return true
			}
		}
	}
	return false
}

func (p *Parser) marshalRequestBody(bodyHCL hcl.Expression, headers map[string]string) (string, error) {
	if isFormContentType(headers) {
		bodyVal, diags := bodyHCL.Value(p.Ctx)
		if diags.HasErrors() {
			p.writeDiags(diags)
			return "", errors.New("could not decode body")
		}
		if bodyVal.Type().IsMapType() || bodyVal.Type().IsObjectType() {
			return encodeFormValue(bodyVal)
		}
	}
	return p.marshalBody(bodyHCL)
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

func (p *Parser) marshalAfter(afterHCL hcl.Expression) (string, error) {
	if afterHCL == nil {
		return "", nil
	}
	afterVal, diags := afterHCL.Value(p.Ctx)
	if diags.HasErrors() {
		p.writeDiags(diags)
		return "", errors.New("could not decode after")
	}
	afterVal, _ = afterVal.Unmark()
	if afterVal.IsNull() {
		return "", nil
	}
	if afterVal.Type() == cty.String {
		return afterVal.AsString(), nil
	}
	if afterVal.Type().IsTupleType() || afterVal.Type().IsListType() {
		var parts []string
		it := afterVal.ElementIterator()
		for it.Next() {
			_, elem := it.Element()
			elem, _ = elem.Unmark()
			if elem.IsNull() {
				continue
			}
			if elem.Type() != cty.String {
				return "", fmt.Errorf("after list elements must be strings, got %s", elem.Type().FriendlyName())
			}
			parts = append(parts, elem.AsString())
		}
		return strings.Join(parts, "\n"), nil
	}
	return "", fmt.Errorf("after must be a string or list of strings, got %s", afterVal.Type().FriendlyName())
}

func (p *Parser) marshalKV(kvHCL hcl.Expression) (map[string]any, error) {
	if kvHCL == nil {
		return nil, nil
	}
	kvVal, diags := kvHCL.Value(p.Ctx)
	if diags.HasErrors() {
		p.writeDiags(diags)
		return nil, errors.New("could not decode server kv")
	}
	kvVal, _ = kvVal.Unmark()
	if kvVal.IsNull() {
		return nil, nil
	}
	if !kvVal.Type().IsMapType() && !kvVal.Type().IsObjectType() {
		return nil, fmt.Errorf("server kv must be a map or object, got %s", kvVal.Type().FriendlyName())
	}
	simple := ctyjson.SimpleJSONValue{Value: kvVal}
	jsonBytes, err := simple.MarshalJSON()
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(jsonBytes, &out); err != nil {
		return nil, err
	}
	return out, nil
}
