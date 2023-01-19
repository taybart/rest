package request

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

type Local struct {
	Name string
	Expr hcl.Expression

	DeclRange hcl.Range
}

func decodeLocals(root Root) (map[string]cty.Value, hcl.Diagnostics) {
	var diags hcl.Diagnostics
	locals := make(map[string]cty.Value)
	for _, l := range root.Locals {
		tmp, diag := decodeLocalsBlock(l.Body)
		if diag.HasErrors() {
			diags = append(diags, diag...)
		}

		for k, v := range tmp {
			locals[k] = v
		}
	}
	return locals, nil
}

func decodeLocalsBlock(block hcl.Body) (map[string]cty.Value, hcl.Diagnostics) {
	attrs, diags := block.JustAttributes()
	if len(attrs) == 0 {
		return nil, diags
	}

	locals := map[string]cty.Value{}
	for name, attr := range attrs {
		var val cty.Value
		val, diags = attr.Expr.Value(nil)
		if !hclsyntax.ValidIdentifier(name) {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid local value name",
				Detail:   "asdf",
				Subject:  &attr.NameRange,
			})
		}
		locals[name] = val
	}

	return locals, diags
}
