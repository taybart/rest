package request

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
)

type SocketAction string

var (
	SocketNOOP        SocketAction = "noop"
	SocketRunPlaybook SocketAction = "run"
	SocketRunEntry    SocketAction = "one-off"
	SocketListen      SocketAction = "listen"
	SocketREPL        SocketAction = "repl"
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

	u *url.URL
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
func (s *Socket) Build(arg string, config Config) (*websocket.Dialer, SocketAction, error) {
	action := SocketNOOP

	if config.NoFollowRedirect {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, action, err
	}

	cookies := []*http.Cookie{}
	if s.Cookies != nil {
		for name, cookie := range s.Cookies {
			cookies = append(cookies, &http.Cookie{
				Name:  name,
				Value: cookie,
			})
		}
	}
	s.u, err = url.Parse(s.URL)
	if err != nil {
		return nil, action, err
	}
	jar.SetCookies(s.u, cookies)

	dialer := &websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: 45 * time.Second,
		Jar:              jar,
	}
	switch arg {
	case "run":
		if s.Run != nil {
			action = SocketRunPlaybook
		}
	case "listen":
		action = SocketListen
	case "":
		action = SocketREPL
	default:
		action = SocketRunEntry

	}

	return dialer, action, nil
}
func (s Socket) String() string {
	// headers := ""
	// for _, h := range r.Headers {
	// 	headers += fmt.Sprintf("%s\n", h)
	// }

	// return fmt.Sprintf("%s %s\n%s\n%s", r.Method, r.URL, headers, r.BodyRaw)
	return ""
}
