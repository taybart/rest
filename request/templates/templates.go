package templates

import (
	"bytes"
	"go/format"
	"io"
	"net/url"
	"strings"
	"text/template"
)

var (
	exports = map[string]RequestTemplate{
		"go":   Go,
		"curl": Curl,
		"js":   Javascript,
	}
)

func NewVanilla(tmpl string) *template.Template {
	return template.Must(template.New("Client").Funcs(stdFns).Parse(tmpl))
}

type Request struct {
	URL       string
	Method    string
	Body      string
	Headers   map[string]string
	Cookies   map[string]string
	Query     map[string]string
	PostHook  string
	UserAgent string

	// extras
	Label  string
	Delay  string
	Expect int

	// metadata
	BlockIndex int
}

//	func (r *Request) Build() error {
//		return nil
//	}

func (r Request) URLWithQuery() string {
	if len(r.Query) == 0 {
		return r.URL
	}

	u, err := url.Parse(r.URL)
	if err != nil {
		return r.URL
	}

	q := u.Query()
	for key, value := range r.Query {
		q.Set(key, value)
	}
	u.RawQuery = q.Encode()

	return u.String()
}

// RequestTemplate : for building onther language requests
type RequestTemplate struct {
	Name        string
	ClientStr   string
	FunctionStr string
	RequestStr  string
	FuncMap     template.FuncMap
	Client      *template.Template
	Function    *template.Template
	Request     *template.Template
}

func (r *RequestTemplate) Build() *RequestTemplate {
	client := r.ClientStr
	if client == "" {
		client = "{{.Code}}"
	}
	r.Client = template.Must(template.New("Client").Funcs(stdFns).Funcs(r.FuncMap).Parse(client))
	function := r.FunctionStr
	if function == "" {
		function = "{{.Code}}\n"
	}
	r.Function = template.Must(template.New("Function").Funcs(stdFns).Funcs(r.FuncMap).Parse(function))
	r.Request = template.Must(template.New("Request").Funcs(stdFns).Funcs(r.FuncMap).Parse(r.RequestStr))
	return r
}

func (r *RequestTemplate) ExecuteClient(wr io.Writer, reqs map[string]Request) error {
	// req.Build()
	var code bytes.Buffer
	for label, req := range reqs {
		var reqBuf bytes.Buffer
		err := r.Execute(&reqBuf, req)
		if err != nil {
			return err
		}
		r.Function.Execute(&code, map[string]any{
			"Code":  strings.TrimSpace(reqBuf.String() + "\n"),
			"Label": label,
		})
	}

	var client bytes.Buffer
	r.Client.Execute(&client, map[string]any{
		"Code": code.String(),
	})
	if r.Name == "go" {
		formatted, err := format.Source(client.Bytes())
		if err != nil {
			return err
		}
		_, err = wr.Write(formatted)
		return err
	}
	_, err := wr.Write(client.Bytes())
	return err
}
func (r *RequestTemplate) Execute(wr io.Writer, req Request) error {
	// req.Build()
	var buf bytes.Buffer
	err := r.Request.Execute(&buf, req)
	if err != nil {
		return err
	}
	// result := strings.TrimSpace(buf.String())
	_, err = wr.Write(buf.Bytes())
	return err
}

func Exports() []string {
	ret := []string{}
	for k := range exports {
		ret = append(ret, k)
	}
	return ret
}

func Get(name string) *RequestTemplate {
	if t, ok := exports[name]; ok {
		return t.Build()
	}
	return nil
}
