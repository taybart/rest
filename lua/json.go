package restlua

import (
	"encoding/json"

	lua "github.com/taybart/midnight"
	"github.com/taybart/midnight/luaconv"
)

func registerJSONModule(l *lua.LState) {
	mod := l.SetFuncs(l.NewTable(), map[string]lua.LGFunction{
		"encode": jsonEncode,
		"decode": jsonDecode,
	})
	l.SetGlobal("json", mod)
}

func jsonEncode(l *lua.LState) int {
	b, err := luaconv.EncodeJSON(l.Get(1))
	if err != nil {
		l.RaiseError("json.encode: %v", err)
		return 0
	}
	l.Push(lua.LString(b))
	return 1
}

func jsonDecode(l *lua.LState) int {
	arg := l.Get(1)
	str, ok := arg.(lua.LString)
	if !ok {
		l.RaiseError("json.decode: expected argument of type string, got %s", arg.Type().String())
		return 0
	}
	if str == "" {
		l.RaiseError("json.decode: cannot decode empty string")
		return 0
	}
	var out any
	if err := json.Unmarshal([]byte(str), &out); err != nil {
		l.RaiseError("json.decode: %v", err)
		return 0
	}
	l.Push(luaconv.FromGo(l, out))
	return 1
}
