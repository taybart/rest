package restlua

import (
	"fmt"
	"net/http"

	lua "github.com/taybart/go-erde"
)

// Requester runs one of the file's request blocks (w/ its expect and after
// hook): target is its label (string) or block index (int), and exports are
// what the block's hcl sees as `exports`.
type Requester func(target any, exports map[string]any) (*RequestResult, error)

// RequestResult is a request block's response, and what its after hook
// exported and returned.
type RequestResult struct {
	Res     *http.Response
	Body    []byte
	Exports map[string]any
	// what the after hook returned, nil if it didn't return anything
	Returns []any
	// for cached responses: the body decoded from json (once), as read-only
	// tables shared by every state, which res:json() returns
	SharedJSON func() (lua.LValue, error)
}

// SetRequester sets `rest.request(label | index [, exports])` on restTbl.
//
// The exports are set in rest.exports, and the request block is evaluated w/
// rest.exports. The after hook's exports are merged back into rest.exports.
// It returns what the after hook returned, if it returned anything, or else
// the response (like rest.res in after hooks). Or nil and an error (a network
// error, a failed expect, or fail() in the after hook).
func SetRequester(l *lua.LState, restTbl *lua.LTable, run Requester) {
	l.SetField(restTbl, "request", l.NewFunction(func(l *lua.LState) int {
		var target any
		switch v := l.Get(1).(type) {
		case lua.LString:
			target = string(v)
		case lua.LNumber:
			target = int(v)
		default:
			l.ArgError(1, "expected a request label or block index")
		}
		if run == nil {
			l.RaiseError("rest.request: no request blocks (rest.request runs request blocks of the .rest file)")
		}

		exports := Exports(l)
		if set, ok := l.Get(2).(*lua.LTable); ok {
			set.ForEach(func(k, v lua.LValue) { exports.RawSet(k, v) })
		} else if l.Get(2) != lua.LNil {
			l.ArgError(2, "expected a table of exports")
		}

		result, err := run(target, LTableToMap(exports))
		if err != nil {
			l.Push(lua.LNil)
			l.Push(lua.LString(fmt.Sprintf("rest.request(%v): %v", target, err)))
			return 2
		}
		if len(result.Exports) > 0 {
			MapToLTable(l, result.Exports).ForEach(func(k, v lua.LValue) { exports.RawSet(k, v) })
		}
		if result.Returns != nil {
			for _, v := range result.Returns {
				l.Push(AnyToLValue(l, v))
			}
			return len(result.Returns)
		}
		res := NewResponseTable(l, result.Res, result.Body)
		if result.SharedJSON != nil {
			l.SetField(res, "json", l.NewFunction(func(l *lua.LState) int {
				v, err := result.SharedJSON()
				if err != nil {
					l.RaiseError("json: %v", err)
				}
				l.Push(v)
				return 1
			}))
		}
		l.Push(res)
		return 1
	}))
}

// Exports returns rest.exports, creating it if needed.
func Exports(l *lua.LState) *lua.LTable {
	restTbl, ok := l.GetGlobal("rest").(*lua.LTable)
	if !ok {
		restTbl = l.NewTable()
		l.SetGlobal("rest", restTbl)
	}
	exports, ok := l.GetField(restTbl, "exports").(*lua.LTable)
	if !ok {
		exports = l.NewTable()
		l.SetField(restTbl, "exports", exports)
	}
	return exports
}

// NewResponseTable makes a response table, like rest.res in after hooks:
// status, headers, body, cookies, and header(), header_values(), cookie(),
// and json() methods.
func NewResponseTable(l *lua.LState, res *http.Response, body []byte) *lua.LTable {
	cookies := map[string]lua.LValue{}
	for _, c := range res.Cookies() {
		cookies[c.Name] = lua.LString(c.Value)
	}
	tbl := MakeLTable(l, map[string]lua.LValue{
		"status":  lua.LNumber(res.StatusCode),
		"headers": MakeLTableFromMapOfArr(l, res.Header),
		"body":    lua.LString(body),
		"cookies": MakeLTable(l, cookies),
	})
	SetResponseMetatable(l, tbl, res)
	return tbl
}

// resJSON decodes the response body (res:json()).
func resJSON(l *lua.LState) int {
	self := l.CheckTable(1)
	l.SetTop(0)
	l.Push(l.GetField(self, "body"))
	return jsonDecode(l)
}
