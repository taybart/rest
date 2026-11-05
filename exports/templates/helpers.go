package templates

import (
	"bytes"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"unicode"
)

var funcs = template.FuncMap{
	"shquote":    shquote,
	"gostr":      gostr,
	"goraw":      goraw,
	"jsstr":      jsstr,
	"ident":      ident,
	"cookies":    cookieHeader,
	"jsonindent": jsonindent,
}

// shquote quotes s for a posix shell
func shquote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// gostr returns s as an interpreted go string literal
func gostr(s string) string {
	return strconv.Quote(s)
}

// goraw prefers a raw go string literal for readability, falling back to an
// interpreted literal when s can't be represented raw
func goraw(s string) string {
	if strings.ContainsAny(s, "`\r") {
		return strconv.Quote(s)
	}
	return "`" + s + "`"
}

// jsstr returns s as a javascript string literal
func jsstr(s string) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s) // strings always encode
	return strings.TrimSpace(buf.String())
}

// ident turns a label into a lowerCamelCase identifier that is valid in go
// and javascript, "get-thing 2" -> "getThing2"
func ident(label string) string {
	words := strings.FieldsFunc(label, func(r rune) bool {
		return !(r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)))
	})
	for i, w := range words {
		if i == 0 {
			words[i] = strings.ToLower(w[:1]) + w[1:]
		} else {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	id := strings.Join(words, "")
	if id == "" || unicode.IsDigit(rune(id[0])) {
		id = "request" + id
	}
	return id
}

// cookieHeader renders cookies as a Cookie header value, sorted by name
func cookieHeader(cookies map[string]string) string {
	names := make([]string, 0, len(cookies))
	for n := range cookies {
		names = append(names, n)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, n := range names {
		parts = append(parts, n+"="+cookies[n])
	}
	return strings.Join(parts, "; ")
}

// jsonindent pretty prints json, continuation lines are prefixed so the
// output can be dropped into already indented code
func jsonindent(prefix, s string) string {
	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(s), prefix, "  "); err != nil {
		return s
	}
	return buf.String()
}
