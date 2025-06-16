package request

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/hashicorp/hcl/v2"
	ctyjson "github.com/zclconf/go-cty/cty/json"
)

type Request struct {
	URL              string            `hcl:"url"`
	Method           string            `hcl:"method,optional"`
	Body             hcl.Expression    `hcl:"body,optional"`
	BodyRaw          string            `hcl:"body_raw,optional"`
	Headers          []string          `hcl:"headers,optional"`
	Cookies          map[string]string `hcl:"cookies,optional"`
	Query            map[string]string `hcl:"query,optional"`
	NoFollowRedirect bool              `hcl:"no_follow_redirect,optional"`
	PostHook         string            `hcl:"post_hook,optional"`
	// extras
	Label  string `hcl:"label,label"`
	Delay  string `hcl:"delay,optional"`
	Expect int    `hcl:"expect,optional"`
	// state
	Jar http.CookieJar
}

func (r *Request) Build() (*http.Request, error) {

	req, err := http.NewRequest(r.Method, r.URL, strings.NewReader(r.BodyRaw))
	if err != nil {
		return nil, err
	}
	for _, h := range r.Headers {
		hdrs := strings.Split(h, ":")
		req.Header.Add(hdrs[0], strings.TrimPrefix(h, hdrs[0]+":"))
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

	req.Header.Set("User-Agent", "rest-client/2.0")

	return req, nil
}

func (r *Request) SetDefaults(ctx *hcl.EvalContext) error {
	if r.Method == "" {
		r.Method = "GET"
	}
	return nil
}
func (r *Request) ParseBody(ctx *hcl.EvalContext) error {

	body, diags := r.Body.Value(ctx)
	if diags.HasErrors() {
		fmt.Println("errors", diags)
		return fmt.Errorf("%+v", diags.Errs())
	}
	simple := ctyjson.SimpleJSONValue{Value: body}
	jsonBytes, err := simple.MarshalJSON()
	if err != nil {
		return err
	}
	r.BodyRaw = string(jsonBytes)
	return nil
}

func (r Request) String() string {
	headers := ""
	for _, h := range r.Headers {
		headers += fmt.Sprintf("%s\n", h)
	}

	return fmt.Sprintf("%s %s\n%s\n%s", r.Method, r.URL, headers, r.BodyRaw)
}
