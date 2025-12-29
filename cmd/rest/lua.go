package main

import (
	"errors"
	"net/http"

	"github.com/taybart/rest"
	restlua "github.com/taybart/rest/lua"
	"github.com/yuin/gluamapper"
	lua "github.com/yuin/gopher-lua"
)

func populateGlobalObject(l *lua.LState, file *rest.Rest) error {
	table := restlua.MakeLTable(l, map[string]lua.LValue{
		// TODO: add "label" function
		"label": lua.LString(""),
	})
	doLabel := func(l *lua.LState) int {
		label := l.ToString(1) /* get argument */
		file.RunLabel(label)
		l.Push(lua.LString(v))
		return 1 /* number of results */
	}
	l.SetGlobal("rest", restlua.MakeLTable(l, map[string]lua.LValue{
		"path_value": l.NewFunction(doLabel),
	}))
	// l.SetGlobal("rest", table)

	return nil
}

func execute(l *lua.LState, code string) error {
	var cleanError error

	l.SetGlobal("fail", l.NewFunction(func(L *lua.LState) int {
		cleanError = errors.New(L.CheckString(1))
		for range 10 {
			L.Push(lua.LNil)
		}
		return 10
	}))

	if err := l.DoString(code); err != nil {
		return restlua.FmtError(code, err)
	}

	return cleanError
}

func luaHelpers(l *lua.LState, req *http.Request) error {
	kvCacheLoader(l)
	// l.PreloadModule("kv", kvCacheLoader)

	pathValue := func(l *lua.LState) int {
		id := l.ToString(1) /* get argument */
		v := req.PathValue(id)
		l.Push(lua.LString(v))
		return 1 /* number of results */
	}
	l.SetGlobal("s", restlua.MakeLTable(l, map[string]lua.LValue{
		"path_value": l.NewFunction(pathValue),
	}))
	return nil
}

func (s *Server) RunLuaHandler(handler string, req *http.Request, w http.ResponseWriter) (Response, error) {

	l := lua.NewState()
	defer l.Close()

	res := Response{
		Status: http.StatusOK,
	}

	if err := restlua.RegisterModules(l); err != nil {
		return res, err
	}
	if err := populateGlobalObject(l, req); err != nil {
		return res, err
	}
	if err := s.luaHelpers(l, req); err != nil {
		return res, err
	}
	if err := execute(l, handler); err != nil {
		return res, err
	}

	luaRes := l.Get(-1)
	err := gluamapper.Map(luaRes.(*lua.LTable), &res)
	if err != nil {
		return res, err
	}
	return res, nil
}
