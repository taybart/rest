// Package templates renders single requests as code snippets in other
// languages (curl, go, js, ...) or with a user supplied template file
package templates

import (
	"bytes"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"
	"text/template"
)

//go:embed builtin/*.tmpl
var builtinFS embed.FS

type Expect struct {
	Status  int
	Headers map[string]string
	Body    string
}

// Auth is set from basic_auth or bearer_token, Type is "", "basic" or "bearer"
type Auth struct {
	Type     string
	User     string
	Password string
	Token    string
}

// Request is the data passed to a template, all values are fully evaluated.
//
// The fields are the separate parts of the request so templates can use
// native features of their target (curl -u, python params=, ...). The URL and
// Headers methods combine them into exactly what rest would send, for
// templates that don't care.
type Request struct {
	Label  string
	Method string
	// BaseURL is the url without a query string
	BaseURL string
	// Query holds params written inline in the url merged with the query
	// block (the block wins on conflicts)
	Query url.Values
	// RequestHeaders are the headers as written in the rest file, without
	// Auth or UserAgent
	RequestHeaders map[string]string
	Auth           Auth
	// UserAgent is only set when configured to something other than the
	// default
	UserAgent string
	Cookies   map[string]string
	Body      string
	Expect    Expect
}

// URL is BaseURL with Query encoded onto it
func (r Request) URL() string {
	if len(r.Query) == 0 {
		return r.BaseURL
	}
	return r.BaseURL + "?" + r.Query.Encode()
}

// Headers are RequestHeaders plus Authorization and User-Agent
func (r Request) Headers() map[string]string {
	h := make(map[string]string, len(r.RequestHeaders)+2)
	maps.Copy(h, r.RequestHeaders)
	switch r.Auth.Type {
	case "basic":
		h["Authorization"] = "Basic " + base64.StdEncoding.EncodeToString([]byte(r.Auth.User+":"+r.Auth.Password))
	case "bearer":
		h["Authorization"] = "Bearer " + r.Auth.Token
	}
	if r.UserAgent != "" {
		h["User-Agent"] = r.UserAgent
	}
	return h
}

// IsJSON reports whether the body is sent as json
func (r Request) IsJSON() bool {
	ct := strings.ToLower(r.RequestHeaders["Content-Type"])
	return strings.HasPrefix(ct, "application/json") && json.Valid([]byte(r.Body))
}

// Template renders requests one at a time
type Template struct {
	Name string
	tmpl *template.Template
}

func parse(name, src string) (*Template, error) {
	t, err := template.New(name).Funcs(funcs).Option("missingkey=zero").Parse(src)
	if err != nil {
		return nil, fmt.Errorf("parsing template %s: %w", name, err)
	}
	return &Template{Name: name, tmpl: t}, nil
}

// Builtins returns the names of all built-in templates
func Builtins() []string {
	entries, _ := builtinFS.ReadDir("builtin")
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, strings.TrimSuffix(e.Name(), ".tmpl"))
	}
	sort.Strings(names)
	return names
}

// Get returns a built-in template by name, or loads name as a template file
// if it is not a built-in
func Get(name string) (*Template, error) {
	src, err := builtinFS.ReadFile(path.Join("builtin", name+".tmpl"))
	if err == nil {
		return parse(name, string(src))
	}
	if _, statErr := os.Stat(name); statErr == nil {
		return Load(name)
	}
	return nil, fmt.Errorf("export %q is not a built-in (%s) or a template file",
		name, strings.Join(Builtins(), ", "))
}

// Load parses a go text/template file, it receives a Request as its data
func Load(filename string) (*Template, error) {
	src, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return parse(filename, string(src))
}

// Execute renders each request, separated by a blank line
func (t *Template) Execute(w io.Writer, reqs []Request) error {
	for i, req := range reqs {
		var buf bytes.Buffer
		if err := t.tmpl.Execute(&buf, req); err != nil {
			return fmt.Errorf("rendering %q: %w", req.Label, err)
		}
		sep := "\n"
		if i < len(reqs)-1 {
			sep = "\n\n"
		}
		if _, err := io.WriteString(w, strings.TrimSpace(buf.String())+sep); err != nil {
			return err
		}
	}
	return nil
}
