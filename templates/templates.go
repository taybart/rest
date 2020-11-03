package templates

import (
	"text/template"
)

// RequestTemplate : for building onther language requests
type RequestTemplate struct {
	ClientStr   string
	FunctionStr string
	RequestStr  string
	FuncMap     template.FuncMap
	Client      *template.Template
	Function    *template.Template
	Request     *template.Template
}

func (r *RequestTemplate) build() *RequestTemplate {
	r.Client = template.Must(template.New("Client").Funcs(stdFns).Funcs(r.FuncMap).Parse(r.ClientStr))
	r.Function = template.Must(template.New("Function").Funcs(stdFns).Funcs(r.FuncMap).Parse(r.FunctionStr))
	r.Request = template.Must(template.New("Request").Funcs(stdFns).Funcs(r.FuncMap).Parse(r.RequestStr))
	return r
}

func Get(name string) *RequestTemplate {
	switch name {
	case "go":
		return Go.build()
	case "curl":
		return Curl.build()
	case "javascript", "js":
		return Javascript.build()
	case "node":
		return Node.build()
	}
	return nil
}
