package file

import (
	"fmt"
	"os"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

// deprecatedFuncs are functions that still work, but have a new name (or
// way of doing it), shown in a warning.
var deprecatedFuncs = map[string]string{
	"try_exports": `use exports::try("key", default)`,
}

// deprecations returns a warning for each use of something deprecated in
// body.
func deprecations(body hcl.Body) hcl.Diagnostics {
	syntaxBody, ok := body.(*hclsyntax.Body)
	if !ok {
		return nil
	}
	var diags hcl.Diagnostics
	hclsyntax.VisitAll(syntaxBody, func(node hclsyntax.Node) hcl.Diagnostics {
		if call, ok := node.(*hclsyntax.FunctionCallExpr); ok {
			if instead, ok := deprecatedFuncs[call.Name]; ok {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagWarning,
					Summary:  fmt.Sprintf("%s is deprecated", call.Name),
					Detail:   fmt.Sprintf("%s is deprecated, %s.", call.Name, instead),
					Subject:  call.NameRange.Ptr(),
				})
			}
		}
		return nil
	})
	return diags
}

// warn writes warnings to stderr (or the parser's DiagOut, if it isn't
// stdout), so they don't end up in output (ex. exports).
func (p *Parser) warn(diags hcl.Diagnostics) {
	if len(diags) == 0 {
		return
	}
	out := p.DiagOut
	if out == nil || out == os.Stdout {
		out = os.Stderr
	}
	wr := hcl.NewDiagnosticTextWriter(out, p.Files, 80, false)
	wr.WriteDiagnostics(diags)
}
