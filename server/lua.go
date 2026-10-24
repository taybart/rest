package server

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"

	restlua "github.com/taybart/rest/lua"
	"github.com/yuin/gluamapper"
	lua "github.com/yuin/gopher-lua"
)

/*
  TODO:
		- maybe better return type
		- maybe add some go funcs for directly writing response and such
*/

func populateGlobalObject(l *lua.LState, req *http.Request) error {
	var body []byte
	if req.Body != nil {
		var err error
		body, err = io.ReadAll(req.Body)
		req.Body.Close()
		if err != nil {
			return err
		}
		req.Body = io.NopCloser(bytes.NewReader(body))
	}

	reqMap := map[string]lua.LValue{
		"url":     lua.LString(req.URL.String()),
		"method":  lua.LString(req.Method),
		"query":   restlua.MakeLTableFromMapOfArr(l, req.URL.Query()),
		"headers": restlua.MakeLTableFromMapOfArr(l, req.Header),
		"body":    lua.LString(body),
	}
	reqTbl := restlua.MakeLTable(l, reqMap)
	restlua.SetRequestMetatable(l, reqTbl, req)

	table := restlua.MakeLTable(l, map[string]lua.LValue{
		"path": lua.LString(req.URL.Path),
		"req":  reqTbl,
		"ctx":  l.NewTable(),
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

func mapLuaResponse(l *lua.LState, luaRes *lua.LTable) (Response, error) {
	res := Response{
		Status: http.StatusOK,
	}
	var cookies []*http.Cookie
	if cookiesTbl, ok := l.GetField(luaRes, "cookies").(*lua.LTable); ok {
		cookies = restlua.ParseLuaCookies(cookiesTbl)
		l.SetField(luaRes, "cookies", lua.LNil)
	}
	if err := gluamapper.Map(luaRes, &res); err != nil {
		return res, err
	}
	res.Cookies = cookies
	return res, nil
}

func (s *Server) findMiddleware(name string) (*Middleware, bool) {
	for _, mw := range s.Config.Middlewares {
		if mw.Name == name {
			return mw, true
		}
	}
	return nil, false
}

func (s *Server) RunLuaPipeline(handler *Handler, req *http.Request, w http.ResponseWriter) (Response, bool, error) {
	l := lua.NewState()
	defer l.Close()

	res := Response{
		Status: http.StatusOK,
	}

	if err := restlua.RegisterModules(l); err != nil {
		return res, false, err
	}
	if err := populateGlobalObject(l, req); err != nil {
		return res, false, err
	}
	if err := s.luaHelpers(l); err != nil {
		return res, false, err
	}

	for _, mwName := range handler.Middleware {
		mw, ok := s.findMiddleware(mwName)
		if !ok {
			return res, false, fmt.Errorf("middleware %q not found", mwName)
		}
		l.SetTop(0)
		if err := execute(l, mw.Fn); err != nil {
			return res, false, err
		}
		if l.GetTop() > 0 && l.Get(-1) != lua.LNil {
			luaRes, ok := l.Get(-1).(*lua.LTable)
			if !ok {
				return res, false, fmt.Errorf("middleware %q must return a response table or nil", mwName)
			}
			mapped, err := mapLuaResponse(l, luaRes)
			return mapped, true, err
		}
	}

	if handler.Fn != "" {
		l.SetTop(0)
		if err := execute(l, handler.Fn); err != nil {
			return res, false, err
		}
		luaRes, ok := l.Get(-1).(*lua.LTable)
		if !ok {
			return res, false, errors.New("handler fn must return a table")
		}
		mapped, err := mapLuaResponse(l, luaRes)
		return mapped, true, err
	}

	return res, false, nil
}

func (s *Server) RunLuaHandler(handler string, req *http.Request, w http.ResponseWriter) (Response, error) {
	res, _, err := s.RunLuaPipeline(&Handler{Fn: handler}, req, w)
	return res, err
}
