package request

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	ctyjson "github.com/zclconf/go-cty/cty/json"
)

type Socket struct {
	URL    string `hcl:"url"`
	Origin string `hcl:"origin,optional"`
	// Playbook       map[string]hcl.Expression `hcl:"playbook,optional"`
	PlaybookParsed map[string]string
	Headers        []string          `hcl:"headers,optional"`
	Cookies        map[string]string `hcl:"cookies,optional"`
	UserAgent      string

	Remain hcl.Attributes `hcl:",remain"`
	// extras
	Label  string `hcl:"label,label"`
	Expect int    `hcl:"expect,optional"`
}

func (s *Socket) ParsePlaybook(ctx *hcl.EvalContext) error {
	s.PlaybookParsed = make(map[string]string)

	// Look for "playbook" attribute in remaining attributes
	if playbook, ok := s.Remain["playbook"]; ok {
		// Evaluate the playbook expression to get the map
		val, diags := playbook.Expr.Value(ctx)
		if diags.HasErrors() {
			return fmt.Errorf("error evaluating playbook: %w", diags)
		}

		// Convert each map entry to JSON
		if val.Type().IsObjectType() || val.Type().IsMapType() {
			for it := val.ElementIterator(); it.Next(); {
				key, value := it.Element()
				keyStr := key.AsString()

				// Convert value to JSON
				jsonBytes, err := ctyjson.Marshal(value, value.Type())
				if err != nil {
					return fmt.Errorf("error marshaling playbook entry '%s': %w", keyStr, err)
				}

				s.PlaybookParsed[keyStr] = string(jsonBytes)
			}
		}
	}

	return nil
}
func (s Socket) String() string {
	// headers := ""
	// for _, h := range r.Headers {
	// 	headers += fmt.Sprintf("%s\n", h)
	// }

	// return fmt.Sprintf("%s %s\n%s\n%s", r.Method, r.URL, headers, r.BodyRaw)
	return ""
}
