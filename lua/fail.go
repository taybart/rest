package restlua

import (
	lua "github.com/taybart/midnight"
)

// Failure is a script's fail(msg [, code]) or fail.usage([msg]). It stops the
// script, Exec returns it as is (no line of code, it was on purpose).
type Failure struct {
	Msg string
	// Code is the exit code, when it ends a cli (1 by default, 2 for
	// fail.usage)
	Code int
	// Usage is set by fail.usage, printed after Msg
	Usage string
}

func (f *Failure) Error() string {
	switch {
	case f.Usage == "":
		return f.Msg
	case f.Msg == "":
		return f.Usage
	}
	return f.Msg + "\n\n" + f.Usage
}

const failureTypeName = "rest.failure"

// RegisterFail sets the global fail: fail(msg [, code]) stops the script with
// msg. With a usage (the cli block's), fail.usage([msg]) stops it with msg
// and the usage.
func RegisterFail(l *lua.LState, usage string) {
	mt := l.NewTypeMetatable(failureTypeName)
	l.SetField(mt, "__tostring", l.NewFunction(func(l *lua.LState) int {
		if f, ok := l.CheckUserData(1).Value.(*Failure); ok {
			l.Push(lua.LString(f.Error()))
			return 1
		}
		l.Push(lua.LString("failure"))
		return 1
	}))

	raise := func(l *lua.LState, f *Failure) {
		ud := l.NewUserData()
		ud.Value = f
		l.SetMetatable(ud, mt)
		l.Error(ud, 0)
	}

	fail := l.NewTable()
	failMt := l.NewTable()
	// fail(msg [, code]), the table is the first argument
	l.SetField(failMt, "__call", l.NewFunction(func(l *lua.LState) int {
		raise(l, &Failure{
			Msg:  l.ToStringMeta(l.Get(2)).String(),
			Code: l.OptInt(3, 1),
		})
		return 0
	}))
	l.SetMetatable(fail, failMt)
	if usage != "" {
		l.SetField(fail, "usage", l.NewFunction(func(l *lua.LState) int {
			var msg string
			if l.GetTop() >= 1 && l.Get(1) != lua.LNil {
				msg = l.ToStringMeta(l.Get(1)).String()
			}
			raise(l, &Failure{Msg: msg, Code: l.OptInt(2, 2), Usage: usage})
			return 0
		}))
	}
	l.SetGlobal("fail", fail)
}

// asFailure is the Failure a script raised with fail, if it did
func asFailure(err error) (*Failure, bool) {
	apiErr, ok := err.(*lua.ApiError)
	if !ok {
		return nil, false
	}
	ud, ok := apiErr.Object.(*lua.LUserData)
	if !ok {
		return nil, false
	}
	f, ok := ud.Value.(*Failure)
	return f, ok
}
