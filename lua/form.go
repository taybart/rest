package restlua

import (
	"net/url"

	lua "github.com/taybart/go-erde"
)

func registerFormModule(l *lua.LState) {
	mod := l.SetFuncs(l.NewTable(), map[string]lua.LGFunction{
		"encode": formEncode,
		"decode": formDecode,
	})
	l.SetGlobal("form", mod)
}

func formEncode(l *lua.LState) int {
	tbl := l.CheckTable(1)
	vals := url.Values{}
	tbl.ForEach(func(k, v lua.LValue) {
		key := k.String()
		switch val := v.(type) {
		case *lua.LTable:
			val.ForEach(func(_, item lua.LValue) {
				if item != lua.LNil {
					vals.Add(key, item.String())
				}
			})
		case *lua.LNilType:
			// skip nil values
		default:
			vals.Set(key, val.String())
		}
	})
	l.Push(lua.LString(vals.Encode()))
	return 1
}

func formDecode(l *lua.LState) int {
	raw := l.CheckString(1)
	vals, err := url.ParseQuery(raw)
	if err != nil {
		l.RaiseError("form.decode: %v", err)
		return 0
	}
	tbl := l.NewTable()
	for k, v := range vals {
		switch len(v) {
		case 0:
			tbl.RawSetString(k, lua.LString(""))
		case 1:
			tbl.RawSetString(k, lua.LString(v[0]))
		default:
			arr := l.NewTable()
			for i, item := range v {
				arr.RawSetInt(i+1, lua.LString(item))
			}
			tbl.RawSetString(k, arr)
		}
	}
	l.Push(tbl)
	return 1
}
