package restlua

import (
	gonanoid "github.com/matoous/go-nanoid/v2"
	lua "github.com/yuin/gopher-lua"
)

func IsNanoID(s string, size int) bool {
	if size <= 0 {
		size = 21
	}
	if len(s) != size {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

func registerNanoIDModule(l *lua.LState) {
	mod := l.SetFuncs(l.NewTable(), map[string]lua.LGFunction{
		"new":       nanoidNew,
		"is_nanoid": nanoidIsNanoID,
	})
	mt := l.NewTable()
	l.SetField(mt, "__call", l.NewFunction(func(l *lua.LState) int {
		l.Remove(1) // remove table self
		return nanoidNew(l)
	}))
	l.SetMetatable(mod, mt)
	l.SetGlobal("nanoid", mod)
}

func nanoidNew(l *lua.LState) int {
	size := l.OptInt(1, 21)
	alphabet := l.OptString(2, "")

	var (
		id  string
		err error
	)
	if alphabet != "" {
		id, err = gonanoid.Generate(alphabet, size)
	} else {
		id, err = gonanoid.New(size)
	}
	if err != nil {
		l.RaiseError("nanoid.new: %v", err)
		return 0
	}
	l.Push(lua.LString(id))
	return 1
}

func nanoidIsNanoID(l *lua.LState) int {
	s := l.OptString(1, "")
	size := l.OptInt(2, 21)
	l.Push(lua.LBool(IsNanoID(s, size)))
	return 1
}
