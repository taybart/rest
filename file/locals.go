package file

import (
	"errors"

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
		if diag := p.decodeLocalsBlock(l.Body); diag.HasErrors() {
			diags = append(diags, diag...)
		}
	}
	if len(diags) != 0 {
		p.writeDiags(diags)
		return errors.New("failed to decode locals")
	}
	return nil
}

func (p *Parser) decodeLocalsBlock(block hcl.Body) hcl.Diagnostics {
	attrs, diags := block.JustAttributes()
	if len(attrs) == 0 {
		return nil
	}

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
		if diags.HasErrors() {
			return diags
		}
		p.Locals[name] = val
		p.updateLocalsContext()
	}
	return diags
}
