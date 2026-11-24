package rest

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/taybart/rest/client"
	"github.com/taybart/rest/file"
)

func (rest *Rest) RunTests(name string) error {
	return rest.RunTestsTo(os.Stdout, name)
}

// RunTestsTo runs the file's test blocks in order, or just the one named
// name. Each test gets fresh exports and cookies, its steps share them. A test
// stops at its first failing step, then runs its cleanup. Returns an error if
// any test failed.
func (rest *Rest) RunTestsTo(w io.Writer, name string) error {
	tests := rest.Parser.Root.Tests
	if len(tests) == 0 {
		return fmt.Errorf("no test blocks in file")
	}
	if name != "" {
		var found *file.HCLTest
		for _, t := range tests {
			if t.Label == name {
				found = t
				break
			}
		}
		if found == nil {
			return fmt.Errorf("test %q not found", name)
		}
		tests = []*file.HCLTest{found}
	}

	failed := 0
	for i, t := range tests {
		if i > 0 {
			fmt.Fprintln(w)
		}
		ok, err := rest.runTest(w, t)
		if err != nil {
			return err
		}
		if !ok {
			failed++
		}
	}
	if len(tests) > 1 {
		fmt.Fprintf(w, "\n%d passed, %d failed\n", len(tests)-failed, failed)
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d tests failed", failed, len(tests))
	}
	return nil
}

// runTest reports whether the test passed, the error is for when it couldn't
// run at all (ex. the client couldn't be made)
func (rest *Rest) runTest(w io.Writer, ht *file.HCLTest) (bool, error) {
	fmt.Fprintf(w, "=== %s\n", ht.Label)
	// a copy so exports stay in this test
	parser := rest.Parser.WithExports(nil)
	t, err := parser.Test(ht)
	if err != nil {
		fmt.Fprintf(w, "FAIL %s: %s\n", ht.Label, indent(err.Error()))
		return false, nil
	}
	c, err := client.New(parser.Config)
	if err != nil {
		return false, err
	}
	c.Out = w
	c.AlwaysExpect = true

	step := func(label string) error {
		req, err := parser.Request(parser.FindRequest(label))
		if err != nil {
			return err
		}
		_, exports, err := c.Do(req)
		parser.AddExportsCtx(exports)
		return err
	}

	passed := 0
	var failure error
	for _, label := range t.Order {
		if err := step(label); err != nil {
			fmt.Fprintf(w, "  ✗ %s: %s\n", label, indent(err.Error()))
			failure = err
			break
		}
		fmt.Fprintf(w, "  ✓ %s\n", label)
		passed++
	}
	for _, label := range t.Cleanup {
		if err := step(label); err != nil {
			fmt.Fprintf(w, "  ↺ ✗ %s: %s\n", label, indent(err.Error()))
			continue
		}
		fmt.Fprintf(w, "  ↺ %s\n", label)
	}

	if failure != nil {
		fmt.Fprintf(w, "FAIL %s (%d/%d)\n", ht.Label, passed, len(t.Order))
		return false, nil
	}
	fmt.Fprintf(w, "PASS %s\n", ht.Label)
	return true, nil
}

// indent keeps multi-line errors (ex. several missing labels) under their
// step
func indent(s string) string {
	return strings.ReplaceAll(s, "\n", "\n    ")
}
