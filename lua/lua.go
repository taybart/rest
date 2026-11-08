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
	"sync"

	lua "github.com/taybart/midnight"
	"github.com/taybart/midnight/parse"
)

//go:embed modules/*
var library embed.FS

// Lua modules, as globals. They're loaded the first time they're used (most
// scripts don't use all, or any, of them), and compiled once: their bytecode
// is shared by every state.
var lazyModules = map[string]string{
	"colors":  "colors.lua",
	"inspect": "inspect.lua",
	"tools":   "tools.lua",
}

var moduleProtos sync.Map // filename -> *lua.FunctionProto

func moduleProto(filename string) (*lua.FunctionProto, error) {
	if proto, ok := moduleProtos.Load(filename); ok {
		return proto.(*lua.FunctionProto), nil
	}
	code, err := library.ReadFile("modules/" + filename)
	if err != nil {
		return nil, err
	}
	chunk, err := parse.Parse(strings.NewReader(string(code)), filename)
	if err != nil {
		return nil, err
	}
	proto, err := lua.Compile(chunk, filename)
	if err != nil {
		return nil, err
	}
	actual, _ := moduleProtos.LoadOrStore(filename, proto)
	return actual.(*lua.FunctionProto), nil
}

func loadModule(l *lua.LState, name, filename string) (lua.LValue, error) {
	proto, err := moduleProto(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to load module %s: %w", name, err)
	}
	l.Push(l.NewFunctionFromProto(proto))
	if err := l.PCall(0, 1, nil); err != nil {
		return nil, fmt.Errorf("failed to load module %s: %w", name, err)
	}
	module := l.Get(-1)
	l.Pop(1)
	if module.Type() != lua.LTTable {
		return nil, fmt.Errorf("module %s did not return a table", name)
	}
	return module, nil
}

// registerLazyModules makes the lazy modules globals that load on first use
// (w/ an __index on the globals table).
func registerLazyModules(l *lua.LState) {
	globals := l.Get(lua.GlobalsIndex).(*lua.LTable)
	mt := l.NewTable()
	l.SetField(mt, "__index", l.NewFunction(func(l *lua.LState) int {
		name, ok := l.Get(2).(lua.LString)
		filename, lazy := lazyModules[string(name)]
		if !ok || !lazy {
			l.Push(lua.LNil)
			return 1
		}
		module, err := loadModule(l, string(name), filename)
		if err != nil {
			l.RaiseError("%s", err.Error())
		}
		l.RawSet(globals, name, module)
		l.Push(module)
		return 1
	}))
	l.SetMetatable(globals, mt)
}

func RegisterModules(l *lua.LState) error {
	registerLazyModules(l)
	registerBase64Module(l)
	registerFormModule(l)
	registerHTMLModule(l)
	registerJSONModule(l)
	registerJWTModule(l)
	registerNanoIDModule(l)
	registerULIDModule(l)
	registerURLModule(l)
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
		// sized exactly: NewTable preallocates room for 32 items and 32
		// keys (~2KB), a lot for a decoded json object w/ a few fields
		tbl := state.CreateTable(len(v), 0)
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
	table := state.CreateTable(0, len(data))

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
