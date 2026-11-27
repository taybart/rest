package file

import (
	"errors"
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
)

// HCLTest is a test block, decoded when the test runs (see Parser.Test) so a
// bad test doesn't break running the file's requests
type HCLTest struct {
	Label string   `hcl:"label,label"`
	Body  hcl.Body `hcl:",remain"`
}

// Test is a list of request labels run in order, sharing exports. Cleanup
// runs after, even if a step failed. Exports seeds the test's exports before
// its first step.
type Test struct {
	Label      string
	Order      []string  `hcl:"order"`
	Cleanup    []string  `hcl:"cleanup,optional"`
	RawExports cty.Value `hcl:"exports,optional"`
	Exports    map[string]any
}

// Test decodes a test block and checks that every label it names is a
// request (imported or skipped ones included)
func (p *Parser) Test(ht *HCLTest) (Test, error) {
	t := Test{Label: ht.Label}
	if err := p.decode(ht.Body, &t); err != nil {
		return t, fmt.Errorf("test(%s): %w", ht.Label, err)
	}
	if !t.RawExports.IsNull() {
		exports, err := ctyToMap(t.RawExports)
		if err != nil {
			return t, fmt.Errorf("test(%s): exports: %w", ht.Label, err)
		}
		t.Exports = exports
	}
	if len(t.Order) == 0 {
		return t, fmt.Errorf("test(%s): order is empty", ht.Label)
	}
	var errs []error
	for _, label := range append(append([]string{}, t.Order...), t.Cleanup...) {
		if p.FindRequest(label) == nil {
			errs = append(errs, fmt.Errorf("test(%s): request %q not found", ht.Label, label))
		}
	}
	return t, errors.Join(errs...)
}

// FindRequest returns the request block with label, or nil
func (p *Parser) FindRequest(label string) *HCLRequest {
	for _, r := range p.Root.Requests {
		if r.Label == label {
			return r
		}
	}
	return nil
}
