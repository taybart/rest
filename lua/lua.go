// Package restlua provides lua modules for doing rest stuff in the lua runtime
package restlua

import (
	"embed"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

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
		"colors":  "colors.lua",
		"inspect": "inspect.lua",
		"tools":   "tools.lua",
	}
	for name, filename := range libs {
		if err := preloadModule(l, name, filename); err != nil {
			return err
		}
	}
	registerBase64Module(l)
	registerFormModule(l)
	registerJSONModule(l)
	registerJWTModule(l)
	registerUUIDModule(l)
	// TODO: maybe add vim module that can somehow reach out to the plugin
	//       this would be nice for copy to just add a vim.notify('success')

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

		if err := writeClipboard(result); err != nil {
			fmt.Println(err)
			l.Push(lua.LBool(false))
			return 1
		}
		l.Push(lua.LBool(true))
		return 1
	}))

	return nil
}

func writeClipboard(text string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "windows":
		cmd = exec.Command("clip")
	default:
		if _, err := exec.LookPath("wl-copy"); err == nil {
			cmd = exec.Command("wl-copy")
		} else if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command("xsel", "--clipboard", "--input")
		} else {
			return fmt.Errorf("no clipboard utility found (install wl-copy, xclip, or xsel)")
		}
	}
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

func LValueToAny(value lua.LValue) any {
	switch v := value.(type) {
	case lua.LString:
		return string(v)
	case lua.LNumber:
		return float64(v)
	case lua.LBool:
		return bool(v)
	case *lua.LTable:
		return lTableToValue(v)
	case *lua.LNilType:
		return nil
	default:
		return lua.LVAsString(v)
	}
}

func lTableToValue(table *lua.LTable) any {
	maxn := table.MaxN()
	count := 0
	isSeq := true
	table.ForEach(func(key, _ lua.LValue) {
		count++
		if num, ok := key.(lua.LNumber); ok {
			idx := int(num)
			if float64(idx) != float64(num) || idx < 1 || idx > maxn {
				isSeq = false
			}
		} else {
			isSeq = false
		}
	})
	if isSeq && count == maxn {
		arr := make([]any, maxn)
		for i := 1; i <= maxn; i++ {
			arr[i-1] = LValueToAny(table.RawGetInt(i))
		}
		return arr
	}
	return LTableToMap(table)
}

func LTableToMap(table *lua.LTable) map[string]any {
	result := make(map[string]any)

	table.ForEach(func(key, value lua.LValue) {
		keyStr := lua.LVAsString(key)
		result[keyStr] = LValueToAny(value)
	})

	return result
}

func AnyToLValue(state *lua.LState, value any) lua.LValue {
	switch v := value.(type) {
	case string:
		return lua.LString(v)
	case float64:
		return lua.LNumber(v)
	case int:
		return lua.LNumber(v)
	case bool:
		return lua.LBool(v)
	case []any:
		tbl := state.NewTable()
		for i, item := range v {
			tbl.RawSetInt(i+1, AnyToLValue(state, item))
		}
		return tbl
	case map[string]any:
		return MapToLTable(state, v)
	case nil:
		return lua.LNil
	default:
		return lua.LString(fmt.Sprint(v))
	}
}

func MapToLTable(state *lua.LState, data map[string]any) *lua.LTable {
	table := state.NewTable()

	for key, value := range data {
		lval := AnyToLValue(state, value)
		table.RawSetString(key, lval)
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
