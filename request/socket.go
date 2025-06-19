package request

import (
	"encoding/json"
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
)

type SocketOrder struct {
	Delay string   `hcl:"delay,optional"`
	Order []string `hcl:"order,optional"`
}

type Socket struct {
	URL     string            `hcl:"url"`
	Origin  string            `hcl:"origin,optional"`
	Headers []string          `hcl:"headers,optional"`
	Cookies map[string]string `hcl:"cookies,optional"`

	// parsed fields
	Run      *SocketOrder
	Playbook map[string]string

	Remain hcl.Attributes `hcl:",remain"`
	// extras
	Label  string `hcl:"label,label"`
	Expect int    `hcl:"expect,optional"`
}

func (s *Socket) ParseExtras(ctx *hcl.EvalContext) error {
	s.Playbook = make(map[string]string)
	if runAttr, ok := s.Remain["run"]; ok {
		val, diags := runAttr.Expr.Value(ctx)
		if diags.HasErrors() {
			return fmt.Errorf("error evaluating run: %w", diags)
		}

		s.Run = &SocketOrder{}

		// Extract delay
		if val.Type().HasAttribute("delay") {
			delayVal := val.GetAttr("delay")
			s.Run.Delay = delayVal.AsString()
		}

		// Extract order array
		if val.Type().HasAttribute("order") {
			orderVal := val.GetAttr("order")
			for it := orderVal.ElementIterator(); it.Next(); {
				_, elem := it.Element()
				s.Run.Order = append(s.Run.Order, elem.AsString())
			}
		}
	}
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

				// If it's already a string that looks like JSON, use it directly
				if value.Type() == cty.String {
					strVal := value.AsString()
					if json.Valid([]byte(strVal)) {
						s.Playbook[keyStr] = string(strVal)
						continue
					}
				}
				// Convert value to JSON
				jsonBytes, err := ctyjson.Marshal(value, value.Type())
				if err != nil {
					return fmt.Errorf("error marshaling playbook entry '%s': %w", keyStr, err)
				}

				s.Playbook[keyStr] = string(jsonBytes)
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
