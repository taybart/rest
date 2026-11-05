package file

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/taybart/rest/request"
	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
)

type Local struct {
	Name string
	Expr hcl.Expression

	DeclRange hcl.Range
}

// namespaces evaluated together, so locals and schemas can reference each
// other
var namespaces = []string{"locals", "schemas"}

type varDef struct {
	ns, name string
	attr     *hcl.Attribute
}

func (d *varDef) key() string { return d.ns + "." + d.name }

// decodeLocals evaluates every locals and schemas block.
//
//   - Definitions can reference each other in any order, across blocks and
//     imported files, they're evaluated dependencies first.
//   - A name can only be defined once per file.
//   - The importing file overrides its imports, and a later import overrides
//     an earlier one. A library can also use a local it never defines and
//     leave it to the file importing it.
func (p *Parser) decodeLocals() error {
	// reset locals and remake context
	p.Locals = map[string]cty.Value{}
	p.Schemas = map[string]cty.Value{}
	p.makeContext()

	var diags hcl.Diagnostics
	defs := map[string]*varDef{}
	collect := func(ns string, bodies []hcl.Body) {
		for _, body := range bodies {
			attrs, d := body.JustAttributes()
			diags = append(diags, d...)
			for name, attr := range attrs {
				if !hclsyntax.ValidIdentifier(name) {
					diags = append(diags, &hcl.Diagnostic{
						Severity: hcl.DiagError,
						Summary:  "Invalid " + ns + " name",
						Detail:   "invalid id",
						Subject:  &attr.NameRange,
					})
					continue
				}
				def := &varDef{ns: ns, name: name, attr: attr}
				prev, ok := defs[def.key()]
				if !ok {
					defs[def.key()] = def
					continue
				}
				prevFile, file := prev.attr.NameRange.Filename, attr.NameRange.Filename
				if prevFile == file {
					diags = append(diags, &hcl.Diagnostic{
						Severity: hcl.DiagError,
						Summary:  "Duplicate " + ns + " definition",
						Detail: fmt.Sprintf("%s is already defined at %s, a name can only be defined once per file",
							def.key(), prev.attr.NameRange),
						Subject: &attr.NameRange,
					})
					continue
				}
				if p.fileRank[file] > p.fileRank[prevFile] {
					defs[def.key()] = def
				}
			}
		}
	}
	collect("locals", bodies(p.Root.Locals))
	collect("schemas", bodies(p.Root.Schemas))

	if !diags.HasErrors() {
		diags = append(diags, p.evalDefs(defs)...)
	}
	if diags.HasErrors() {
		p.writeDiags(diags)
		return errors.New("failed to decode locals")
	}
	return nil
}

func bodies(blocks []*struct {
	Body hcl.Body `hcl:",remain"`
}) []hcl.Body {
	out := make([]hcl.Body, 0, len(blocks))
	for _, b := range blocks {
		out = append(out, b.Body)
	}
	return out
}

func (p *Parser) evalDefs(defs map[string]*varDef) hcl.Diagnostics {
	const (
		visiting = 1
		done     = 2
	)
	state := map[string]int{}
	var visit func(key string, path []string) hcl.Diagnostics
	visit = func(key string, path []string) hcl.Diagnostics {
		def := defs[key]
		switch state[key] {
		case done:
			return nil
		case visiting:
			return hcl.Diagnostics{{
				Severity: hcl.DiagError,
				Summary:  "Definitions reference each other",
				Detail:   "can't depend on itself: " + strings.Join(append(path, key), " -> "),
				Subject:  &def.attr.NameRange,
			}}
		}
		state[key] = visiting
		for _, dep := range refs(def.attr.Expr) {
			if _, ok := defs[dep]; ok {
				if diags := visit(dep, append(path, key)); diags.HasErrors() {
					return diags
				}
			}
		}
		val, diags := def.attr.Expr.Value(p.Ctx)
		if diags.HasErrors() {
			return diags
		}
		if def.ns == "schemas" {
			// a schema carries its name so exports can keep it
			b, err := ctyjson.SimpleJSONValue{Value: val}.MarshalJSON()
			if err != nil {
				return hcl.Diagnostics{{
					Severity: hcl.DiagError,
					Summary:  "Invalid schema",
					Detail:   err.Error(),
					Subject:  &def.attr.NameRange,
				}}
			}
			p.Schemas[def.name] = cty.StringVal(request.NamedToken(def.name, string(b)))
		} else {
			p.Locals[def.name] = val
		}
		p.updateLocalsContext()
		state[key] = done
		return nil
	}

	keys := make([]string, 0, len(defs))
	for k := range defs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if diags := visit(k, nil); diags.HasErrors() {
			return diags
		}
	}
	return nil
}

// refs lists the definitions an expression reads: locals.x, schemas.x or
// either with ["x"]
func refs(expr hcl.Expression) []string {
	var out []string
	for _, t := range expr.Variables() {
		root := t.RootName()
		known := false
		for _, ns := range namespaces {
			known = known || root == ns
		}
		if !known || len(t) < 2 {
			continue
		}
		switch step := t[1].(type) {
		case hcl.TraverseAttr:
			out = append(out, root+"."+step.Name)
		case hcl.TraverseIndex:
			if step.Key.Type() == cty.String && step.Key.IsKnown() && !step.Key.IsNull() {
				out = append(out, root+"."+step.Key.AsString())
			}
		}
	}
	return out
}

// SchemaValues returns what each schemas block entry holds (the matcher
// without its name), for exports that define schemas by name
func (p *Parser) SchemaValues() map[string]any {
	out := map[string]any{}
	for name, val := range p.Schemas {
		if val.IsNull() || !val.IsWhollyKnown() || val.Type() != cty.String {
			continue
		}
		if _, inner, ok := request.CutNamed(val.AsString()); ok {
			out[name] = inner
		}
	}
	return out
}
