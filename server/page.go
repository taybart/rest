package server

import (
	"fmt"
	"net/http"
	"strings"

	lua "github.com/taybart/go-erde"
)

// Pages are handlers w/ `page` (instead of `fn`): a script that returns a markup
// element (go-erde's markup experiment, ex. `return <App/>`), which is served as
// an interactive page. Functions in `on*` attributes of its elements become
// actions: clicking (etc.) POSTs to the page's own path, which runs the
// function and responds w/ the components that changed (see erde.markup's
// mount / dispatch).
//
// Pages are stateful (see state.go): each page load gets its own lua state,
// which its actions run in (found by the StateHeader the page sends back).
// A page's middleware also runs for its actions.

// mountPage mounts the element a page script returned (args: element, rpc
// url, state id), so rendering it includes the client side of actions.
const mountPage = `
local root, rpc, state = ...
return require('erde.markup').mount(function() return root end, {
  rpc = rpc,
  headers = { ['` + StateHeader + `'] = state },
})
`

// pageActions runs an action, from a request body of { id, data }.
const pageActions = `
local ok, body = pcall(json.decode, rest.req.body)
if not ok or type(body) ~= 'table' or type(body.id) ~= 'string' then
  return { status = 400, body = '{"error": "expected a json body w/ an action id"}' }
end
return require('erde.markup').dispatch(body.id, body.data)
`

// pageActionsHandler is the handler for a page's actions (POST to its path).
func pageActionsHandler(page *Handler) *Handler {
	return &Handler{
		Method:      http.MethodPost,
		Path:        page.Path,
		Middleware:  page.Middleware,
		Fn:          pageActions,
		Lang:        "lua",
		Stateful:    true,
		pageActions: true,
	}
}

// checkPages returns an error for page handlers that can't work: w/ `fn` too,
// or w/ a POST handler of their own on the same path (where actions go).
func checkPages(handlers []*Handler) error {
	posts := map[string]bool{}
	for _, h := range handlers {
		if h.Page == "" && (strings.EqualFold(h.Method, http.MethodPost)) {
			posts[h.Path] = true
		}
	}
	for _, h := range handlers {
		if h.Page == "" {
			continue
		}
		if h.Fn != "" {
			return fmt.Errorf("handler %s %s: use either fn or page, not both", h.Method, h.Path)
		}
		if posts[h.Path] {
			return fmt.Errorf("handler %s %s: page actions are POSTs to the same path, which already has a POST handler", h.Method, h.Path)
		}
	}
	return nil
}

// pageIsInteractive returns whether the page that was just rendered has
// actions. Pages w/o any never call back, so their state isn't kept.
func pageIsInteractive(l *lua.LState) (bool, error) {
	if l.GetField(l.GetField(l.GetGlobal("package"), "loaded"), lua.MarkupLibName) == lua.LNil {
		return false, nil // not a markup page (ex. a redirect)
	}
	fn, err := l.LoadString(`return require('erde.markup').isInteractive()`)
	if err != nil {
		return false, err
	}
	if err := l.CallByParam(lua.P{Fn: fn, NRet: 1, Protect: true}); err != nil {
		return false, err
	}
	interactive := lua.LVAsBool(l.Get(-1))
	l.Pop(1)
	return interactive, nil
}

// mountPageResult mounts what a page script returned, if it's an element (a
// page may also return a response table, ex. a redirect).
func mountPageResult(l *lua.LState, v lua.LValue, rpc, stateID string) (lua.LValue, error) {
	if _, isElement, err := markupElement(l, v); err != nil || !isElement {
		return v, err
	}
	fn, err := l.LoadString(mountPage)
	if err != nil {
		return nil, err
	}
	if err := l.CallByParam(lua.P{Fn: fn, NRet: 1, Protect: true}, v, lua.LString(rpc), lua.LString(stateID)); err != nil {
		return nil, err
	}
	mounted := l.Get(-1)
	l.Pop(1)
	return mounted, nil
}
