package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	lua "github.com/taybart/go-erde"
	"github.com/taybart/log"
	restlua "github.com/taybart/rest/lua"
)

/*
  TODO:
		- maybe better return type
		- maybe add some go funcs for directly writing response and such
*/

func populateGlobalObject(l *lua.LState, req *http.Request) error {
	var body []byte
	if req.Body != nil {
		var err error
		body, err = io.ReadAll(req.Body)
		req.Body.Close()
		if err != nil {
			return err
		}
		req.Body = io.NopCloser(bytes.NewReader(body))
	}

	reqMap := map[string]lua.LValue{
		"url":     lua.LString(req.URL.String()),
		"method":  lua.LString(req.Method),
		"query":   restlua.MakeLTableFromMapOfArr(l, req.URL.Query()),
		"headers": restlua.MakeLTableFromMapOfArr(l, req.Header),
		"body":    lua.LString(body),
	}
	reqTbl := restlua.MakeLTable(l, reqMap)
	restlua.SetRequestMetatable(l, reqTbl, req)

	table := restlua.MakeLTable(l, map[string]lua.LValue{
		"path": lua.LString(req.URL.Path),
		"req":  reqTbl,
		"ctx":  l.NewTable(),
	})
	l.SetGlobal("rest", table)
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

	return cleanError
}

func (s *Server) initKV() error {
	var initErr error
	s.kvOnce.Do(func() {
		s.kvMu.Lock()
		s.kv = make(map[string]any, len(s.Config.KV))
		for k, v := range s.Config.KV {
			s.kv[k] = v
		}
		s.kvMu.Unlock()

		if s.Config.Setup != "" {
			initErr = s.RunLuaSetup()
		}
	})
	return initErr
}

func (s *Server) kvCacheLoader(l *lua.LState) int {
	get := func(l *lua.LState) int {
		key := l.ToString(1)
		s.kvMu.RLock()
		v, ok := s.kv[key]
		s.kvMu.RUnlock()
		if ok && v != nil {
			l.Push(restlua.AnyToLValue(l, v))
		} else {
			l.Push(lua.LNil)
		}
		return 1
	}
	set := func(l *lua.LState) int {
		key := l.ToString(1)
		value := l.Get(2)
		goVal := restlua.LValueToAny(value)
		s.kvMu.Lock()
		if s.kv == nil {
			s.kv = make(map[string]any)
		}
		if goVal == nil {
			delete(s.kv, key)
		} else {
			s.kv[key] = goVal
		}
		s.kvMu.Unlock()
		return 0
	}
	del := func(l *lua.LState) int {
		key := l.ToString(1)
		s.kvMu.Lock()
		delete(s.kv, key)
		s.kvMu.Unlock()
		return 0
	}
	all := func(l *lua.LState) int {
		s.kvMu.RLock()
		snapshot := make(map[string]any, len(s.kv))
		for k, v := range s.kv {
			snapshot[k] = v
		}
		s.kvMu.RUnlock()
		l.Push(restlua.MapToLTable(l, snapshot))
		return 1
	}
	l.SetGlobal("kv", restlua.MakeLTable(l, map[string]lua.LValue{
		"get": l.NewFunction(get),
		"set": l.NewFunction(set),
		"del": l.NewFunction(del),
		"all": l.NewFunction(all),
	}))
	return 1
}

func (s *Server) luaHelpers(l *lua.LState) error {
	_ = s.initKV()
	s.kvCacheLoader(l)
	return nil
}

func (s *Server) RunLuaSetup() error {
	if s.Config.Setup == "" {
		return nil
	}
	l := restlua.NewState()
	defer l.Close()

	if err := restlua.RegisterModules(l); err != nil {
		return err
	}
	s.kvCacheLoader(l)
	rest := l.NewTable()
	l.SetGlobal("rest", rest)
	restlua.SetRequester(l, rest, s.Config.Requester)
	if err := execute(l, s.Config.Setup, s.Config.SetupLang); err != nil {
		return err
	}
	// setup's exports are where every request's (and state's) exports start
	s.setupExports = restlua.LTableToMap(restlua.Exports(l))
	return nil
}

// markupElement returns the markup runtime if `v` is a markup element (built by
// erde.markup, ex. `<App/>` in an erde script w/ the markup experiment).
func markupElement(l *lua.LState, v lua.LValue) (lua.LValue, bool, error) {
	// only tables w/ a metatable can be elements, so plain responses don't
	// load the runtime
	if _, ok := v.(*lua.LTable); !ok || l.GetMetatable(v) == lua.LNil {
		return nil, false, nil
	}
	if err := l.CallByParam(lua.P{Fn: l.GetGlobal("require"), NRet: 1, Protect: true}, lua.LString(lua.MarkupLibName)); err != nil {
		return nil, false, err
	}
	markup := l.Get(-1)
	l.Pop(1)

	if err := l.CallByParam(lua.P{Fn: l.GetField(markup, "isElement"), NRet: 1, Protect: true}, v); err != nil {
		return nil, false, err
	}
	isElement := lua.LVAsBool(l.Get(-1))
	l.Pop(1)
	return markup, isElement, nil
}

// renderElement renders `v` as an HTML document if it's a markup element.
func renderElement(l *lua.LState, v lua.LValue) (string, bool, error) {
	markup, isElement, err := markupElement(l, v)
	if err != nil || !isElement {
		return "", false, err
	}

	if err := l.CallByParam(lua.P{Fn: l.GetField(markup, "document"), NRet: 1, Protect: true}, v); err != nil {
		return "", false, err
	}
	html := l.Get(-1).String()
	l.Pop(1)
	return html, true, nil
}

func setDefaultHeader(res *Response, key, value string) {
	for k := range res.Headers {
		if strings.EqualFold(k, key) {
			return
		}
	}
	if res.Headers == nil {
		res.Headers = make(map[string]string)
	}
	res.Headers[key] = value
}

const htmlContentType = "text/html; charset=utf-8"

// mapLuaResult maps what a handler / middleware returned to a response: a
// response table, or a markup element (rendered as an HTML document).
func mapLuaResult(l *lua.LState, v lua.LValue) (Response, error) {
	html, ok, err := renderElement(l, v)
	if err != nil {
		return Response{}, err
	}
	if ok {
		res := Response{Status: http.StatusOK, Body: json.RawMessage(html)}
		setDefaultHeader(&res, "Content-Type", htmlContentType)
		return res, nil
	}
	return mapLuaResponse(l, v.(*lua.LTable))
}

func mapLuaResponse(l *lua.LState, luaRes *lua.LTable) (Response, error) {
	res := Response{
		Status: http.StatusOK,
	}
	isHTML, isJSON := false, false
	if cookiesTbl, ok := l.GetField(luaRes, "cookies").(*lua.LTable); ok {
		res.Cookies = restlua.ParseLuaCookies(cookiesTbl)
	}
	var mapErr error
	luaRes.ForEach(func(k, v lua.LValue) {
		if mapErr != nil {
			return
		}
		switch strings.ToLower(k.String()) {
		case "status":
			if num, ok := v.(lua.LNumber); ok {
				res.Status = int(num)
			} else {
				mapErr = fmt.Errorf("expected number for status, got %s", v.Type().String())
			}
		case "headers":
			if tbl, ok := v.(*lua.LTable); ok {
				res.Headers = make(map[string]string)
				tbl.ForEach(func(hk, hv lua.LValue) {
					res.Headers[hk.String()] = hv.String()
				})
			} else if v != lua.LNil {
				mapErr = fmt.Errorf("expected table for headers, got %s", v.Type().String())
			}
		case "body":
			// `body = <App/>`
			if html, ok, err := renderElement(l, v); err != nil || ok {
				res.Body, isHTML, mapErr = json.RawMessage(html), ok, err
				return
			}
			switch b := v.(type) {
			case lua.LString:
				res.Body = json.RawMessage(b)
			case *lua.LTable:
				encoded, err := restlua.EncodeJSON(b)
				if err != nil {
					mapErr = err
					return
				}
				res.Body = json.RawMessage(encoded)
				isJSON = true
			case *lua.LNilType:
				// leave empty
			default:
				res.Body = json.RawMessage(b.String())
			}
		}
	})
	if isHTML {
		setDefaultHeader(&res, "Content-Type", htmlContentType)
	}
	if isJSON {
		setDefaultHeader(&res, "Content-Type", "application/json")
	}
	return res, mapErr
}

func (s *Server) findMiddleware(name string) (*Middleware, bool) {
	for _, mw := range s.Config.Middlewares {
		if mw.Name == name {
			return mw, true
		}
	}
	return nil, false
}

func (s *Server) RunLuaPipeline(handler *Handler, req *http.Request, w http.ResponseWriter) (Response, bool, error) {
	res := Response{
		Status: http.StatusOK,
	}

	var l *lua.LState
	stateID := ""
	keepPage := false
	if handler.pageActions && req.Header.Get(StateHeader) == "" {
		return Response{Status: http.StatusBadRequest, Body: json.RawMessage(`{"error": "missing ` + StateHeader + ` header"}`)}, true, nil
	}
	// cached responses (see cache.go): w/o middleware, before any lua runs
	cache, cacheKey := handler.cache, ""
	if cache != nil && (req.Method == http.MethodGet || req.Method == http.MethodHead) {
		cacheKey = responseCacheKey(handler, req)
		if len(handler.Middleware) == 0 {
			if cached, ok := cache.get(cacheKey, acceptsGzip(req)); ok {
				return cached, true, nil
			}
		}
	} else {
		cache = nil
	}
	if (handler.Stateful || handler.Page != "") && s.states != nil {
		id, state, isNew, err := s.states.acquire(req)
		if gone := (errStateGone{}); errors.As(err, &gone) {
			body, _ := json.Marshal(map[string]string{"error": gone.Error()})
			return Response{Status: http.StatusGone, Body: body}, true, nil
		} else if err != nil {
			return res, false, err
		}
		defer s.states.release(state)
		l, stateID = state.l, id
		if handler.Page != "" {
			// a page's state is only kept if it has actions (see pageIsInteractive)
			defer func() {
				if !keepPage {
					s.states.discard(id, state)
				}
			}()
		}
		if isNew {
			if err := restlua.RegisterModules(l); err != nil {
				return res, false, err
			}
		}
	} else {
		l = restlua.NewState()
		defer l.Close()
		if err := restlua.RegisterModules(l); err != nil {
			return res, false, err
		}
	}

	// a kept state keeps its exports between requests
	var exports lua.LValue = lua.LNil
	if prev, ok := l.GetGlobal("rest").(*lua.LTable); ok {
		exports = l.GetField(prev, "exports")
	}
	if err := populateGlobalObject(l, req); err != nil {
		return res, false, err
	}
	rest := l.GetGlobal("rest").(*lua.LTable)
	if stateID != "" {
		l.SetField(rest, "state", lua.LString(stateID))
	}
	if err := s.luaHelpers(l); err != nil {
		return res, false, err
	}
	if _, ok := exports.(*lua.LTable); !ok {
		exports = restlua.MapToLTable(l, s.setupExports)
	}
	l.SetField(rest, "exports", exports)
	restlua.SetRequester(l, rest, s.Config.Requester)

	for _, mwName := range handler.Middleware {
		mw, ok := s.findMiddleware(mwName)
		if !ok {
			return res, false, fmt.Errorf("middleware %q not found", mwName)
		}
		l.SetTop(0)
		if err := execute(l, mw.Fn, mw.Lang); err != nil {
			return res, false, err
		}
		if l.GetTop() > 0 && l.Get(-1) != lua.LNil {
			if _, ok := l.Get(-1).(*lua.LTable); !ok {
				return res, false, fmt.Errorf("middleware %q must return a response table, markup element, or nil%s", mwName, returnedHint(l.Get(-1)))
			}
			mapped, err := mapLuaResult(l, l.Get(-1))
			return mapped, true, err
		}
	}

	if err := ValidateRequest(handler.Expect, req); err != nil {
		log.Warnf("%s %s: %v\n", req.Method, req.URL.Path, err)
		return validationResponse(err), true, nil
	}
	// w/ middleware, it's run first (ex. auth), even for cached responses
	if cache != nil && len(handler.Middleware) > 0 {
		if cached, ok := cache.get(cacheKey, acceptsGzip(req)); ok {
			return cached, true, nil
		}
	}

	if handler.Page != "" {
		l.SetTop(0)
		if err := execute(l, handler.Page, handler.Lang); err != nil {
			return res, false, err
		}
		if _, ok := l.Get(-1).(*lua.LTable); !ok {
			return res, false, fmt.Errorf("page must return a markup element or response table%s", returnedHint(l.Get(-1)))
		}
		page, err := mountPageResult(l, l.Get(-1), req.URL.Path, stateID)
		if err != nil {
			return res, false, err
		}
		mapped, err := mapLuaResult(l, page)
		if err == nil {
			keepPage, err = pageIsInteractive(l)
		}
		if err == nil && cache != nil {
			cache.store(handler, cacheKey, &mapped, keepPage)
		}
		return mapped, true, err
	}

	if handler.Fn != "" {
		l.SetTop(0)
		if err := execute(l, handler.Fn, handler.Lang); err != nil {
			return res, false, err
		}
		if _, ok := l.Get(-1).(*lua.LTable); !ok {
			return res, false, fmt.Errorf("handler fn must return a response table or markup element%s", returnedHint(l.Get(-1)))
		}
		mapped, err := mapLuaResult(l, l.Get(-1))
		if err == nil && cache != nil {
			cache.store(handler, cacheKey, &mapped, false)
		}
		return mapped, true, err
	}

	return res, false, nil
}

// returnedHint describes what a script returned instead of a response, ex.
// a component instead of an element.
func returnedHint(v lua.LValue) string {
	switch v.Type() {
	case lua.LTNil:
		return " (got nothing, is the return missing?)"
	case lua.LTFunction:
		return " (got a function: if it's a component, render it w/ `return <App/>`)"
	default:
		return fmt.Sprintf(" (got a %s)", v.Type())
	}
}

func (s *Server) RunLuaHandler(handler string, req *http.Request, w http.ResponseWriter) (Response, error) {
	res, _, err := s.RunLuaPipeline(&Handler{Fn: handler}, req, w)
	return res, err
}
