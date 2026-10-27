package restlua

import (
	"github.com/google/uuid"
	lua "github.com/yuin/gopher-lua"
)

func registerUUIDModule(l *lua.LState) {
	mod := l.SetFuncs(l.NewTable(), map[string]lua.LGFunction{
		"newv4":     uuidNewV4,
		"is_uuid":   uuidIsUUID,
		"is_uuidv4": uuidIsUUIDv4,
	})
	l.SetGlobal("uuid", mod)
}

func uuidNewV4(l *lua.LState) int {
	l.Push(lua.LString(uuid.NewString()))
	return 1
}

func uuidIsUUID(l *lua.LState) int {
	s := l.OptString(1, "")
	l.Push(lua.LBool(uuid.Validate(s) == nil))
	return 1
}

func uuidIsUUIDv4(l *lua.LState) int {
	s := l.OptString(1, "")
	u, err := uuid.Parse(s)
	l.Push(lua.LBool(err == nil && u.Version() == 4))
	return 1
}
