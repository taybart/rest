package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/taybart/midnight/pages"
)

// Pages are handlers with `page` (instead of `fn`): a script that returns a markup
// element, served as an interactive page (see the pages package). Its actions
// are POSTs to the page's own path, handled by a POST handler rest adds.
//
// Pages are stateful: each page load gets its own lua state, which its
// actions run in (found by the StateHeader the page sends back). A page's
// middleware also runs for its actions.

// StateHeader is the header pages (and stateful handlers) send their state
// id in.
const StateHeader = pages.StateHeader

// pageActionsHandler is the handler for a page's actions (POST to its path).
func pageActionsHandler(page *Handler) *Handler {
	return &Handler{
		Method:      http.MethodPost,
		Path:        page.Path,
		Middleware:  page.Middleware,
		Fn:          dispatchScript,
		Lang:        "lua",
		Stateful:    true,
		pageActions: true,
	}
}

// checkPages returns an error for page handlers that can't work: with `fn` too,
// or with a POST handler of their own on the same path (where actions go).
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

// dispatchScript runs an action of a page handler, from a request body
// (rest.req.body) of { id, data }. It returns a response table.
const dispatchScript = `
local ok, body = pcall(json.decode, rest.req.body)
if not ok or type(body) ~= 'table' or type(body.id) ~= 'string' then
  return { status = 400, body = '{"error": "expected a json body with an action id"}' }
end
return require('midnight.markup').dispatch(body.id, body.data)
`
