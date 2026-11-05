package templates

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// lang knows how to write values as literals in one target language. Only
// str is required, a language without mapOpen can't represent maps/lists.
type lang struct {
	name string
	// indent added per nesting level when pretty printing
	unit string
	str  func(string) string
	null string
	// mapOpen/listOpen get the go type so typed languages (go) can name it
	mapOpen   func(reflect.Type) string
	mapClose  string
	listOpen  func(reflect.Type) string
	listClose string
	key       func(string) string
	padBraces bool // `{ a = 1 }` vs `{a: 1}` on one line
	// num rewrites json number text when it isn't a valid literal as is
	num func(string) string
	// noNUL for languages that can't hold a NUL byte in a string at all
	noNUL bool
}

var langs = map[string]lang{
	"sh": {
		name:  "sh",
		str:   shStr,
		null:  "''",
		noNUL: true,
	},
	"go": {
		name:      "go",
		unit:      "\t",
		str:       goStr,
		null:      "nil",
		mapOpen:   func(t reflect.Type) string { return goType(t) + "{" },
		mapClose:  "}",
		listOpen:  func(t reflect.Type) string { return goType(t) + "{" },
		listClose: "}",
		key:       func(k string) string { return strconv.Quote(k) + ": " },
		num:       goNum,
	},
	"js": {
		name:      "js",
		unit:      "  ",
		str:       jsStr,
		null:      "null",
		mapOpen:   func(reflect.Type) string { return "{" },
		mapClose:  "}",
		listOpen:  func(reflect.Type) string { return "[" },
		listClose: "]",
		key:       func(k string) string { return jsStr(k) + ": " },
		padBraces: true,
	},
	"lua": {
		name:      "lua",
		unit:      "  ",
		str:       luaStr,
		null:      "nil",
		mapOpen:   func(reflect.Type) string { return "{" },
		mapClose:  "}",
		listOpen:  func(reflect.Type) string { return "{" },
		listClose: "}",
		key:       luaKey,
		padBraces: true,
	},
}

// langFromName picks the language from a template file name,
// "harmony.lua.tmpl" or "harmony.lua" -> lua
func langFromName(name string) string {
	base := strings.TrimSuffix(filepath.Base(name), ".tmpl")
	return strings.TrimPrefix(filepath.Ext(base), ".")
}

func knownLangs() string {
	names := make([]string, 0, len(langs))
	for n := range langs {
		names = append(names, n)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// litFunc returns the `lit` template function for a template file
//
//	{{lit .URL}}           -> a string literal
//	{{lit .Headers}}       -> a map literal on one line
//	{{lit .JSON "    "}}   -> pretty printed, continuation lines start with "    "
func litFunc(templateName string) func(v any, indent ...string) (string, error) {
	return func(v any, indent ...string) (string, error) {
		name := langFromName(templateName)
		l, ok := langs[name]
		if !ok {
			return "", fmt.Errorf("lit: can't tell what language %q is for, name it like foo.<lang>.tmpl (known: %s)",
				filepath.Base(templateName), knownLangs())
		}
		if len(indent) > 1 {
			return "", fmt.Errorf("lit: takes a value and an optional indent")
		}
		e := encoder{lang: l, pretty: len(indent) == 1}
		if e.pretty {
			e.prefix = indent[0]
		}
		return e.encode(reflect.ValueOf(v), e.prefix)
	}
}

type encoder struct {
	lang
	pretty bool
	prefix string
}

var jsonNumber = reflect.TypeFor[json.Number]()

func (e encoder) encode(v reflect.Value, prefix string) (string, error) {
	for v.IsValid() && (v.Kind() == reflect.Interface || v.Kind() == reflect.Pointer) {
		if v.IsNil() {
			return e.null, nil
		}
		v = v.Elem()
	}
	if !v.IsValid() {
		return e.null, nil
	}
	if v.Type() == jsonNumber {
		if e.num != nil {
			return e.num(v.String()), nil
		}
		return v.String(), nil
	}

	switch v.Kind() {
	case reflect.String:
		if e.noNUL && strings.ContainsRune(v.String(), 0) {
			return "", fmt.Errorf("lit: %s strings can't contain a NUL byte", e.name)
		}
		return e.str(v.String()), nil
	case reflect.Bool:
		return strconv.FormatBool(v.Bool()), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(v.Uint(), 10), nil
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(v.Float(), 'g', -1, 64), nil
	case reflect.Map:
		if e.mapOpen == nil {
			return "", fmt.Errorf("lit: %s can't represent a map", e.name)
		}
		if v.Type().Key().Kind() != reflect.String {
			return "", fmt.Errorf("lit: map keys must be strings, got %s", v.Type().Key())
		}
		keys := v.MapKeys()
		sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
		items := make([]string, 0, len(keys))
		for _, k := range keys {
			val, err := e.encode(v.MapIndex(k), prefix+e.unit)
			if err != nil {
				return "", err
			}
			items = append(items, e.key(k.String())+val)
		}
		return e.wrap(e.mapOpen(v.Type()), e.mapClose, items, prefix), nil
	case reflect.Slice, reflect.Array:
		if e.listOpen == nil {
			return "", fmt.Errorf("lit: %s can't represent a list", e.name)
		}
		items := make([]string, 0, v.Len())
		for i := range v.Len() {
			val, err := e.encode(v.Index(i), prefix+e.unit)
			if err != nil {
				return "", err
			}
			items = append(items, val)
		}
		return e.wrap(e.listOpen(v.Type()), e.listClose, items, prefix), nil
	}
	return "", fmt.Errorf("lit: can't write a %s as %s", v.Type(), e.name)
}

func (e encoder) wrap(open, close string, items []string, prefix string) string {
	if len(items) == 0 {
		return open + close
	}
	if e.pretty {
		var b strings.Builder
		b.WriteString(open + "\n")
		for _, it := range items {
			b.WriteString(prefix + e.unit + it + ",\n")
		}
		b.WriteString(prefix + close)
		return b.String()
	}
	if e.padBraces {
		return open + " " + strings.Join(items, ", ") + " " + close
	}
	return open + strings.Join(items, ", ") + close
}

// sh

func shStr(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// go

// goStr prefers a raw string when it saves escaping (json bodies), falling
// back to an interpreted literal for anything a raw string can't hold
func goStr(s string) string {
	if strings.ContainsAny(s, "\"\\\n") && canBeRaw(s) {
		return "`" + s + "`"
	}
	return strconv.Quote(s)
}

func canBeRaw(s string) bool {
	if !utf8.ValidString(s) || strings.ContainsAny(s, "`\r") {
		return false
	}
	for _, r := range s {
		if r != '\n' && r != '\t' && !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}

// goNum keeps integers that fit in an int, anything bigger has to be a float
// constant or it won't compile into an any (json decodes it as a float64 too)
func goNum(n string) string {
	if strings.ContainsAny(n, ".eE") {
		return n
	}
	if _, err := strconv.ParseInt(n, 10, 64); err == nil {
		return n
	}
	return n + ".0"
}

func goType(t reflect.Type) string {
	return strings.ReplaceAll(t.String(), "interface {}", "any")
}

// js

func jsStr(s string) string {
	b, _ := json.Marshal(s) // a string always marshals
	// json.Marshal html-escapes <, > and &, which is valid js but noisy
	r := strings.NewReplacer(`<`, "<", `>`, ">", `&`, "&")
	return r.Replace(string(b))
}

// lua, written for 5.1 (luajit, gopher-lua) so only \ddd numeric escapes

func luaStr(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if c < 0x20 || c == 0x7f {
				// always 3 digits so a following digit isn't swallowed
				fmt.Fprintf(&b, `\%03d`, c)
			} else {
				b.WriteByte(c)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

var luaIdent = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

var luaKeywords = map[string]bool{
	"and": true, "break": true, "do": true, "else": true, "elseif": true,
	"end": true, "false": true, "for": true, "function": true, "goto": true,
	"if": true, "in": true, "local": true, "nil": true, "not": true, "or": true,
	"repeat": true, "return": true, "then": true, "true": true, "until": true,
	"while": true,
}

func luaKey(k string) string {
	if luaIdent.MatchString(k) && !luaKeywords[k] {
		return k + " = "
	}
	return "[" + luaStr(k) + "] = "
}
