package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	lua "github.com/taybart/go-erde"
	"github.com/taybart/log"
	restlua "github.com/taybart/rest/lua"
	"github.com/taybart/rest/pages"
)

/*
  TODO:
		- maybe better return type
		- maybe add some go funcs for directly writing response and such
*/

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

// mapLuaResult maps what a handler / middleware returned to a response: a
// response table, or a markup element (rendered as an HTML document).
func mapLuaResult(l *lua.LState, v lua.LValue) (Response, error) {
	res, err := pages.MapResult(l, v)
	return Response{Status: res.Status, Headers: res.Headers, Cookies: res.Cookies, Body: json.RawMessage(res.Body)}, err
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
		id, state, isNew, err := s.states.Acquire(req)
		if gone := (pages.ErrStateGone{}); errors.As(err, &gone) {
			body, _ := json.Marshal(map[string]string{"error": gone.Error()})
			return Response{Status: http.StatusGone, Body: body}, true, nil
		} else if err != nil {
			return res, false, err
		}
		defer s.states.Release(state)
		l, stateID = state.L, id
		if handler.Page != "" {
			// a page's state is only kept if it has actions (see pageIsInteractive)
			defer func() {
				if !keepPage {
					s.states.Discard(id, state)
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

	// (a kept state keeps its rest table, and so its exports)
	if err := restlua.SetRestGlobal(l, req); err != nil {
		return res, false, err
	}
	rest := l.GetGlobal("rest").(*lua.LTable)
	if stateID != "" {
		l.SetField(rest, "state", lua.LString(stateID))
	}
	if err := s.luaHelpers(l); err != nil {
		return res, false, err
	}
	if _, ok := l.GetField(rest, "exports").(*lua.LTable); !ok {
		l.SetField(rest, "exports", restlua.MapToLTable(l, s.setupExports))
	}
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
		page, err := pages.Mount(l, l.Get(-1), req.URL.Path, stateID)
		if err != nil {
			return res, false, err
		}
		mapped, err := mapLuaResult(l, page)
		if err == nil {
			keepPage, err = pages.IsInteractive(l)
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
