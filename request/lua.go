package request

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	lua "github.com/taybart/go-erde"
	restlua "github.com/taybart/rest/lua"
)

func getExportsTable(l *lua.LState) (*lua.LTable, error) {
	exportsValue := l.GetField(l.GetGlobal("rest"), "exports")
	if exportsValue.Type() != lua.LTTable {
		return nil, fmt.Errorf("rest.exports is not a table")
	}
	return exportsValue.(*lua.LTable), nil
}

func syncExportsTable(l *lua.LState) error {
	_, err := getExportsTable(l)
	return err
}

func populateGlobalObject(l *lua.LState, req *Request, res *http.Response, jar http.CookieJar) error {
	defer res.Body.Close()
	if res.Request.GetBody != nil {
		bodyCopy, err := res.Request.GetBody()
		if err != nil {
			return err
		}
		res.Request.Body = bodyCopy
	} else if req.Body != "" {
		res.Request.Body = io.NopCloser(strings.NewReader(req.Body))
	}
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
			"exact":   lua.LBool(req.Expect.Exact),
			"headers": restlua.MakeLTableFromMap(l, req.Expect.Headers),
		})
	} else if req.ExpectStatus != 0 {
		reqMap["expect"] = restlua.MakeLTable(l, map[string]lua.LValue{
			"status": lua.LNumber(req.ExpectStatus),
		})
	}
	reqTbl := restlua.MakeLTable(l, reqMap)
	restlua.SetRequestMetatable(l, reqTbl, res.Request)

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
	restlua.SetResponseMetatable(l, resTbl, res)

	exportsTable := l.NewTable()
	if len(req.Exports) > 0 {
		exportsTable = restlua.MapToLTable(l, req.Exports)
	}

	table := restlua.MakeLTable(l, map[string]lua.LValue{
		"label":   lua.LString(req.Label),
		"req":     reqTbl,
		"res":     resTbl,
		"exports": exportsTable,
	})
	l.SetGlobal("rest", table)
	l.SetGlobal("__rest_check_expectation", l.NewFunction(func(l *lua.LState) int {
		var exp *Expect
		expectStatus := req.ExpectStatus
		if restTbl, ok := l.GetGlobal("rest").(*lua.LTable); ok {
			if rTbl, ok := l.GetField(restTbl, "req").(*lua.LTable); ok {
				if eTbl, ok := l.GetField(rTbl, "expect").(*lua.LTable); ok {
					exp = &Expect{
						Status: int(lua.LVAsNumber(l.GetField(eTbl, "status"))),
						Body:   lua.LVAsString(l.GetField(eTbl, "body")),
						Exact:  lua.LVAsBool(l.GetField(eTbl, "exact")),
					}
					if hTbl, ok := l.GetField(eTbl, "headers").(*lua.LTable); ok {
						headers := map[string]string{}
						hTbl.ForEach(func(k, v lua.LValue) {
							headers[lua.LVAsString(k)] = lua.LVAsString(v)
						})
						exp.Headers = headers
					}
				}
			}
		}
		if err := CheckExpectation(req.Label, exp, expectStatus, res.StatusCode, res.Header, string(body)); err != nil {
			l.Push(lua.LBool(false))
			l.Push(lua.LString(err.Error()))
			return 2
		}
		l.Push(lua.LBool(true))
		return 1
	}))
	return nil
}

func execute(l *lua.LState, code, lang string) error {
	var cleanError error

	l.SetGlobal("fail", l.NewFunction(func(L *lua.LState) int {
		cleanError = errors.New(L.CheckString(1))
		for range 10 {
			L.Push(lua.LNil)
		}
		return 10
	}))

	if err := restlua.Exec(l, code, lang); err != nil {
		return err
	}

	if err := syncExportsTable(l); err != nil {
		return err
	}
	return cleanError
}

func (r *Request) RunAfterHook(res *http.Response, jar http.CookieJar) (map[string]any, error) {

	l := lua.NewState()
	defer l.Close()

	if err := restlua.RegisterModules(l); err != nil {
		return nil, err
	}
	if err := populateGlobalObject(l, r, res, jar); err != nil {
		return nil, err
	}

	if err := execute(l, r.After, r.Lang); err != nil {
		return nil, err
	}
	exportsTable, err := getExportsTable(l)
	if err != nil {
		return nil, err
	}
	return restlua.LTableToMap(exportsTable), nil
}
