package restlua_test

import (
	"errors"
	"testing"

	restlua "github.com/taybart/rest/lua"
)

func TestFail(t *testing.T) {
	tests := []struct {
		name, lang, code string
		usage            string
		want             restlua.Failure
	}{
		{
			name: "stops the script", lang: "midnight",
			code: `fail('nope [x]'); error('not reached')`,
			want: restlua.Failure{Msg: "nope [x]", Code: 1},
		},
		{
			name: "exit code", lang: "lua",
			code: `fail('nope', 3)`,
			want: restlua.Failure{Msg: "nope", Code: 3},
		},
		{
			name: "usage", lang: "midnight", usage: "usage: x <y>",
			code: `fail.usage('no y')`,
			want: restlua.Failure{Msg: "no y", Code: 2, Usage: "usage: x <y>"},
		},
		{
			name: "just the usage", lang: "lua", usage: "usage: x <y>",
			code: `fail.usage()`,
			want: restlua.Failure{Code: 2, Usage: "usage: x <y>"},
		},
		{
			name: "inside a function", lang: "midnight",
			code: `local function check(v) { if !v { fail('missing') } }; check(nil)`,
			want: restlua.Failure{Msg: "missing", Code: 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := newLuaState(t)
			restlua.RegisterFail(l, tt.usage)
			err := restlua.Exec(l, tt.code, tt.lang)
			var f *restlua.Failure
			if !errors.As(err, &f) {
				t.Fatalf("expected a *Failure, got %T: %v", err, err)
			}
			if *f != tt.want {
				t.Fatalf("expected %+v, got %+v", tt.want, *f)
			}
		})
	}
}

func TestFailCaught(t *testing.T) {
	l := newLuaState(t)
	restlua.RegisterFail(l, "")
	code := `
    local e = nil
    fail('caught') catch err { e = tostring(err) }
    if e != 'caught' { error("expected 'caught', got " .. tostring(e)) }
  `
	if err := restlua.Exec(l, code, "midnight"); err != nil {
		t.Fatal(err)
	}
}

// without a usage there's no fail.usage (only the cli has one)
func TestFailNoUsage(t *testing.T) {
	l := newLuaState(t)
	restlua.RegisterFail(l, "")
	if err := restlua.Exec(l, `if fail.usage != nil { error('fail.usage is set') }`, "midnight"); err != nil {
		t.Fatal(err)
	}
}
