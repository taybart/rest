package server

import (
	"embed"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	lua "github.com/yuin/gopher-lua"
)

//go:embed lua/*
var library embed.FS

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
		// "base64":  "base64.lua",
		// "colors":  "colors.lua",
		"json":    "json.lua",
		"inspect": "inspect.lua",
		// "tools":   "tools.lua",
	}
	for name, filename := range libs {
		if err := loadModule(l, name, filename); err != nil {
			return err
		}
	}
	return nil
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
		"query":   makeLTableFromMapOfArr(l, req.URL.Query()),
		"headers": makeLTableFromMapOfArr(l, req.Header),
		"body":    lua.LString(body),
	}
	reqTbl := makeLTable(l, reqMap)

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

	table := makeLTable(l, map[string]lua.LValue{
		// "label":   lua.LString(Label),
		"req": reqTbl,
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

	if err := registerModules(l); err != nil {
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

// Convert LTable to Go map
func ltableToMap(table *lua.LTable) map[string]any {
	result := make(map[string]any)

	table.ForEach(func(key, value lua.LValue) {
		keyStr := lua.LVAsString(key)
		switch v := value.(type) {
		case lua.LString:
			result[keyStr] = string(v)
		case lua.LNumber:
			result[keyStr] = float64(v)
		case lua.LBool:
			result[keyStr] = bool(v)
		case *lua.LTable:
			// Recursively convert nested tables
			result[keyStr] = ltableToMap(v)
		case *lua.LNilType:
			result[keyStr] = nil
		default:
			result[keyStr] = lua.LVAsString(v)
		}

	})

	return result
}
func makeLTableFromMap(l *lua.LState, inMap map[string]string) *lua.LTable {
	tbl := l.NewTable()
	for k, v := range inMap {
		l.SetField(tbl, k, lua.LString(v))
	}
	return tbl
}
func makeLTableFromMapOfArr(l *lua.LState, inMap map[string][]string) *lua.LTable {
	tbl := l.NewTable()
	for k, v := range inMap {
		toMap := map[string]string{}
		for i, v := range v {
			index := strconv.Itoa(i + 1) // because lua stuff
			toMap[index] = v
		}
		l.SetField(tbl, k, makeLTableFromMap(l, toMap))
		// l.SetField(tbl, k, lua.LString(v[0]))
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
