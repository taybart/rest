package file

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"text/template"

	"github.com/google/uuid"
	gonanoid "github.com/matoous/go-nanoid/v2"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	"github.com/zclconf/go-cty/cty/gocty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
)

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
			pathArg, _ := args[0].Unmark()
			path := pathArg.AsString()
			if strings.HasPrefix(path, "~/") {
				home, _ := os.UserHomeDir()
				path = filepath.Join(home, path[2:])
			}
			val, err := os.ReadFile(path)
			if err != nil {
				return cty.StringVal(""), err
			}
			return cty.StringVal(string(val)), nil
		},
	})
}
func makeTrimFunc() function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{
			{
				Name: "messy",
				Type: cty.String,
			},
		},
		Type: function.StaticReturnType(cty.String),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			if len(args) == 0 {
				return cty.StringVal(""), nil
			}
			messy := args[0]
			return cty.StringVal(strings.TrimSpace(messy.AsString())), nil
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

func makeFormFunc() function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{
			{
				Name:             "values",
				Type:             cty.DynamicPseudoType,
				AllowDynamicType: true,
			},
		},
		Type: function.StaticReturnType(cty.String),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			valuesArg := args[0]

			if valuesArg.Type().IsMapType() || valuesArg.Type().IsObjectType() {
				valuesMap := valuesArg.AsValueMap()
				data := url.Values{}
				for k, v := range valuesMap {
					data.Set(k, v.AsString())
				}

				return cty.StringVal(data.Encode()), nil
			}

			return cty.NilVal, fmt.Errorf("values must be a list or map, got %s", valuesArg.Type().FriendlyName())
		},
	})
}

func makeUUIDFunc() function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{},
		Type:   function.StaticReturnType(cty.String),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			return cty.StringVal(uuid.NewString()), nil
		},
	})
}

func makeNanoIDFunc() function.Function {
	return function.New(&function.Spec{
		VarParam: &function.Parameter{
			Name: "args",
			Type: cty.DynamicPseudoType,
		},
		Type: function.StaticReturnType(cty.String),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			// Default values
			lengthInt := 21
			var alphabet string

			// Handle optional size argument
			if len(args) > 0 && !args[0].IsNull() {
				err := gocty.FromCtyValue(args[0], &lengthInt)
				if err != nil {
					return cty.NilVal, err
				}
			}

			// Handle optional alphabet argument
			if len(args) > 1 && !args[1].IsNull() {
				alphabet = args[1].AsString()
			}

			// Generate the ID
			if alphabet != "" {
				id, err := gonanoid.Generate(alphabet, lengthInt)
				if err != nil {
					return cty.NilVal, err
				}
				return cty.StringVal(id), nil
			}

			id, err := gonanoid.New(lengthInt)
			if err != nil {
				return cty.NilVal, err
			}
			return cty.StringVal(id), nil
		},
	})
}

func makeGoTemplateFunc() function.Function {
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
			tmpl := args[0].AsString()
			valuesArg := args[1]

			if valuesArg.Type().IsMapType() || valuesArg.Type().IsObjectType() {
				valuesMap := valuesArg.AsValueMap()
				values := make(map[string]any)
				for k, v := range valuesMap {
					values[k] = v.AsString()
				}

				t, err := template.New("tmpl").
					Funcs(template.FuncMap{
						"get": func(key string) string {
							if val, ok := values[key]; ok && val != nil {
								return fmt.Sprintf("%v", val)
							}
							return fmt.Sprintf("{{.%s}}", key)
						},
						"uuid": func() string {
							return uuid.NewString()
						},
						"nanoid": func(args ...any) string {
							length := 21
							alphabet := ""

							if len(args) >= 1 {
								switch v := args[0].(type) {
								case int:
									length = v
								case string:
									if l, err := strconv.Atoi(v); err == nil {
										length = l
									}
								}
							}
							if len(args) >= 2 {
								if str, ok := args[1].(string); ok {
									alphabet = str
								}
							}
							if alphabet != "" {
								return gonanoid.MustGenerate(alphabet, length)
							}
							return gonanoid.Must(length)
						},
					}).Parse(tmpl)
				if err != nil {
					return cty.StringVal(""), err
				}
				var ret strings.Builder
				if err := t.Execute(&ret, values); err != nil {
					return cty.StringVal(""), err
				}
				return cty.StringVal(ret.String()), nil
			}

			return cty.StringVal(""), fmt.Errorf("values must be a list or map, got %s", valuesArg.Type().FriendlyName())
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
	re := regexp.MustCompile(`\{\{([\.a-zA-Z_]*)\}\}`)

	result = re.ReplaceAllStringFunc(result, func(match string) string {
		name := re.FindStringSubmatch(match)[1]

		val, exists := valuesMap[name]
		// Keep placeholder if not found
		if !exists {
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

	// fmt.Println(result)
	return cty.StringVal(result), nil
}
