package request

import (
	"embed"
	"fmt"
	"io"
	"net/http"

	lua "github.com/yuin/gopher-lua"
)

//go:embed lua/*
var library embed.FS

func loadModule(l *lua.LState, name, filename string) error {
	// read code from embed fs
	code, err := library.ReadFile("lua/" + filename)
	if err != nil {
		return err
	}
	// Load the module code
	if err := l.DoString(string(code)); err != nil {
		return fmt.Errorf("failed to load module %s: %w", name, err)
	}
	// Get the returned module table
	module := l.Get(-1)
	l.Pop(1)
	// Verify it's a table
	if module.Type() != lua.LTTable {
		return fmt.Errorf("module %s did not return a table", name)
	}
	// Set as global
	l.SetGlobal(name, module)
	return nil
}

func registerModules(l *lua.LState) error {
	libs := map[string]string{
		"base64":  "base64.lua",
		"json":    "json.lua",
		"inspect": "inspect.lua",
		// "u":       "util.lua",
	}
	for name, filename := range libs {
		if err := loadModule(l, name, filename); err != nil {
			return err
		}
	}
	return nil
}

func makeLMap[M ~map[string][]string](inMap M) map[string]lua.LValue {
	lmap := map[string]lua.LValue{}
	for k, v := range inMap {
		lmap[k] = lua.LString(v[0])
	}
	return lmap
}

//	func makeLArray(l *lua.LState, tblArr []lua.LValue) *lua.LTable {
//		tbl := l.NewTable()
//		for i, v := range tblArr {
//			l.SetField(tbl, i, v)
//		}
//		return tbl
//	}
func makeLTable(l *lua.LState, tblMap map[string]lua.LValue) *lua.LTable {
	tbl := l.NewTable()
	for k, v := range tblMap {
		l.SetField(tbl, k, v)
	}
	return tbl
}

func populateGlobalObject(l *lua.LState, req *Request, res *http.Response) error {
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}

	// headerMap := map[string]lua.LValue{}
	// for k, v := range res.Request.Header {
	// 	headerMap[k] = lua.LString(v[0])
	// }
	headerMap := makeLMap(res.Request.Header)
	headerTbl := makeLTable(l, headerMap)
	// queryMap := map[string]lua.LValue{}
	// for k, v := range res.Request.URL.Query() {
	// 	queryMap[k] = lua.LString(v[0])
	// }
	queryMap := makeLMap(res.Request.URL.Query())
	queryTbl := makeLTable(l, queryMap)

	reqMap := map[string]lua.LValue{
		"url":     lua.LString(res.Request.URL.String()),
		"method":  lua.LString(res.Request.Method),
		"query":   queryTbl,
		"headers": headerTbl,
		"body":    lua.LString(req.BodyRaw),
	}
	if req.Expect != 0 {
		reqMap["expect"] = lua.LNumber(req.Expect)
	}
	reqTbl := makeLTable(l, reqMap)

	// headerMap = map[string]lua.LValue{}
	// for k, v := range res.Header {
	// 	headerMap[k] = lua.LString(v[0])
	// }
	fmt.Println("cookies", res.Cookies())
	headerMap = makeLMap(res.Header)
	headerTbl = makeLTable(l, headerMap)
	resMap := map[string]lua.LValue{
		"status":  lua.LNumber(res.StatusCode),
		"headers": headerTbl,
		"body":    lua.LString(string(body)),
		// "cookies": lua.LTable(),
	}
	resTbl := makeLTable(l, resMap)

	table := makeLTable(l, map[string]lua.LValue{
		"req": reqTbl,
		"res": resTbl,
	})
	l.SetGlobal("rest", table)
	return nil
}

func execute(l *lua.LState, code string) error {
	var cleanError error

	l.SetGlobal("fail", l.NewFunction(func(L *lua.LState) int {
		cleanError = fmt.Errorf(L.CheckString(1))
		for range 10 {
			L.Push(lua.LNil)
		}
		return 10
	}))

	if err := l.DoString(fmt.Sprintf("%s\nreturn nil", code)); err != nil {
		return err
	}
	return cleanError
}

func (r *Request) RunPostHook(res *http.Response) (string, error) {

	l := lua.NewState()
	defer l.Close()

	if err := registerModules(l); err != nil {
		return "", err
	}
	if err := populateGlobalObject(l, r, res); err != nil {
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
