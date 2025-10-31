package request

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"

	restlua "github.com/taybart/rest/lua"
	lua "github.com/yuin/gopher-lua"
)

var exportsTable *lua.LTable

func syncExportsTable(L *lua.LState) error {
	// Get the global "rest" table
	restValue := L.GetGlobal("rest")
	restTable, ok := restValue.(*lua.LTable)
	if !ok {
		return fmt.Errorf("global 'rest' is not a table")
	}

	// Get the "exports" field from the rest table
	exportsValue := L.GetField(restTable, "exports")
	exportsTable, ok = exportsValue.(*lua.LTable)
	if !ok {
		return fmt.Errorf("rest.exports is not a table")
	}

	return nil
}

func populateGlobalObject(l *lua.LState, req *Request, res *http.Response, jar http.CookieJar) error {
	defer res.Body.Close()
	reqdump, err := httputil.DumpRequest(res.Request, true)
	if err != nil {
		return err
	}
	resdump, err := httputil.DumpResponse(res, true)
	if err != nil {
		return err
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}

	reqMap := map[string]lua.LValue{
		"url":     lua.LString(res.Request.URL.String()),
		"method":  lua.LString(res.Request.Method),
		"query":   restlua.MakeLTableFromMapOfArr(l, res.Request.URL.Query()),
		"headers": restlua.MakeLTableFromMapOfArr(l, res.Request.Header),
		"body":    lua.LString(req.Body),
		"dump":    lua.LString(string(reqdump)),
	}
	if req.Expect != nil {
		reqMap["expect"] = restlua.MakeLTable(l, map[string]lua.LValue{
			"status":  lua.LNumber(req.Expect.Status),
			"body":    lua.LString(req.Expect.Body),
			"headers": restlua.MakeLTableFromMap(l, req.Expect.Headers),
		})
	} else if req.ExpectStatus != 0 {
		reqMap["expect"] = restlua.MakeLTable(l, map[string]lua.LValue{
			"status": lua.LNumber(req.ExpectStatus),
		})
	}
	reqTbl := restlua.MakeLTable(l, reqMap)

	u, err := url.Parse(res.Request.URL.String())
	if err != nil {
		return err
	}
	cookieMap := map[string]lua.LValue{}
	if jar != nil {
		for _, cookie := range jar.Cookies(u) {
			cookieMap[cookie.Name] = lua.LString(cookie.Value)
		}
	}

	resTbl := restlua.MakeLTable(l, map[string]lua.LValue{
		"status":  lua.LNumber(res.StatusCode),
		"headers": restlua.MakeLTableFromMapOfArr(l, res.Header),
		"body":    lua.LString(string(body)),
		"cookies": restlua.MakeLTable(l, cookieMap),
		"dump":    lua.LString(string(resdump)),
	})

	if exportsTable == nil {
		exportsTable = l.NewTable()
	}

	table := restlua.MakeLTable(l, map[string]lua.LValue{
		"label":   lua.LString(req.Label),
		"req":     reqTbl,
		"res":     resTbl,
		"exports": exportsTable,
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
		return restlua.FmtError(code, err)
	}

	if err := syncExportsTable(l); err != nil {
		return err
	}
	return cleanError
}

func (r *Request) RunPostHook(res *http.Response, jar http.CookieJar) (map[string]any, error) {

	l := lua.NewState()
	defer l.Close()

	if err := restlua.RegisterModules(l); err != nil {
		return nil, err
	}
	if err := populateGlobalObject(l, r, res, jar); err != nil {
		return nil, err
	}

	if err := execute(l, r.PostHook); err != nil {
		return nil, err
	}
	return restlua.LTableToMap(exportsTable), nil
}
