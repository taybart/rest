// Package restlua provides lua modules for doing rest stuff in the lua runtime
package restlua

import (
	"embed"
	"encoding/json"
	"fmt"
	"strconv"

	lua "github.com/yuin/gopher-lua"
	"golang.design/x/clipboard"
)

//go:embed modules/*
var library embed.FS

func preloadModule(l *lua.LState, name, filename string) error {
	code, err := library.ReadFile("modules/" + filename)
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

func RegisterModules(l *lua.LState) error {
	libs := map[string]string{
		"base64":  "base64.lua",
		"colors":  "colors.lua",
		"inspect": "inspect.lua",
		"json":    "json.lua",
		"tools":   "tools.lua",
		"uuid":    "uuid.lua",
	}
	for name, filename := range libs {
		if err := preloadModule(l, name, filename); err != nil {
			return err
		}
	}

	if err := clipboard.Init(); err != nil {
		panic(err)
	}

	l.SetGlobal("copy", l.NewFunction(func(l *lua.LState) int {
		toCopy := l.Get(1)

		var result string

		switch v := toCopy.(type) {
		case *lua.LTable:
			// Marshal table to string
			tbl := LTableToMap(v)
			b, err := json.Marshal(tbl)
			if err != nil {
				l.Push(lua.LBool(false))
				return 1
			}
			result = string(b)
		case lua.LString:
			result = string(v)
		case lua.LNumber:
			result = v.String()
		case lua.LBool:
			result = v.String()
		default:
			result = toCopy.String()
		}

		clipboard.Write(clipboard.FmtText, []byte(result))
		l.Push(lua.LBool(true))
		return 1
	}))

	return nil
}

func LTableToMap(table *lua.LTable) map[string]any {
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
			result[keyStr] = LTableToMap(v)
		case *lua.LNilType:
			result[keyStr] = nil
		default:
			result[keyStr] = lua.LVAsString(v)
		}

	})

	return result
}
func MakeLTableFromMap(l *lua.LState, inMap map[string]string) *lua.LTable {
	tbl := l.NewTable()
	for k, v := range inMap {
		l.SetField(tbl, k, lua.LString(v))
	}
	return tbl
}
func MakeLTableFromMapOfArr(l *lua.LState, inMap map[string][]string) *lua.LTable {
	tbl := l.NewTable()
	for k, v := range inMap {
		toMap := map[string]string{}
		for i, v := range v {
			index := strconv.Itoa(i + 1) // i+1 because lua stuff
			toMap[index] = v
		}
		l.SetField(tbl, k, MakeLTableFromMap(l, toMap))
	}
	return tbl
}

func MakeLTable(l *lua.LState, tblMap map[string]lua.LValue) *lua.LTable {
	tbl := l.NewTable()
	for k, v := range tblMap {
		l.SetField(tbl, k, v)
	}
	return tbl
}
