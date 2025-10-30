package request

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"strings"

	"github.com/hashicorp/hcl/v2"
)

type Expect struct {
	Status  int               `hcl:"status,optional"`
	Headers map[string]string `hcl:"headers,optional"`
	Body    string
	BodyHCL hcl.Expression `hcl:"body,optional"`
}

type Request struct {
	// hcl
	Label string `hcl:"label,label"`
	// block body
	URL         string            `hcl:"url,optional"`
	Method      string            `hcl:"method,optional"`
	BodyHCL     hcl.Expression    `hcl:"body,optional"`
	BasicAuth   string            `hcl:"basic_auth,optional"`
	BearerToken string            `hcl:"bearer_token,optional"`
	Headers     map[string]string `hcl:"headers,optional"`
	Cookies     map[string]string `hcl:"cookies,optional"`
	Query       map[string]string `hcl:"query,optional"`
	PostHook    string            `hcl:"post_hook,optional"`
	CopyFrom    string            `hcl:"copy_from,optional"`
	// extras
	Expect       *Expect `hcl:"expect,block"`
	ExpectStatus int     `hcl:"expect,optional"`
	Delay        string  `hcl:"delay,optional"`
	Skip         bool    `hcl:"skip,optional"`

	// ...rest
	Remain hcl.Expression `hcl:"remain,optional"`

	// parsed values
	UserAgent string
	Body      string

	Built *http.Request

	Block *hcl.Body
}

func (r *Request) Build() (*http.Request, error) {
	if r.Built != nil {
		return r.Built, nil
	}

	body := r.Body
	if r.Headers["Content-Type"] == "application/json" {
		var buf bytes.Buffer
		err := json.Compact(&buf, []byte(r.Body))
		if err != nil {
			return nil, err
		}
		body = buf.String()
	}
	req, err := http.NewRequest(r.Method, r.URL, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	for k, v := range r.Headers {
		req.Header.Add(k, v)
	}
	if r.BasicAuth != "" {
		ba := strings.Split(r.BasicAuth, ":")
		if len(ba) != 2 {
			return nil, fmt.Errorf("malformed basic auth value should be -> user:password")
		}
		req.SetBasicAuth(ba[0], ba[1])
	}

	if r.BearerToken != "" {
		req.Header.Add("Authorization", "Bearer "+r.BearerToken)
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

	r.Built = req
	return req, nil
}

func (r *Request) SetDefaults(ctx *hcl.EvalContext) error {
	if r.Method == "" {
		r.Method = "GET"
	}
	return nil
}

func (r Request) String() string {
	headers := ""
	for _, h := range r.Headers {
		headers += fmt.Sprintf("%s\n", h)
	}

	return fmt.Sprintf("%s %s\n%s\n%s", r.Method, r.URL, headers, r.Body)
}

func (r Request) Equal(cmp Request) bool {
	if r.Label != cmp.Label {
		fmt.Println("labels don't match")
		return false
	}
	if r.Method != cmp.Method {
		fmt.Println("methods don't match")
		return false
	}
	if r.URL != cmp.URL {
		fmt.Println("urls don't match")
		return false
	}
	if r.Body != cmp.Body {
		fmt.Println("bodies don't match")
		return false
	}
	if !maps.Equal(r.Headers, cmp.Headers) {
		fmt.Println("headers don't match")
		return false
	}
	if !maps.Equal(r.Cookies, cmp.Cookies) {
		fmt.Println("cookies don't match")
		return false
	}
	if !maps.Equal(r.Query, cmp.Query) {
		fmt.Println("queries don't match")

		return false
	}
	if r.BearerToken != cmp.BearerToken {
		fmt.Println("bearer tokens don't match")
		return false
	}
	if r.BasicAuth != cmp.BasicAuth {
		fmt.Println("basic auths don't match")
		return false
	}
	if r.UserAgent != cmp.UserAgent {
		fmt.Println("user agents don't match")

		return false
	}
	if r.PostHook != cmp.PostHook {
		fmt.Println("post hooks don't match")
		return false
	}
	if r.Expect != cmp.Expect {
		fmt.Println("expect values don't match")
		return false
	}
	return true
}

func (r *Request) CombineFrom(from Request) {
	if r.Method == "GET" {
		r.Method = from.Method
	}
	if r.URL == "" {
		r.URL = from.URL
	}
	if r.Body == "" {
		r.Body = from.Body
	}

	r.Headers = combineMap(from.Headers, r.Headers)
	r.Cookies = combineMap(from.Cookies, r.Cookies)
	r.Query = combineMap(from.Query, r.Query)

	if r.BearerToken == "" {
		r.BearerToken = from.BearerToken
	}
	if r.BasicAuth == "" {
		r.BasicAuth = from.BasicAuth
	}
	if r.UserAgent == "" {
		r.UserAgent = from.UserAgent
	}
	if r.PostHook == "" {
		r.PostHook = from.PostHook
	}
	if from.Expect != nil {
		if r.Expect == nil {
			r.Expect = &Expect{}
		}
		if r.Expect.Status == 0 {
			r.Expect.Status = from.Expect.Status
			r.Expect.Body = from.Expect.Body
			r.Expect.Headers = from.Expect.Headers
		}
	}
	if r.ExpectStatus == 0 {
		r.ExpectStatus = from.ExpectStatus
	}
}

// combineMap: combines in a weird way for the CombineFrom method
// we want to overwrite second with first values if they exist
func combineMap(first, second map[string]string) map[string]string {
	if second == nil {
		return first
	}
	dst := maps.Clone(second)
	maps.Copy(dst, first)
	return dst
}
