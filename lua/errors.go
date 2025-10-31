package restlua

import (
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/taybart/log"
	lua "github.com/yuin/gopher-lua"
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
