package restlua

import (
	"encoding/base64"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// Base64Options configures base64 encoding and decoding.
type Base64Options struct {
	URL bool
	Raw bool
}

func registerBase64Module(l *lua.LState) {
	mod := l.SetFuncs(l.NewTable(), map[string]lua.LGFunction{
		"encode": luaBase64Encode,
		"decode": luaBase64Decode,
	})
	l.SetGlobal("base64", mod)
}

func parseLuaBase64Opts(l *lua.LState, idx int) Base64Options {
	var opts Base64Options
	if l.GetTop() < idx {
		return opts
	}
	tbl, ok := l.Get(idx).(*lua.LTable)
	if !ok {
		return opts
	}
	if v := tbl.RawGetString("url"); v != lua.LNil {
		opts.URL = lua.LVAsBool(v)
	} else if v := tbl.RawGetString("url_safe"); v != lua.LNil {
		opts.URL = lua.LVAsBool(v)
	}
	if v := tbl.RawGetString("raw"); v != lua.LNil {
		opts.Raw = lua.LVAsBool(v)
	} else if v := tbl.RawGetString("pad"); v != lua.LNil {
		opts.Raw = !lua.LVAsBool(v)
	} else if v := tbl.RawGetString("padding"); v != lua.LNil {
		opts.Raw = !lua.LVAsBool(v)
	}
	return opts
}

// EncodeBase64 encodes data using standard or URL-safe base64.
func EncodeBase64(data string, opts Base64Options) string {
	switch {
	case opts.URL && opts.Raw:
		return base64.RawURLEncoding.EncodeToString([]byte(data))
	case opts.URL:
		return base64.URLEncoding.EncodeToString([]byte(data))
	case opts.Raw:
		return base64.RawStdEncoding.EncodeToString([]byte(data))
	default:
		return base64.StdEncoding.EncodeToString([]byte(data))
	}
}

// DecodeBase64 decodes standard or URL-safe (padded or unpadded) base64 strings.
func DecodeBase64(data string, opts Base64Options) (string, error) {
	cleaned := strings.TrimSpace(data)
	cleaned = strings.TrimRight(cleaned, "=")

	enc := base64.RawStdEncoding
	if opts.URL || strings.ContainsAny(cleaned, "-_") {
		enc = base64.RawURLEncoding
	}

	b, err := enc.DecodeString(cleaned)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func luaBase64Encode(l *lua.LState) int {
	data := l.CheckString(1)
	opts := parseLuaBase64Opts(l, 2)
	l.Push(lua.LString(EncodeBase64(data, opts)))
	return 1
}

func luaBase64Decode(l *lua.LState) int {
	data := l.CheckString(1)
	opts := parseLuaBase64Opts(l, 2)
	decoded, err := DecodeBase64(data, opts)
	if err != nil {
		l.RaiseError("base64.decode: %v", err)
		return 0
	}
	l.Push(lua.LString(decoded))
	return 1
}
