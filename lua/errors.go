package restlua

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"sync"

	lua "github.com/taybart/go-erde"
	"github.com/taybart/go-erde/parse"
	"github.com/taybart/log"
)

func extractLineNumber(errMsg string) int {
	re := regexp.MustCompile(`<string>:(\d+)`)
	matches := re.FindStringSubmatch(errMsg)
	if len(matches) > 1 {
		ret, err := strconv.Atoi(matches[1])
		if err != nil {
			log.Error(err)
			return -1
		}
		return ret
	}
	return -1
}
func getLineOfCode(code string, lineNum int) string {
	lines := strings.Split(code, "\n")
	if lineNum > 0 && lineNum <= len(lines) {
		return strings.TrimSpace(lines[lineNum-1]) // lineNum is 1-indexed
	}
	return ""
}
func extractErrorMessage(fullError string) string {
	re := regexp.MustCompile(`^<[^>]+>:\d+:\s*(.+?)(?:\nstack traceback:|\z)`)
	matches := re.FindStringSubmatch(fullError)

	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}

	// Fallback: just remove the first line if regex doesn't match
	lines := strings.Split(fullError, "\n")
	if len(lines) > 0 {
		return strings.TrimSpace(lines[0])
	}

	return fullError
}

const (
	LangLua  = "lua"
	LangErde = "erde"
)

// NewState makes a runtime w/ the experimental erde extensions on (only affects
// erde scripts, ex. `match`)
func NewState() *lua.LState {
	return lua.NewState(lua.Options{Erde: parse.ErdeOptions{Match: true, Markup: true}})
}

// SetPrint makes lua's print write to w instead of stdout
func SetPrint(l *lua.LState, w io.Writer) {
	l.SetGlobal("print", l.NewFunction(func(l *lua.LState) int {
		top := l.GetTop()
		for i := 1; i <= top; i++ {
			if i > 1 {
				fmt.Fprint(w, "\t")
			}
			fmt.Fprint(w, l.ToStringMeta(l.Get(i)).String())
		}
		fmt.Fprintln(w)
		return 0
	}))
}

// Exec runs code as lua or erde (https://erde-lang.github.io)
func Exec(l *lua.LState, code, lang string) error {
	fn, err := loadScript(l, code, lang)
	if err == nil {
		l.Push(fn)
		err = l.PCall(0, lua.MultRet, nil)
	}
	if err != nil {
		return FmtError(code, err)
	}
	return nil
}

// Scripts (handler fns, middleware, hooks, etc.) are compiled once: their
// (immutable) compiled code is shared by every state that runs them. The
// cache is bounded, in case code is ever generated.
type scriptKey struct {
	code, lang string
	erde       parse.ErdeOptions
}

const scriptsMax = 256

var scripts struct {
	sync.Mutex
	protos map[scriptKey]*lua.FunctionProto
}

func loadScript(l *lua.LState, code, lang string) (*lua.LFunction, error) {
	key := scriptKey{code: code, lang: lang}
	if lang == LangErde {
		key.erde = l.Options.Erde
	}
	scripts.Lock()
	proto := scripts.protos[key]
	scripts.Unlock()
	if proto != nil {
		return l.NewFunctionFromProto(proto), nil
	}

	var fn *lua.LFunction
	var err error
	if lang == LangErde {
		fn, err = l.LoadErdeString(code)
	} else {
		fn, err = l.LoadString(code)
	}
	if err != nil {
		return nil, err
	}

	scripts.Lock()
	if scripts.protos == nil || len(scripts.protos) >= scriptsMax {
		scripts.protos = map[scriptKey]*lua.FunctionProto{}
	}
	scripts.protos[key] = fn.Proto
	scripts.Unlock()
	return fn, nil
}

// Quote makes a single quoted string literal, valid in both lua and erde
// (erde interpolates {} in double quoted strings)
func Quote(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `'`, `\'`, "\n", `\n`, "\r", `\r`, "\x00", `\0`)
	return "'" + r.Replace(s) + "'"
}

func FmtError(code string, err error) error {
	if apiErr, ok := err.(*lua.ApiError); ok {
		errMsg := apiErr.Object.String()
		lineNum := extractLineNumber(errMsg)
		var msg strings.Builder
		msg.WriteString(extractErrorMessage(errMsg))
		if lineNum != -1 {
			msg.WriteString("\nline " + strconv.Itoa(lineNum))
			loc := getLineOfCode(code, lineNum)
			if loc != "" {
				msg.WriteString(" -> " + loc)
			}
		}
		return errors.New(msg.String())
	}
	return err
}
