package restlua

import (
	"net/url"

	lua "github.com/taybart/midnight"
)

func registerURLModule(l *lua.LState) {
	mod := l.SetFuncs(l.NewTable(), map[string]lua.LGFunction{
		"escape":        urlQueryEscape,
		"unescape":      urlQueryUnescape,
		"path_escape":   urlPathEscape,
		"path_unescape": urlPathUnescape,
	})
	l.SetGlobal("url", mod)
}

// urlQueryEscape escapes a string for use as a query parameter value (space -> "+").
func urlQueryEscape(l *lua.LState) int {
	l.Push(lua.LString(url.QueryEscape(l.CheckString(1))))
	return 1
}

// urlQueryUnescape reverses url.escape ("+" -> space).
func urlQueryUnescape(l *lua.LState) int {
	s, err := url.QueryUnescape(l.CheckString(1))
	if err != nil {
		l.RaiseError("url.unescape: %v", err)
		return 0
	}
	l.Push(lua.LString(s))
	return 1
}

// urlPathEscape escapes a string for use as a single path segment (space -> "%20", "/" -> "%2F").
func urlPathEscape(l *lua.LState) int {
	l.Push(lua.LString(url.PathEscape(l.CheckString(1))))
	return 1
}

// urlPathUnescape reverses url.path_escape ("+" is left as-is).
func urlPathUnescape(l *lua.LState) int {
	s, err := url.PathUnescape(l.CheckString(1))
	if err != nil {
		l.RaiseError("url.path_unescape: %v", err)
		return 0
	}
	l.Push(lua.LString(s))
	return 1
}
