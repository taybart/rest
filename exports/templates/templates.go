// Package templates is used for generating clients from requests
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

type Expect struct {
	Status  int
	Headers map[string]string
	Body    string
}

type Request struct {
	URL       string
	Method    string
	Body      string
	Headers   map[string]string
	Cookies   map[string]string
	Query     map[string]string
	After     string
	UserAgent string

	// extras
	Label  string
	Delay  string
	Expect Expect

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

func (r *RequestTemplate) Execute(wr io.Writer, filename string, reqs map[string]Request) error {
	// req.Build()
	var code bytes.Buffer
	for label, req := range reqs {
		var reqBuf bytes.Buffer
		err := r.executeRequest(&reqBuf, req)
		if err != nil {
			return err
		}
		r.Function.Execute(&code, map[string]any{
			"Code":  strings.TrimSpace(reqBuf.String() + "\n"),
			"Label": label,
		})
	}

	labels := []string{}
	for _, req := range reqs {
		labels = append(labels, req.Label)
	}

	var client bytes.Buffer
	r.Client.Execute(&client, map[string]any{
		"Filename": filename,
		"Code":     code.String(),
		"Labels":   labels,
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
func (r *RequestTemplate) executeRequest(wr io.Writer, req Request) error {
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

// Exports : returns a list of all available templates
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
