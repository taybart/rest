package request

import (
	"embed"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	lua "github.com/yuin/gopher-lua"
)

//go:embed lua/*
var library embed.FS

var sharedState *lua.LTable

func loadModule(l *lua.LState, name, filename string) error {
	code, err := library.ReadFile("lua/" + filename)
	if err != nil {
		return err
	}
	if err := l.DoString(string(code)); err != nil {
		return fmt.Errorf("failed to load module %s: %w", name, err)
	}
	module := l.Get(-1)
	l.Pop(1)
	if module.Type() != lua.LTTable {
		return fmt.Errorf("module %s did not return a table", name)
	}
	l.SetGlobal(name, module)
	return nil
}

func registerModules(l *lua.LState) error {
	libs := map[string]string{
		"base64":  "base64.lua",
		"json":    "json.lua",
		"inspect": "inspect.lua",
	}
	for name, filename := range libs {
		if err := loadModule(l, name, filename); err != nil {
			return err
		}
	}
	return nil
}

func makeLTableFromMap[M ~map[string][]string](l *lua.LState, inMap M) *lua.LTable {
	tbl := l.NewTable()
	for k, v := range inMap {
		l.SetField(tbl, k, lua.LString(v[0]))
	}
	return tbl
}

func makeLTable(l *lua.LState, tblMap map[string]lua.LValue) *lua.LTable {
	tbl := l.NewTable()
	for k, v := range tblMap {
		l.SetField(tbl, k, v)
	}
	return tbl
}

func populateGlobalObject(l *lua.LState, req *Request, res *http.Response, jar http.CookieJar) error {
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}

	reqMap := map[string]lua.LValue{
		"url":     lua.LString(res.Request.URL.String()),
		"method":  lua.LString(res.Request.Method),
		"query":   makeLTableFromMap(l, res.Request.URL.Query()),
		"headers": makeLTableFromMap(l, res.Request.Header),
		"body":    lua.LString(req.Body),
	}
	if req.Expect != 0 {
		reqMap["expect"] = lua.LNumber(req.Expect)
	}
	reqTbl := makeLTable(l, reqMap)

	u, err := url.Parse(res.Request.URL.String())
	if err != nil {
		return err
	}
	cookieMap := map[string]lua.LValue{}
	if jar != nil {
		for _, cookie := range jar.Cookies(u) {
			cookieMap[cookie.Name] = lua.LString(cookie.Value)
		}
	}

	resTbl := makeLTable(l, map[string]lua.LValue{
		"status":  lua.LNumber(res.StatusCode),
		"headers": makeLTableFromMap(l, res.Header),
		"body":    lua.LString(string(body)),
		"cookies": makeLTable(l, cookieMap),
	})

	if sharedState == nil {
		sharedState = l.NewTable()
	}

	table := makeLTable(l, map[string]lua.LValue{
		"label":  lua.LString(req.Label),
		"req":    reqTbl,
		"res":    resTbl,
		"shared": sharedState,
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

func (r *Request) RunPostHook(res *http.Response, jar http.CookieJar) (string, error) {

	l := lua.NewState()
	defer l.Close()

	if err := registerModules(l); err != nil {
		return "", err
	}
	if err := populateGlobalObject(l, r, res, jar); err != nil {
		return "", err
	}

	if err := execute(l, r.PostHook); err != nil {
		return "", err
	}

	if ret := l.Get(-1); ret.String() != "nil" {
		return ret.String(), nil
	}
	return "", nil
}
