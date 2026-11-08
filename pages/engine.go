// Package pages serves midnight markup pages (interactive, w/ actions that run on
// the server) as http.Handlers, for any router:
//
//	eng := pages.New(pages.Options{})
//	defer eng.Close()
//	mux.Handle("/app", eng.File("ui/app.mn", pages.Props(props)))
//
// A page handler renders on GET (and HEAD), and runs actions on POST, so it
// has to be mounted for both (std mux patterns w/o a method, gin's Any,
// fiber's All). See PAGE_SPLIT.md.
package pages

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	"github.com/taybart/log"
	lua "github.com/taybart/midnight"
	restlua "github.com/taybart/rest/lua"
)

// Options configure an Engine.
type Options struct {
	// MaxStates is the max number of kept page states (one per interactive
	// page load), DefaultMaxStates if 0. When full, the least recently used
	// is closed (and its page reloads on its next action).
	MaxStates int
	// StateTTL is how long unused page states are kept, DefaultStateTTL if 0.
	StateTTL time.Duration
	// Path is prepended to package.path (where `require` looks for modules),
	// ex. "./ui/?.mn;./ui/?.midnight;./ui/?.lua". File pages also look in their own
	// directory first.
	Path string
	// Requester runs request blocks for `rest.request(label [, exports])`,
	// ex. from a .rest file: rest.NewFile("api.rest") then .Requester(client).
	// W/o one, rest.request raises an error.
	Requester restlua.Requester
	// Exports are where every page's rest.exports start (what request blocks
	// see as `exports`), before the page's own (see the Exports option).
	Exports map[string]any
	// OnError is called w/ errors from page scripts (the response is a 500).
	// The default logs them.
	OnError func(r *http.Request, err error)
}

// Engine serves pages. Its pages share its state store (and so MaxStates).
type Engine struct {
	opts   Options
	states *StateStore
}

// New makes an Engine. Close it to close its pages' states.
func New(opts Options) *Engine {
	if opts.OnError == nil {
		opts.OnError = func(r *http.Request, err error) {
			log.Errorf("%s %s: %v\n", r.Method, r.URL.Path, err)
		}
	}
	return &Engine{opts: opts, states: NewStateStore(opts.MaxStates, opts.StateTTL)}
}

// Close closes the states of every page (that isn't handling a request).
func (e *Engine) Close() { e.states.Close() }

// An Option configures a page handler.
type Option func(*handler)

// Props sets the props of a page, by request. A page whose entry returns a
// component (`return (props) -> <html>...</html>`) is rendered w/ them,
// converted to lua (ex. maps and structs become tables, see convert.go).
func Props(fn func(r *http.Request) (any, error)) Option {
	return func(h *handler) { h.props = fn }
}

// ActionURL sets the url a page's actions POST to. By default, it's the
// page's own url as the browser sees it (so it works behind
// http.StripPrefix, router groups, proxies, ...), which has to be routed to
// the page handler. Set it if actions should go elsewhere, ex. a separate
// route to the same handler.
func ActionURL(url string) Option {
	return func(h *handler) { h.actionURL = url }
}

// Exports adds to a page's rest.exports (on top of the engine's Exports),
// by the request that loads the page. They're kept, w/ what rest.request
// adds, for the page's actions (it isn't called again for them).
func Exports(fn func(r *http.Request) (map[string]any, error)) Option {
	return func(h *handler) { h.exports = fn }
}

// LuaFuncs makes fns a module of the page, `require(module)`, a table of the
// functions by name. They're plain gopher-lua functions (see midnight): they
// get their args from the stack, push their results, and raise errors w/
// l.RaiseError / l.ArgError (pcall-able in the page). Request(l) is the
// request being handled (the page load's, or the action's).
func LuaFuncs(module string, fns map[string]lua.LGFunction) Option {
	return func(h *handler) {
		h.modules = append(h.modules, luaModule{name: module, fns: fns})
	}
}

type luaModule struct {
	name string
	fns  map[string]lua.LGFunction
}

// requestKey is where the request being handled is kept, in the registry
// (shared by the state's coroutines).
const requestKey = "pages.request"

// Request returns the request a page's state is handling (the page load's,
// or the action's), for Go functions called by the page (see LuaFuncs). It's
// nil outside of a request.
func Request(l *lua.LState) *http.Request {
	ud, ok := l.GetField(l.Get(lua.RegistryIndex), requestKey).(*lua.LUserData)
	if !ok {
		return nil
	}
	r, _ := ud.Value.(*http.Request)
	return r
}

// setRequest sets the request a state is handling (nil once it's done, so a
// kept state doesn't keep it).
func setRequest(l *lua.LState, r *http.Request) {
	var v lua.LValue = lua.LNil
	if r != nil {
		ud := l.NewUserData()
		ud.Value = r
		v = ud
	}
	l.SetField(l.Get(lua.RegistryIndex), requestKey, v)
}

// File serves the page at path: a midnight (or lua) file that returns a
// component, a markup element, or a response table (ex. a redirect).
// Changes to it (and the modules it requires) show up on the next page load.
func (e *Engine) File(path string, opts ...Option) http.Handler {
	dir := filepath.Dir(path)
	h := &handler{
		engine: e,
		path:   filepath.Join(dir, "?.mn") + ";" + filepath.Join(dir, "?.midnight") + ";" + filepath.Join(dir, "?.lua"),
		load: func(l *lua.LState) error {
			fn, err := l.LoadFile(path)
			if err != nil {
				return err
			}
			l.Push(fn)
			return l.PCall(0, 1, nil)
		},
	}
	return h.with(opts)
}

// Source serves a page from midnight source, which returns a component, a markup
// element, or a response table (ex. a redirect).
func (e *Engine) Source(src string, opts ...Option) http.Handler {
	h := &handler{
		engine: e,
		load: func(l *lua.LState) error {
			return restlua.Exec(l, src, restlua.LangMidnight)
		},
	}
	return h.with(opts)
}

type handler struct {
	engine *Engine
	// package.path of the page's own modules (before the engine's Path)
	path string
	// load runs the page's entry, leaving what it returned on the stack
	load      func(l *lua.LState) error
	props     func(r *http.Request) (any, error)
	exports   func(r *http.Request) (map[string]any, error)
	actionURL string // "" for the page's own url
	modules   []luaModule
}

func (h *handler) with(opts []Option) *handler {
	for _, opt := range opts {
		opt(h)
	}
	return h
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var res Response
	var err error
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		res, err = h.render(r)
	case http.MethodPost:
		res, err = h.action(r)
	default:
		w.Header().Set("Allow", "GET, HEAD, POST")
		res = jsonError(http.StatusMethodNotAllowed, "method not allowed")
	}
	if err != nil {
		// script errors are the server's (and only logged: they can have
		// script source in them)
		h.engine.opts.OnError(r, err)
		res = Response{Status: http.StatusInternalServerError}
	}
	res.Write(w)
}

// setup sets up a new page state for the request that loads the page.
func (h *handler) setup(l *lua.LState, r *http.Request, stateID string) error {
	if err := restlua.RegisterModules(l); err != nil {
		return err
	}
	for _, m := range h.modules {
		l.PreloadModule(m.name, func(l *lua.LState) int {
			mod := l.CreateTable(0, len(m.fns))
			for name, fn := range m.fns {
				l.SetField(mod, name, l.NewFunction(fn))
			}
			l.Push(mod)
			return 1
		})
	}
	if err := request(l, r, stateID); err != nil {
		return err
	}
	rest := l.GetGlobal("rest").(*lua.LTable)
	restlua.SetRequester(l, rest, h.engine.opts.Requester)
	exports := restlua.MapToLTable(l, h.engine.opts.Exports)
	if h.exports != nil {
		own, err := h.exports(r)
		if err != nil {
			return fmt.Errorf("exports: %w", err)
		}
		restlua.MapToLTable(l, own).ForEach(func(k, v lua.LValue) { exports.RawSet(k, v) })
	}
	l.SetField(rest, "exports", exports)
	path := ""
	for _, p := range []string{h.path, h.engine.opts.Path} {
		if p != "" {
			path += p + ";"
		}
	}
	if path != "" {
		pkg := l.GetGlobal("package")
		l.SetField(pkg, "path", lua.LString(path+lua.LVAsString(l.GetField(pkg, "path"))))
	}
	return nil
}

// request sets the `rest` global for a request (a kept state keeps its rest
// table, w/ its exports and rest.request), and the request for Request.
func request(l *lua.LState, r *http.Request, stateID string) error {
	setRequest(l, r)
	if err := restlua.SetRestGlobal(l, r); err != nil {
		return err
	}
	l.SetField(l.GetGlobal("rest"), "state", lua.LString(stateID))
	return nil
}

// render renders the page in a new state, which is kept if it has actions.
func (h *handler) render(r *http.Request) (Response, error) {
	id, st := h.engine.states.New()
	keep := false
	l := st.L
	defer func() {
		if keep {
			setRequest(l, nil)
		} else {
			h.engine.states.Discard(id, st)
		}
		h.engine.states.Release(st)
	}()

	if err := h.setup(l, r, id); err != nil {
		return Response{}, err
	}
	l.SetTop(0)
	if err := h.load(l); err != nil {
		return Response{}, err
	}
	page := l.Get(1)
	if fn, ok := page.(*lua.LFunction); ok {
		var props lua.LValue = lua.LNil
		if h.props != nil {
			p, err := h.props(r)
			if err != nil {
				return Response{}, fmt.Errorf("props: %w", err)
			}
			if props, err = toLuaValue(l, p); err != nil {
				return Response{}, fmt.Errorf("props: %w", err)
			}
		}
		if err := l.CallByParam(lua.P{Fn: fn, NRet: 1, Protect: true}, props); err != nil {
			return Response{}, err
		}
		page = l.Get(-1)
	}
	if _, ok := page.(*lua.LTable); !ok {
		return Response{}, fmt.Errorf("page must return a component, markup element, or response table%s", returnedHint(page))
	}
	mounted, err := Mount(l, page, h.actionURL, id)
	if err != nil {
		return Response{}, err
	}
	res, err := MapResult(l, mounted)
	if err != nil {
		return Response{}, err
	}
	keep, err = IsInteractive(l)
	return res, err
}

// action runs an action in the page's kept state.
func (h *handler) action(r *http.Request) (Response, error) {
	id := r.Header.Get(StateHeader)
	if id == "" {
		return jsonError(http.StatusBadRequest, "missing "+StateHeader+" header"), nil
	}
	st, err := h.engine.states.Get(id)
	if gone := (ErrStateGone{}); errors.As(err, &gone) {
		// the client reloads the page
		return jsonError(http.StatusGone, gone.Error()), nil
	} else if err != nil {
		return Response{}, err
	}
	l := st.L
	defer func() {
		setRequest(l, nil)
		h.engine.states.Release(st)
	}()

	if err := request(l, r, id); err != nil {
		return Response{}, err
	}
	l.SetTop(0)
	if err := restlua.Exec(l, DispatchScript, restlua.LangLua); err != nil {
		return Response{}, err
	}
	return MapResult(l, l.Get(-1))
}

func jsonError(status int, msg string) Response {
	body, _ := json.Marshal(map[string]string{"error": msg})
	res := Response{Status: status, Body: body}
	res.SetDefaultHeader("Content-Type", "application/json")
	return res
}

// returnedHint describes what a script returned instead of a page.
func returnedHint(v lua.LValue) string {
	if v.Type() == lua.LTNil {
		return " (got nothing, is the return missing?)"
	}
	return fmt.Sprintf(" (got a %s)", v.Type())
}
