//go:build js && wasm

// Command wasm exposes rest to the browser as a global restRun(src) function
package main

import (
	"bytes"
	"fmt"
	"strings"
	"syscall/js"

	"github.com/taybart/rest"
)

func main() {
	// restRun(src) runs every request in src
	js.Global().Set("restRun", js.FuncOf(func(this js.Value, args []js.Value) any {
		src := args[0].String()
		return promise(func() (string, error) {
			return runSource(src, func(r *rest.Rest, out *bytes.Buffer) error {
				return r.RunFileTo(out, true)
			})
		})
	}))
	// restRunLabel(src, label) runs just the request with that label
	js.Global().Set("restRunLabel", js.FuncOf(func(this js.Value, args []js.Value) any {
		src, label := args[0].String(), args[1].String()
		return promise(func() (string, error) {
			return runSource(src, func(r *rest.Rest, out *bytes.Buffer) error {
				return r.RunLabelTo(out, label)
			})
		})
	}))
	select {}
}

// runSource parses src and hands it to run, folding hcl diagnostics into errors
func runSource(src string, run func(*rest.Rest, *bytes.Buffer) error) (string, error) {
	var diags bytes.Buffer
	r, err := rest.NewSource("browser.rest", []byte(src), &diags)
	if err != nil {
		return "", withDiags(err, diags)
	}
	var out bytes.Buffer
	if err := run(r, &out); err != nil {
		return out.String(), withDiags(err, diags)
	}
	return out.String(), nil
}

// promise runs fn off the JS event loop, http calls deadlock if they block it
func promise(fn func() (string, error)) js.Value {
	handler := js.FuncOf(func(this js.Value, args []js.Value) any {
		resolve, reject := args[0], args[1]
		go func() {
			res, err := fn()
			if err != nil {
				reject.Invoke(js.Global().Get("Error").New(err.Error()))
				return
			}
			resolve.Invoke(res)
		}()
		return nil
	})
	defer handler.Release()
	return js.Global().Get("Promise").New(handler)
}

// withDiags folds hcl diagnostics into err so the page can show them
func withDiags(err error, diags bytes.Buffer) error {
	if d := strings.TrimSpace(diags.String()); d != "" {
		return fmt.Errorf("%w\n\n%s", err, d)
	}
	return err
}
