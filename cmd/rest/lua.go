package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	lua "github.com/taybart/go-erde"
	"github.com/taybart/rest"
	"github.com/taybart/rest/client"
	"github.com/taybart/rest/file"
	restlua "github.com/taybart/rest/lua"
	"github.com/taybart/rest/request"
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

func populateGlobalObject(l *lua.LState, f *rest.Rest, cliBlock file.CLI, cliFlags map[string]string, restArgs []string) error {

	if exportsTable == nil {
		exportsTable = l.NewTable()
	}

	cliTable := l.NewTable()

	restTbl := l.NewTable()
	for i, arg := range restArgs {
		restTbl.RawSetInt(i+1, lua.LString(arg))
	}
	mt := l.NewTable()
	idxTbl := l.NewTable()
	joinFn := l.NewFunction(func(l *lua.LState) int {
		tbl, ok := l.Get(1).(*lua.LTable)
		if !ok {
			l.Push(lua.LString(strings.Join(restArgs, " ")))
			return 1
		}
		sep := " "
		if l.GetTop() >= 2 && l.Get(2) != lua.LNil {
			sep = l.ToString(2)
		}
		var parts []string
		for i := 1; i <= tbl.MaxN(); i++ {
			v := tbl.RawGetInt(i)
			if v != lua.LNil {
				parts = append(parts, lua.LVAsString(v))
			}
		}
		l.Push(lua.LString(strings.Join(parts, sep)))
		return 1
	})
	l.SetField(idxTbl, "string", joinFn)
	l.SetField(mt, "__index", idxTbl)
	l.SetField(mt, "__tostring", joinFn)
	l.SetMetatable(restTbl, mt)
	l.SetField(cliTable, "rest", restTbl)

	for k, v := range cliFlags {
		if flagDef, ok := cliBlock.Flags[k]; ok && flagDef.Bool {
			l.SetField(cliTable, k, lua.LBool(v == "true"))
		} else {
			l.SetField(cliTable, k, lua.LString(v))
		}
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
	lDoLoop := func(l *lua.LState) int {
		if cliBlock.Loop == nil {
			l.RaiseError("no loop defined in cli block")
			return 0
		}
		if err := runLoop(l, *cliBlock.Loop, cliBlock.LoopLang); err != nil {
			l.RaiseError("%s", err.Error())
		}
		return 0
	}

	l.SetGlobal("rest", restlua.MakeLTable(l, map[string]lua.LValue{
		"file":    l.NewFunction(lDoFile),
		"label":   l.NewFunction(lDoLabel),
		"block":   l.NewFunction(lDoIndex),
		"loop":    l.NewFunction(lDoLoop),
		"exports": exportsTable,
	}))

	l.SetGlobal("sleep", l.NewFunction(func(l *lua.LState) int {
		time.Sleep(time.Duration(l.ToInt(1)) * time.Second)
		return 0
	}))

	return nil
}

func execute(l *lua.LState, code, lang string) error {
	return restlua.Exec(l, code, lang)
}

func runLoop(l *lua.LState, loopCode, lang string) error {
	repl := client.NewREPL(true)
	reader := bufio.NewReader(os.Stdin)
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

	if lang == restlua.LangErde {
		// erde has continue built in
		return execute(l, fmt.Sprintf(`
		while true {
			local input = __rest_readline(PROMPT || "> ")
			if input == nil { break }
			%s
		}`, loopCode), lang)
	}
	return execute(l, fmt.Sprintf(`
		while true do
			local input = __rest_readline(PROMPT or "> ")
			if input == nil then break end
			%s
    	::continue::
		end`, loopCode), lang)
}

func runCLITool(f *rest.Rest, cliBlock file.CLI, cliFlags map[string]string, restArgs []string) error {
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
	if err := populateGlobalObject(l, f, cliBlock, cliFlags, restArgs); err != nil {
		return err
	}

	if cliBlock.Fn != nil {
		return execute(l, *cliBlock.Fn, cliBlock.FnLang)
	}

	return runLoop(l, *cliBlock.Loop, cliBlock.LoopLang)
}
