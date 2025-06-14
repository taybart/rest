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
		"base64": "base64.lua",
		"json":   "json.lua",
		"u":      "util.lua",
	}
	for name, filename := range libs {
		if err := loadModule(l, name, filename); err != nil {
			return err
		}
	}
	return nil
}

func (r *Request) populateLuaRuntime(l *lua.LState, res *http.Response) error {
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}

	reqTable := l.NewTable()
	l.SetField(reqTable, "body", lua.LString(r.BodyRaw))
	l.SetField(reqTable, "expect", lua.LNumber(r.Expect))

	resTable := l.NewTable()
	l.SetField(resTable, "status", lua.LNumber(res.StatusCode))
	l.SetField(resTable, "body", lua.LString(string(body)))

	table := l.NewTable()
	l.SetField(table, "req", reqTable)
	l.SetField(table, "res", resTable)
	l.SetGlobal("rest", table)
	return nil
}

func (r *Request) RunPostHook(res *http.Response) error {

	l := lua.NewState()
	defer l.Close()

	if err := registerModules(l); err != nil {
		return err
	}

	if err := r.populateLuaRuntime(l, res); err != nil {
		return err
	}

	if err := l.DoString(r.PostHook); err != nil {
		return err
	}
	return nil
}
