package file

import (
	"errors"
	"maps"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

type Local struct {
	Name string
	Expr hcl.Expression

	DeclRange hcl.Range
}

func (p *Parser) decodeLocals() error {
	// reset locals and remake context
	p.Locals = map[string]cty.Value{}
	p.makeContext()

	var diags hcl.Diagnostics
	for _, l := range p.Root.Locals {
		tmp, diag := p.decodeLocalsBlock(l.Body)
		if diag.HasErrors() {
			diags = append(diags, diag...)
		}
		maps.Copy(p.Locals, tmp)
	}
	if len(diags) != 0 {
		p.writeDiags(diags)
		return errors.New("failed to decode locals")
	}
	return nil
}

func (p *Parser) decodeLocalsBlock(block hcl.Body) (map[string]cty.Value, hcl.Diagnostics) {
	attrs, diags := block.JustAttributes()
	if len(attrs) == 0 {
		return nil, nil
	}

	locals := map[string]cty.Value{}
	for name, attr := range attrs {
		var val cty.Value
		val, diags = attr.Expr.Value(p.Ctx)
		if !hclsyntax.ValidIdentifier(name) {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid local value name",
				Detail:   "invalid id",
				Subject:  &attr.NameRange,
			})
		}
		locals[name] = val
	}

	return locals, diags
}
