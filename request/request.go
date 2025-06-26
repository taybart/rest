package request

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
)

type Request struct {
	URL       string            `hcl:"url"`
	Method    string            `hcl:"method,optional"`
	Body      hcl.Expression    `hcl:"body,optional"`
	BasicAuth string            `hcl:"basic_auth,optional"`
	BodyRaw   string            `hcl:"body_raw,optional"`
	Headers   []string          `hcl:"headers,optional"`
	Cookies   map[string]string `hcl:"cookies,optional"`
	Query     map[string]string `hcl:"query,optional"`
	PostHook  string            `hcl:"post_hook,optional"`
	UserAgent string
	// extras
	Label  string `hcl:"label,label"`
	Delay  string `hcl:"delay,optional"`
	Expect int    `hcl:"expect,optional"`

	Remain hcl.Expression `hcl:"remain,optional"`
}

func (r *Request) Build() (*http.Request, error) {
	req, err := http.NewRequest(r.Method, r.URL, strings.NewReader(r.BodyRaw))
	if err != nil {
		return nil, err
	}
	for _, h := range r.Headers {
		hdrs := strings.Split(h, ":")
		req.Header.Add(hdrs[0], strings.TrimSpace(strings.TrimPrefix(h, hdrs[0]+":")))
	}
	if r.BasicAuth != "" {
		ba := strings.Split(r.BasicAuth, ":")
		if len(ba) != 2 {
			return nil, fmt.Errorf("malformed basic auth value should be -> user:password")
		}
		req.SetBasicAuth(ba[0], ba[1])
	}
	for n, c := range r.Cookies {
		req.AddCookie(&http.Cookie{
			Name:  n,
			Value: c,
		})
	}

	query := url.Values{}
	for k, v := range r.Query {
		query.Add(k, v)
	}
	req.URL.RawQuery = query.Encode()
	req.Header.Set("User-Agent", r.UserAgent)

	return req, nil
}

func (r *Request) SetDefaults(ctx *hcl.EvalContext) error {
	if r.Method == "" {
		r.Method = "GET"
	}
	return nil
}

func (r *Request) ParseBody(ctx *hcl.EvalContext) error {
	bodyVal, diags := r.Body.Value(ctx)
	if diags.HasErrors() {
		return fmt.Errorf("%+v", diags.Errs())
	}

	// Handle different value types
	switch bodyVal.Type() {
	case cty.String:
		body := bodyVal.AsString()
		if json.Valid([]byte(body)) {
			r.BodyRaw = body
			return nil
		}
		// Not valid JSON, treat as plain string and marshal it
		r.BodyRaw = body
		return nil

	case cty.DynamicPseudoType:
		if bodyVal.IsNull() {
			return nil
		}

	default:
		// For objects, lists, maps, etc.
		// Check if it's already been converted to a JSON string somehow
		if bodyVal.Type().IsPrimitiveType() && bodyVal.Type().FriendlyName() == "string" {
			strVal := bodyVal.AsString()
			if json.Valid([]byte(strVal)) {
				r.BodyRaw = strVal
				return nil
			}
		}
	}

	simple := ctyjson.SimpleJSONValue{Value: bodyVal}
	jsonBytes, err := simple.MarshalJSON()
	if err != nil {
		return err
	}

	if string(jsonBytes) != "null" {
		r.BodyRaw = string(jsonBytes)
	}

	return nil
}

func (r Request) String() string {
	headers := ""
	for _, h := range r.Headers {
		headers += fmt.Sprintf("%s\n", h)
	}

	return fmt.Sprintf("%s %s\n%s\n%s", r.Method, r.URL, headers, r.BodyRaw)
}
