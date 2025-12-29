package main

import (
	"errors"

	"github.com/taybart/rest"
	"github.com/taybart/rest/client"
	restlua "github.com/taybart/rest/lua"
	lua "github.com/yuin/gopher-lua"
)

var rclient *client.Client

func doRequestLabel(f *rest.Rest, label string) (map[string]any, error) {
	req, err := f.Request(label)
	if err != nil {
		return nil, err
	}

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

var exportsTable *lua.LTable

func populateGlobalObject(l *lua.LState, f *rest.Rest) error {
	doLabel := func(l *lua.LState) int {
		label := l.ToString(1) /* get argument */
		exports, err := doRequestLabel(f, label)
		if err != nil {
			panic(err)
		}
		// set all exports from running the request,
		// this will reset on every run for the cli context not the client context
		l.SetField(l.GetGlobal("rest"), "exports", restlua.MapToLTable(l, exports))
		return 0 /* number of results */
	}
	l.SetGlobal("rest", restlua.MakeLTable(l, map[string]lua.LValue{
		"label":   l.NewFunction(doLabel),
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
