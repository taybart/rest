package main

import (
	"errors"

	"github.com/taybart/rest"
	"github.com/taybart/rest/client"
	restlua "github.com/taybart/rest/lua"
	"github.com/taybart/rest/request"
	lua "github.com/yuin/gopher-lua"
)

var exportsTable *lua.LTable

func syncExportsTable(l *lua.LState, f *rest.Rest) error {
	// Get the "exports" field from the rest table
	exportsValue := l.GetField(l.GetGlobal("rest"), "exports")
	var ok bool
	exportsTable, ok = exportsValue.(*lua.LTable)
	if !ok {
		return errors.New("rest.exports is not a table")
	}
	f.Parser.AddExportsCtx(restlua.LTableToMap(exportsTable))

	return nil
}

var rclient *client.Client

func do(f *rest.Rest, req request.Request) (map[string]any, error) {
	if req.Skip {
		return nil, errors.New("request marked as skip = true")
	}

	_, exports, err := rclient.Do(req)
	if err != nil {
		return nil, err
	}

	// make sure to add the exports back into parsers ctx
	f.Parser.AddExportsCtx(exports)
	return exports, nil
}

func populateGlobalObject(l *lua.LState, f *rest.Rest) error {

	if exportsTable == nil {
		exportsTable = l.NewTable()
	}

	lDoFile := func(l *lua.LState) int {
		ignoreFail := l.ToBool(1) /* get argument */
		if err := syncExportsTable(l, f); err != nil {
			panic(err)
		}

		if err := f.RunFile(ignoreFail); err != nil {
			panic(err)
		}
		return 0 /* number of results */
	}
	lDoLabel := func(l *lua.LState) int {
		label := l.ToString(1) /* get argument */
		if err := syncExportsTable(l, f); err != nil {
			panic(err)
		}
		req, err := f.Request(label)
		if err != nil {
			panic(err)
		}
		exports, err := do(f, req)
		if err != nil {
			panic(err)
		}
		// set all exports from running the request,
		// this will reset on every run for the cli context not the client context
		l.SetField(l.GetGlobal("rest"), "exports", restlua.MapToLTable(l, exports))
		return 0 /* number of results */
	}
	lDoIndex := func(l *lua.LState) int {
		idx := l.ToInt(1) /* get argument */
		if err := syncExportsTable(l, f); err != nil {
			panic(err)
		}
		req, err := f.RequestByIndex(idx)
		if err != nil {
			panic(err)
		}
		exports, err := do(f, req)
		if err != nil {
			panic(err)
		}
		// set all exports from running the request,
		// this will reset on every run for the cli context not the client context
		l.SetField(l.GetGlobal("rest"), "exports", restlua.MapToLTable(l, exports))
		return 0 /* number of results */
	}

	l.SetGlobal("rest", restlua.MakeLTable(l, map[string]lua.LValue{
		"file":    l.NewFunction(lDoFile),
		"label":   l.NewFunction(lDoLabel),
		"block":   l.NewFunction(lDoIndex),
		"exports": exportsTable,
	}))

	return nil
}

func execute(l *lua.LState, code string) error {
	if err := l.DoString(code); err != nil {
		return restlua.FmtError(code, err)
	}
	return nil
}

func RunCLITool(f *rest.Rest) error {

	var err error
	rclient, err = client.New(f.Parser.Config)
	if err != nil {
		return err
	}

	l := lua.NewState()
	defer l.Close()

	if err := restlua.RegisterModules(l); err != nil {
		return err
	}
	if err := populateGlobalObject(l, f); err != nil {
		return err
	}
	if err := execute(l, *f.Parser.Root.CLI); err != nil {
		return err
	}

	return nil
}
