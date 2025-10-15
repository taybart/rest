package server

import (
	"errors"
	"io"
	"net/http"

	restlua "github.com/taybart/rest/lua"
	lua "github.com/yuin/gopher-lua"
)

/*
  TODO:
		- Cookies
		- maybe better return type
		- maybe add some go funcs for directly writing response and such
*/

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

	// u, err := url.Parse(req.URL.String())
	// if err != nil {
	// 	return err
	// }
	// cookieMap := map[string]lua.LValue{}
	// if jar != nil {
	// 	for _, cookie := range jar.Cookies(u) {
	// 		cookieMap[cookie.Name] = lua.LString(cookie.Value)
	// 	}
	// }

	// resTbl := makeLTable(l, map[string]lua.LValue{
	// 	"status":  lua.LNumber(res.StatusCode),
	// 	"headers": makeLTableFromMapOfArr(l, res.Header),
	// 	"body":    lua.LString(string(body)),
	// 	"cookies": makeLTable(l, cookieMap),
	// })

	table := restlua.MakeLTable(l, map[string]lua.LValue{
		"path": lua.LString(req.URL.Path),
		"req":  reqTbl,
		// "res":     resTbl,
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
		return err
	}

	return cleanError
}

func (s *Server) RunLuaHandler(handler string, req *http.Request, w http.ResponseWriter) (int, string, error) {

	l := lua.NewState()
	defer l.Close()

	if err := restlua.RegisterModules(l); err != nil {
		return -1, "", err
	}
	if err := populateGlobalObject(l, req); err != nil {
		return -1, "", err
	}

	if err := execute(l, handler); err != nil {
		return -1, "", err
	}

	body := l.Get(-1)
	// if body.String() != "nil" {
	// fmt.Println("ret", ret, "-2", l.Get(-2))
	status := lua.LVAsNumber(l.Get(-2))
	return int(status), body.String(), nil
	// }
	// return -1, "", nil
}
