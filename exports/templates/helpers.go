package templates

import (
	"strings"
	"unicode"
)

// ident turns a label into a lowerCamelCase identifier that is valid in go,
// javascript and lua, "get-thing 2" -> "getThing2"
func ident(label string) string {
	words := strings.FieldsFunc(label, func(r rune) bool {
		return !(r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)))
	})
	for i, w := range words {
		if i == 0 {
			words[i] = strings.ToLower(w[:1]) + w[1:]
		} else {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	id := strings.Join(words, "")
	if id == "" || unicode.IsDigit(rune(id[0])) {
		id = "request" + id
	}
	return id
}
