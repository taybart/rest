package restlua

import (
	"net/http"
	"net/textproto"
	"strings"

	lua "github.com/taybart/go-erde"
)

const (
	reqUserDataKey = "__req"
	resUserDataKey = "__res"
)

func checkRequest(l *lua.LState) *http.Request {
	return checkRequestAt(l, 1)
}

func checkRequestAt(l *lua.LState, idx int) *http.Request {
	self := l.CheckTable(idx)
	mt, ok := l.GetMetatable(self).(*lua.LTable)
	if !ok {
		l.ArgError(idx, "expected rest.req table (use ':' syntax, e.g. rest.req:header(...))")
		return nil
	}
	ud, ok := l.GetField(mt, reqUserDataKey).(*lua.LUserData)
	if !ok {
		l.ArgError(idx, "missing underlying http.Request")
		return nil
	}
	req, ok := ud.Value.(*http.Request)
	if !ok {
		l.ArgError(idx, "invalid underlying http.Request")
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

func reqQuery(l *lua.LState) int {
	// Supports both rest.req:query("key") (via query table __call where arg1=queryTbl, arg2=reqTbl, arg3="key"
	// or via __index where arg1=reqTbl, arg2="key") and rest.req.query("key") (arg1=queryTbl, arg2="key").
	var req *http.Request
	var key string
	if l.GetTop() >= 3 && l.Get(2).Type() == lua.LTTable {
		req = checkRequestAt(l, 2)
		key = l.CheckString(3)
	} else {
		req = checkRequestAt(l, 1)
		key = l.CheckString(2)
	}
	vals, ok := req.URL.Query()[key]
	if !ok || len(vals) == 0 {
		l.Push(lua.LNil)
		return 1
	}
	l.Push(lua.LString(vals[0]))
	return 1
}

func reqQueryValues(l *lua.LState) int {
	req := checkRequest(l)
	key := l.CheckString(2)
	vals, ok := req.URL.Query()[key]
	if !ok || len(vals) == 0 {
		l.Push(lua.LNil)
		return 1
	}
	tbl := l.NewTable()
	for i, v := range vals {
		tbl.RawSetInt(i+1, lua.LString(v))
	}
	l.Push(tbl)
	return 1
}

func reqCookie(l *lua.LState) int {
	req := checkRequest(l)
	name := l.CheckString(2)
	c, err := req.Cookie(name)
	if err != nil || c == nil {
		l.Push(lua.LNil)
		return 1
	}
	l.Push(lua.LString(c.Value))
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

func sameSiteToString(s http.SameSite) string {
	switch s {
	case http.SameSiteDefaultMode:
		return "default"
	case http.SameSiteLaxMode:
		return "lax"
	case http.SameSiteStrictMode:
		return "strict"
	case http.SameSiteNoneMode:
		return "none"
	default:
		return ""
	}
}

func parseSameSite(s string) http.SameSite {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "default":
		return http.SameSiteDefaultMode
	case "lax":
		return http.SameSiteLaxMode
	case "strict":
		return http.SameSiteStrictMode
	case "none":
		return http.SameSiteNoneMode
	default:
		return 0
	}
}

func resCookie(l *lua.LState) int {
	self := l.CheckTable(1)
	res := checkResponse(l)
	name := l.CheckString(2)
	for _, c := range res.Cookies() {
		if c.Name == name {
			l.Push(lua.LString(c.Value))
			details := MakeLTable(l, map[string]lua.LValue{
				"name":      lua.LString(c.Name),
				"value":     lua.LString(c.Value),
				"path":      lua.LString(c.Path),
				"domain":    lua.LString(c.Domain),
				"max_age":   lua.LNumber(c.MaxAge),
				"secure":    lua.LBool(c.Secure),
				"http_only": lua.LBool(c.HttpOnly),
				"same_site": lua.LString(sameSiteToString(c.SameSite)),
				"raw":       lua.LString(c.Raw),
			})
			l.Push(details)
			return 2
		}
	}
	if cookiesTbl, ok := l.GetField(self, "cookies").(*lua.LTable); ok {
		if val := l.GetField(cookiesTbl, name); val != lua.LNil {
			l.Push(val)
			return 1
		}
	}
	l.Push(lua.LNil)
	return 1
}

// ParseLuaCookies converts a Lua cookies table (either map of name -> string/table
// or array of cookie tables) into a slice of *http.Cookie.
func ParseLuaCookies(tbl *lua.LTable) []*http.Cookie {
	var cookies []*http.Cookie
	tbl.ForEach(func(k, v lua.LValue) {
		switch val := v.(type) {
		case *lua.LTable:
			c := &http.Cookie{}
			if k.Type() == lua.LTString {
				c.Name = k.String()
			}
			val.ForEach(func(ck, cv lua.LValue) {
				switch strings.ToLower(ck.String()) {
				case "name":
					c.Name = cv.String()
				case "value":
					c.Value = cv.String()
				case "path":
					c.Path = cv.String()
				case "domain":
					c.Domain = cv.String()
				case "max_age", "maxage":
					if n, ok := cv.(lua.LNumber); ok {
						c.MaxAge = int(n)
					}
				case "secure":
					c.Secure = lua.LVAsBool(cv)
				case "http_only", "httponly":
					c.HttpOnly = lua.LVAsBool(cv)
				case "same_site", "samesite":
					c.SameSite = parseSameSite(cv.String())
				}
			})
			if c.Name != "" {
				cookies = append(cookies, c)
			}
		case *lua.LNilType:
			// skip nil
		default:
			if k.Type() == lua.LTString {
				cookies = append(cookies, &http.Cookie{
					Name:  k.String(),
					Value: val.String(),
				})
			}
		}
	})
	return cookies
}

// MakeCookiesTable converts a slice of *http.Cookie into a Lua table mapping cookie name to value.
func MakeCookiesTable(l *lua.LState, cookies []*http.Cookie) *lua.LTable {
	cookieMap := make(map[string]lua.LValue, len(cookies))
	for _, c := range cookies {
		cookieMap[c.Name] = lua.LString(c.Value)
	}
	return MakeLTable(l, cookieMap)
}

// SetRequestMetatable attaches the underlying *http.Request as hidden userdata
// and registers request helper methods (path_value, query, query_values, header, header_values, cookie, bearer_token)
// on reqTbl.
func SetRequestMetatable(l *lua.LState, reqTbl *lua.LTable, req *http.Request) {
	if l.GetField(reqTbl, "cookies") == lua.LNil {
		l.SetField(reqTbl, "cookies", MakeCookiesTable(l, req.Cookies()))
	}
	ud := l.NewUserData()
	ud.Value = req
	mt := l.NewTable()
	l.SetField(mt, reqUserDataKey, ud)
	l.SetField(mt, "__index", l.SetFuncs(l.NewTable(), map[string]lua.LGFunction{
		"path_value":    reqPathValue,
		"query":         reqQuery,
		"query_values":  reqQueryValues,
		"header":        reqHeader,
		"header_values": reqHeaderValues,
		"cookie":        reqCookie,
		"bearer_token":  reqBearerToken,
	}))
	l.SetMetatable(reqTbl, mt)

	if qTbl, ok := l.GetField(reqTbl, "query").(*lua.LTable); ok {
		qmt := l.NewTable()
		l.SetField(qmt, reqUserDataKey, ud)
		l.SetField(qmt, "__call", l.NewFunction(reqQuery))
		l.SetMetatable(qTbl, qmt)
	}
}

// SetResponseMetatable attaches the underlying *http.Response as hidden userdata
// and registers response helper methods (header, header_values, cookie) on resTbl.
func SetResponseMetatable(l *lua.LState, resTbl *lua.LTable, res *http.Response) {
	ud := l.NewUserData()
	ud.Value = res
	mt := l.NewTable()
	l.SetField(mt, resUserDataKey, ud)
	l.SetField(mt, "__index", l.SetFuncs(l.NewTable(), map[string]lua.LGFunction{
		"header":        resHeader,
		"header_values": resHeaderValues,
		"cookie":        resCookie,
	}))
	l.SetMetatable(resTbl, mt)
}
