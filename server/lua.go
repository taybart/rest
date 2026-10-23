package server

import (
	"errors"
	"io"
	"net/http"

	restlua "github.com/taybart/rest/lua"
	"github.com/yuin/gluamapper"
	lua "github.com/yuin/gopher-lua"
)

/*
  TODO:
		- Cookies
		- maybe better return type
		- maybe add some go funcs for directly writing response and such
*/

const reqUserDataKey = "__req"

func checkRequest(l *lua.LState) *http.Request {
	self := l.CheckTable(1)
	mt, ok := l.GetMetatable(self).(*lua.LTable)
	if !ok {
		l.ArgError(1, "expected rest.req table (use ':' syntax, e.g. rest.req:path_value(...))")
		return nil
	}
	ud, ok := l.GetField(mt, reqUserDataKey).(*lua.LUserData)
	if !ok {
		l.ArgError(1, "missing underlying http.Request")
		return nil
	}
	req, ok := ud.Value.(*http.Request)
	if !ok {
		l.ArgError(1, "invalid underlying http.Request")
		return nil
	}
	return req
}

func reqPathValue(l *lua.LState) int {
	req := checkRequest(l)
	id := l.CheckString(2)
	l.Push(lua.LString(req.PathValue(id)))
	return 1
}

func populateGlobalObject(l *lua.LState, req *http.Request) error {
	defer req.Body.Close()
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return err
	}

	reqMap := map[string]lua.LValue{
		"url":     lua.LString(req.URL.String()),
		"method":  lua.LString(req.Method),
		"query":   restlua.MakeLTableFromMapOfArr(l, req.URL.Query()),
		"headers": restlua.MakeLTableFromMapOfArr(l, req.Header),
		"body":    lua.LString(body),
	}
	reqTbl := restlua.MakeLTable(l, reqMap)

	ud := l.NewUserData()
	ud.Value = req
	mt := l.NewTable()
	l.SetField(mt, reqUserDataKey, ud)
	l.SetField(mt, "__index", l.SetFuncs(l.NewTable(), map[string]lua.LGFunction{
		"path_value": reqPathValue,
	}))
	l.SetMetatable(reqTbl, mt)

	table := restlua.MakeLTable(l, map[string]lua.LValue{
		"path": lua.LString(req.URL.Path),
		"req":  reqTbl,
	})
	l.SetGlobal("rest", table)
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

// this is goofy and probably not necessary
var kv map[string]lua.LValue

func kvCacheLoader(l *lua.LState) int {
	if len(kv) == 0 {
		kv = make(map[string]lua.LValue)
	}
	get := func(l *lua.LState) int {
		key := l.ToString(1)
		if v := kv[key]; v != nil {
			l.Push(v)
		} else {
			l.Push(lua.LNil)
		}
		return 1
	}
	set := func(l *lua.LState) int {
		key := l.ToString(1)
		value := l.Get(2)
		kv[key] = value
		return 0
	}
	l.SetGlobal("kv", restlua.MakeLTable(l, map[string]lua.LValue{
		"get": l.NewFunction(get),
		"set": l.NewFunction(set),
	}))
	// mod := l.SetFuncs(l.NewTable(), map[string]lua.LGFunction{
	// 	"get": get,
	// 	"set": set,
	// })
	// l.Push(mod)
	return 1
}

func (s *Server) luaHelpers(l *lua.LState) error {
	kvCacheLoader(l)
	// l.PreloadModule("kv", kvCacheLoader)
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
	if err := s.luaHelpers(l); err != nil {
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
