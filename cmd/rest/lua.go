package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/taybart/rest"
	"github.com/taybart/rest/client"
	"github.com/taybart/rest/file"
	restlua "github.com/taybart/rest/lua"
	"github.com/taybart/rest/request"
	lua "github.com/yuin/gopher-lua"
	"golang.org/x/term"
)

var rclient *client.Client
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

func mergeIntoExports(l *lua.LState, exports map[string]any) {
	restVal := l.GetGlobal("rest")
	if restVal.Type() != lua.LTTable {
		return
	}
	exportsVal := l.GetField(restVal, "exports")
	var exportsTbl *lua.LTable
	if exportsVal.Type() == lua.LTTable {
		exportsTbl = exportsVal.(*lua.LTable)
	} else {
		exportsTbl = l.NewTable()
		l.SetField(restVal, "exports", exportsTbl)
	}

	newTbl := restlua.MapToLTable(l, exports)
	newTbl.ForEach(func(key, value lua.LValue) {
		exportsTbl.RawSet(key, value)
	})
}

func populateGlobalObject(l *lua.LState, f *rest.Rest, cliFlags map[string]string) error {

	if exportsTable == nil {
		exportsTable = l.NewTable()
	}

	cliTable := l.NewTable()
	for k, v := range cliFlags {
		l.SetField(cliTable, k, lua.LString(v))
	}
	l.SetGlobal("cli", cliTable)

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
		mergeIntoExports(l, exports)
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
		mergeIntoExports(l, exports)
		return 0 /* number of results */
	}

	l.SetGlobal("rest", restlua.MakeLTable(l, map[string]lua.LValue{
		"file":    l.NewFunction(lDoFile),
		"label":   l.NewFunction(lDoLabel),
		"block":   l.NewFunction(lDoIndex),
		"exports": exportsTable,
	}))

	l.SetGlobal("sleep", l.NewFunction(func(l *lua.LState) int {
		time.Sleep(time.Duration(l.ToInt(1)) * time.Second)
		return 0
	}))

	return nil
}

func execute(l *lua.LState, code string) error {
	if err := l.DoString(code); err != nil {
		return restlua.FmtError(code, err)
	}
	return nil
}

func runCLITool(f *rest.Rest, cliBlock file.CLI, cliFlags map[string]string) error {
	if cliBlock.Loop == nil && cliBlock.Fn == nil {
		return errors.New("no handler fn or loop defined")
	}

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
	if err := populateGlobalObject(l, f, cliFlags); err != nil {
		return err
	}

	if cliBlock.Fn != nil {
		return execute(l, *cliBlock.Fn)
	}

	loopSetup := ""
	if cliBlock.LoopSetup != nil {
		loopSetup = *cliBlock.LoopSetup
	}

	repl := client.NewREPL(true)
	readlineFn := func(l *lua.LState) int {
		prompt := "> "
		if l.GetTop() >= 1 {
			if p, ok := l.Get(1).(lua.LString); ok {
				prompt = string(p)
			}
		}
		var input string
		if term.IsTerminal(int(os.Stdin.Fd())) {
			var err error
			input, err = repl.ReadLine(prompt)
			if err != nil {
				if err == io.EOF {
					l.Push(lua.LNil)
					return 1
				}
				panic(err)
			}
		} else {
			fmt.Print(prompt)
			reader := bufio.NewReader(os.Stdin)
			var err error
			input, err = reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					l.Push(lua.LNil)
					return 1
				}
				panic(err)
			}
			input = strings.TrimSuffix(input, "\n")
		}
		l.Push(lua.LString(input))
		return 1
	}
	l.SetGlobal("__rest_readline", l.NewFunction(readlineFn))

	if err := execute(l, fmt.Sprintf(`
		%s
		while true do
			local input = __rest_readline(PROMPT or "> ")
			if input == nil then break end
			%s
    	::continue::
		end`, loopSetup, *cliBlock.Loop)); err != nil {
		return err
	}

	return nil
}
