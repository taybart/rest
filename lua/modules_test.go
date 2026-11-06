package restlua

import (
	"testing"

	lua "github.com/taybart/go-erde"
)

// The lua modules are globals that load on first use.
func TestLazyModules(t *testing.T) {
	a, b := NewState(), NewState()
	defer a.Close()
	defer b.Close()
	for _, l := range []*lua.LState{a, b} {
		if err := RegisterModules(l); err != nil {
			t.Fatal(err)
		}
	}

	err := a.DoString(`
		assert(rawget(_G, 'inspect') == nil, 'loaded before use')
		assert(inspect({ 1 }) == '{ 1 }', 'inspect')
		assert(rawget(_G, 'inspect') ~= nil, 'not cached')
		assert(type(colors) == 'table' and type(tools.expected) == 'function', 'colors / tools')
		assert(nope == nil, 'other globals')
		inspect = 'mine'
		assert(inspect == 'mine', 'not overridable')
	`)
	if err != nil {
		t.Fatal(err)
	}

	// each state gets its own module
	if err := b.DoString(`assert(type(inspect) == 'table', 'shared between states')`); err != nil {
		t.Fatal(err)
	}
}

// Scripts are compiled once, but run in each state on their own.
func TestExecCachesScripts(t *testing.T) {
	code := `n = (n or 0) + 1; return n`
	a, b := NewState(), NewState()
	defer a.Close()
	defer b.Close()
	for i, l := range []*lua.LState{a, a, b} {
		if err := Exec(l, code, LangLua); err != nil {
			t.Fatal(err)
		}
		want := []string{"1", "2", "1"}[i]
		if got := l.Get(-1).String(); got != want {
			t.Errorf("run %d: got %s, want %s", i, got, want)
		}
	}
	scripts.Lock()
	cached := scripts.protos[scriptKey{code: code, lang: LangLua}] != nil
	scripts.Unlock()
	if !cached {
		t.Error("not cached")
	}

	// errors are still reported (and not cached)
	for range 2 {
		if err := Exec(a, `local = 1`, LangLua); err == nil {
			t.Error("expected a syntax error")
		}
	}
}
