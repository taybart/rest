// Package restlua provides lua modules for doing rest stuff in the lua runtime
package restlua

import (
	"embed"
	"fmt"
	"strconv"

	lua "github.com/yuin/gopher-lua"
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

func MapToLTable(state *lua.LState, data map[string]any) *lua.LTable {
	table := state.NewTable()

	for key, value := range data {
		switch v := value.(type) {
		case string:
			table.RawSetString(key, lua.LString(v))
		case float64:
			table.RawSetString(key, lua.LNumber(v))
		case bool:
			table.RawSetString(key, lua.LBool(v))
		case map[string]any:
			// Recursively convert nested maps
			table.RawSetString(key, MapToLTable(state, v))
		case nil:
			table.RawSetString(key, lua.LNil)
		default:
			// Fallback for unknown types
			table.RawSetString(key, lua.LString(fmt.Sprint(v)))
		}
	}

	return table
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
