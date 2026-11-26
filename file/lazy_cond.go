package file

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"
)

// HCL evaluates both branches of `cond ? a : b` (to unify their types), so
// `locals.prod ? op::read(...) : "admin"` would call op even when it's not
// prod. lazyConditionals wraps each branch so the one the condition doesn't
// pick is never evaluated: it's a dynamic value, which HCL doesn't unify
// with, so the picked branch's value is the result, as is.

// lazyBranch is a conditional's branch, evaluated only if cond is when
type lazyBranch struct {
	hclsyntax.Expression
	cond hclsyntax.Expression
	when bool
}

func (b *lazyBranch) Value(ctx *hcl.EvalContext) (cty.Value, hcl.Diagnostics) {
	cond, diags := b.cond.Value(ctx)
	// anything but a clear true/false is left to the conditional to report
	if !diags.HasErrors() && cond.IsWhollyKnown() && !cond.IsNull() {
		cond, _ = cond.Unmark()
		if cond, err := convert.Convert(cond, cty.Bool); err == nil && cond.True() != b.when {
			return cty.DynamicVal, nil
		}
	}
	return b.Expression.Value(ctx)
}

// lazyConditionals makes every conditional in body only evaluate the branch
// it picks
func lazyConditionals(body hclsyntax.Node) {
	hclsyntax.VisitAll(body, func(n hclsyntax.Node) hcl.Diagnostics {
		c, ok := n.(*hclsyntax.ConditionalExpr)
		if !ok {
			return nil
		}
		if _, done := c.TrueResult.(*lazyBranch); done {
			return nil
		}
		c.TrueResult = &lazyBranch{Expression: c.TrueResult, cond: c.Condition, when: true}
		c.FalseResult = &lazyBranch{Expression: c.FalseResult, cond: c.Condition, when: false}
		return nil
	})
}
