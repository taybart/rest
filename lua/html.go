package restlua

import (
	"html"

	lua "github.com/taybart/go-erde"
)

func registerHTMLModule(l *lua.LState) {
	mod := l.SetFuncs(l.NewTable(), map[string]lua.LGFunction{
		"escape":   htmlEscape,
		"unescape": htmlUnescape,
	})
	l.SetGlobal("html", mod)
}

// htmlEscape escapes <, >, &, ' and " so the string is safe to embed in HTML.
func htmlEscape(l *lua.LState) int {
	l.Push(lua.LString(html.EscapeString(l.CheckString(1))))
	return 1
}

// htmlUnescape decodes all HTML entities (named, decimal and hex), e.g.
// "&amp;" -> "&", "&#39;" -> "'", "&#x27;" -> "'", "&hellip;" -> "…".
func htmlUnescape(l *lua.LState) int {
	l.Push(lua.LString(html.UnescapeString(l.CheckString(1))))
	return 1
}
