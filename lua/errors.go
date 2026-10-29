package restlua

import (
	"errors"
	"regexp"
	"strconv"
	"strings"

	lua "github.com/taybart/go-erde"
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

// Exec runs code as lua or erde (https://erde-lang.github.io)
func Exec(l *lua.LState, code, lang string) error {
	var err error
	if lang == LangErde {
		err = l.DoErdeString(code)
	} else {
		err = l.DoString(code)
	}
	if err != nil {
		return FmtError(code, err)
	}
	return nil
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
