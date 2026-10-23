package restlua

import (
	"net/http"
	"net/textproto"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

const (
	reqUserDataKey = "__req"
	resUserDataKey = "__res"
)

func checkRequest(l *lua.LState) *http.Request {
	self := l.CheckTable(1)
	mt, ok := l.GetMetatable(self).(*lua.LTable)
	if !ok {
		l.ArgError(1, "expected rest.req table (use ':' syntax, e.g. rest.req:header(...))")
		return nil
	}
	ud, ok := l.GetField(mt, reqUserDataKey).(*lua.LUserData)
	if !ok {
		l.ArgError(1, "missing underlying http.Request")
		return nil
	}
	req, ok := ud.Value.(*http.Request)
	if !ok {
		l.ArgError(1, "invalid underlying http.Request")
		return nil
	}
	return req
}

func checkResponse(l *lua.LState) *http.Response {
	self := l.CheckTable(1)
	mt, ok := l.GetMetatable(self).(*lua.LTable)
	if !ok {
		l.ArgError(1, "expected rest.res table (use ':' syntax, e.g. rest.res:header(...))")
		return nil
	}
	ud, ok := l.GetField(mt, resUserDataKey).(*lua.LUserData)
	if !ok {
		l.ArgError(1, "missing underlying http.Response")
		return nil
	}
	res, ok := ud.Value.(*http.Response)
	if !ok {
		l.ArgError(1, "invalid underlying http.Response")
		return nil
	}
	return res
}

func findHeader(h http.Header, key string) ([]string, string, bool) {
	if vals, ok := h[key]; ok && len(vals) > 0 {
		return vals, key, true
	}
	canon := textproto.CanonicalMIMEHeaderKey(key)
	if vals, ok := h[canon]; ok && len(vals) > 0 {
		return vals, canon, true
	}
	for k, vals := range h {
		if strings.EqualFold(k, key) && len(vals) > 0 {
			return vals, k, true
		}
	}
	return nil, key, false
}

func pushHeader(l *lua.LState, h http.Header, key string) int {
	vals, matchedKey, ok := findHeader(h, key)
	if !ok {
		l.Push(lua.LNil)
		l.Push(lua.LString(key))
		return 2
	}
	l.Push(lua.LString(vals[0]))
	l.Push(lua.LString(matchedKey))
	return 2
}

func pushHeaderValues(l *lua.LState, h http.Header, key string) int {
	vals, matchedKey, ok := findHeader(h, key)
	if !ok {
		l.Push(lua.LNil)
		l.Push(lua.LString(key))
		return 2
	}
	tbl := l.NewTable()
	for i, v := range vals {
		tbl.RawSetInt(i+1, lua.LString(v))
	}
	l.Push(tbl)
	l.Push(lua.LString(matchedKey))
	return 2
}

func reqPathValue(l *lua.LState) int {
	req := checkRequest(l)
	id := l.CheckString(2)
	l.Push(lua.LString(req.PathValue(id)))
	return 1
}

func reqHeader(l *lua.LState) int {
	req := checkRequest(l)
	key := l.CheckString(2)
	return pushHeader(l, req.Header, key)
}

func reqHeaderValues(l *lua.LState) int {
	req := checkRequest(l)
	key := l.CheckString(2)
	return pushHeaderValues(l, req.Header, key)
}

func reqBearerToken(l *lua.LState) int {
	req := checkRequest(l)
	vals, _, ok := findHeader(req.Header, "Authorization")
	if !ok {
		l.Push(lua.LNil)
		return 1
	}
	l.Push(lua.LString(strings.TrimPrefix(vals[0], "Bearer ")))
	return 1
}

func resHeader(l *lua.LState) int {
	res := checkResponse(l)
	key := l.CheckString(2)
	return pushHeader(l, res.Header, key)
}

func resHeaderValues(l *lua.LState) int {
	res := checkResponse(l)
	key := l.CheckString(2)
	return pushHeaderValues(l, res.Header, key)
}

// SetRequestMetatable attaches the underlying *http.Request as hidden userdata
// and registers request helper methods (path_value, header, header_values, bearer_token)
// on reqTbl.
func SetRequestMetatable(l *lua.LState, reqTbl *lua.LTable, req *http.Request) {
	ud := l.NewUserData()
	ud.Value = req
	mt := l.NewTable()
	l.SetField(mt, reqUserDataKey, ud)
	l.SetField(mt, "__index", l.SetFuncs(l.NewTable(), map[string]lua.LGFunction{
		"path_value":    reqPathValue,
		"header":        reqHeader,
		"header_values": reqHeaderValues,
		"bearer_token":  reqBearerToken,
	}))
	l.SetMetatable(reqTbl, mt)
}

// SetResponseMetatable attaches the underlying *http.Response as hidden userdata
// and registers response helper methods (header, header_values) on resTbl.
func SetResponseMetatable(l *lua.LState, resTbl *lua.LTable, res *http.Response) {
	ud := l.NewUserData()
	ud.Value = res
	mt := l.NewTable()
	l.SetField(mt, resUserDataKey, ud)
	l.SetField(mt, "__index", l.SetFuncs(l.NewTable(), map[string]lua.LGFunction{
		"header":        resHeader,
		"header_values": resHeaderValues,
	}))
	l.SetMetatable(resTbl, mt)
}
