package request

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

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

/*** Functions ***/

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

func makeTemplateFunc() function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{
			{
				Name: "template",
				Type: cty.String,
			},
			{
				Name:             "values",
				Type:             cty.DynamicPseudoType,
				AllowDynamicType: true,
			},
		},
		Type: function.StaticReturnType(cty.String),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			template := args[0].AsString()
			valuesArg := args[1]

			// check if named or indexed template
			switch {
			case valuesArg.Type().IsListType() || valuesArg.Type().IsTupleType():
				return replaceIndexedPlaceholders(template, valuesArg)

			case valuesArg.Type().IsMapType() || valuesArg.Type().IsObjectType():
				return replaceNamedPlaceholders(template, valuesArg)

			default:
				return cty.NilVal, fmt.Errorf("values must be a list or map, got %s", valuesArg.Type().FriendlyName())
			}
		},
	})
}

func replaceIndexedPlaceholders(template string, values cty.Value) (cty.Value, error) {
	valuesList := values.AsValueSlice()
	result := template

	// Use regex to find all indexed placeholders
	re := regexp.MustCompile(`\{\{\$(\d+)\}\}`)

	result = re.ReplaceAllStringFunc(result, func(match string) string {
		indexStr := re.FindStringSubmatch(match)[1]
		index, _ := strconv.Atoi(indexStr)

		// Check bounds
		if index >= len(valuesList) {
			// Keep placeholder if index out of bounds
			return match
		}

		val := valuesList[index]
		if val.Type() == cty.String {
			return val.AsString()
		}

		// For non-string types, convert to JSON representation
		jsonVal := ctyjson.SimpleJSONValue{Value: val}
		jsonBytes, _ := jsonVal.MarshalJSON()
		return string(jsonBytes)
	})

	return cty.StringVal(result), nil
}

func replaceNamedPlaceholders(template string, values cty.Value) (cty.Value, error) {
	valuesMap := values.AsValueMap()
	result := template

	// Use regex to find all named placeholders
	// TODO: test if whitespace in regex is too loose
	// re := regexp.MustCompile(`\{\{([a-zA-Z_]\w*)\}\}`)
	re := regexp.MustCompile(`\{\{([a-zA-Z_]*)\}\}`)

	result = re.ReplaceAllStringFunc(result, func(match string) string {
		name := re.FindStringSubmatch(match)[1]

		val, exists := valuesMap[name]
		if !exists {
			// Keep placeholder if not found
			return match
		}

		if val.Type() == cty.String {
			return val.AsString()
		}

		// For non-string types, convert to JSON representation
		jsonVal := ctyjson.SimpleJSONValue{Value: val}
		jsonBytes, _ := jsonVal.MarshalJSON()
		return string(jsonBytes)
	})

	return cty.StringVal(strings.TrimSpace(result)), nil
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
			"tmpl": makeTemplateFunc(),
		},
	}
}
