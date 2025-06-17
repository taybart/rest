package request

import (
	"fmt"
	"os"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	ctyjson "github.com/zclconf/go-cty/cty/json"
)

func readFile(filename string) (*hcl.File, hcl.Diagnostics) {
	src, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, hcl.Diagnostics{
				{
					Severity: hcl.DiagError,
					Summary:  "Configuration file not found",
					Detail:   fmt.Sprintf("The configuration file %s does not exist.", filename),
				},
			}
		}
		return nil, hcl.Diagnostics{
			{
				Severity: hcl.DiagError,
				Summary:  "Failed to read configuration",
				Detail:   fmt.Sprintf("Can't read %s: %s.", filename, err),
			},
		}
	}
	return hclsyntax.ParseConfig(src, filename, hcl.Pos{Line: 1, Column: 1})

}

func writeDiags(files map[string]*hcl.File, diags hcl.Diagnostics) {
	wr := hcl.NewDiagnosticTextWriter(
		os.Stdout,
		files, // the parser's file cache, for source snippets
		78,    // wrapping width
		false, // generate colored/highlighted output
	)
	wr.WriteDiagnostics(diags)
}

func makeFileReadFunc() function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{
			{
				Name:        "read",
				Type:        cty.String,
				AllowMarked: true,
			},
		},
		Type: function.StaticReturnType(cty.String),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			path, _ := args[0].Unmark()
			val, err := os.ReadFile(path.AsString())
			if err != nil {
				return cty.StringVal(""), err
			}
			return cty.StringVal(string(val)), nil
		},
	})
}
func makeEnvFunc() function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{
			{
				Name:        "env",
				Type:        cty.String,
				AllowMarked: true,
			},
		},
		Type: function.StaticReturnType(cty.String),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			envArg, _ := args[0].Unmark()
			val := os.Getenv(envArg.AsString())
			return cty.StringVal(string(val)), nil
		},
	})
}
func makeJSONFunc() function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{
			{
				Name:             "str",
				Type:             cty.String,
				AllowDynamicType: true,
			},
		},
		Type: func(args []cty.Value) (cty.Type, error) {
			if !args[0].IsKnown() {
				return cty.DynamicPseudoType, nil
			}

			jsonStr := args[0].AsString()
			jsonType, err := ctyjson.ImpliedType([]byte(jsonStr))
			if err != nil {
				return cty.DynamicPseudoType, err
			}

			return jsonType, nil
		},
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			jsonStr := args[0].AsString()

			// First determine the type
			jsonType, err := ctyjson.ImpliedType([]byte(jsonStr))
			if err != nil {
				return cty.DynamicVal, fmt.Errorf("invalid JSON: %s", err)
			}

			// Then unmarshal with that type
			val, err := ctyjson.Unmarshal([]byte(jsonStr), jsonType)
			if err != nil {
				return cty.DynamicVal, fmt.Errorf("failed to parse JSON: %s", err)
			}

			return val, nil
		},
	})
}

func makeContext(vars map[string]cty.Value) *hcl.EvalContext {
	return &hcl.EvalContext{
		Variables: map[string]cty.Value{
			"locals": cty.ObjectVal(vars),
		},
		Functions: map[string]function.Function{
			"env":  makeEnvFunc(),
			"read": makeFileReadFunc(),
			"json": makeJSONFunc(),
		},
	}
}
