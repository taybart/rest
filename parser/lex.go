package parser

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

/*

take input
split by \n
for lines
	scan for vars
	lex

*/

/*
label LABEL
DELAY
URL
HEADER
METHOD {path}
BODY
EXPECTATION
*/

type stateFn func() stateFn

type item struct {
	token Token
	value string
}

type lexer struct {
	name  string // used only for error reports
	input string // the string being scanned
	start int    // start position of this item
	pos   int    // current position in the input
	width int    // width of last rune read
	items chan item
	cmd   chan string
}

func newLexer(input string) lexer {
	return lexer{
		input: input,
		items: make(chan item),
		cmd:   make(chan string),
	}
}

// Parse : Get all parts of request from request block
func (l *lexer) Run() {
	done := make(chan bool)
	for state := l.lexLine; state != nil; {
		state = state()
	}
	close(l.items) // we are done
	<-done
}

func (l *lexer) emit(t Token) {
	l.items <- item{t, l.input[l.start:l.pos]}
	l.start = l.pos
}

func (l *lexer) next() (r rune) {
	if l.pos >= len(l.input) { // are we done with the input?
		l.width = 0
		return eof
	}
	r, l.width = utf8.DecodeRuneInString(l.input[l.pos:]) // Advance uno mas
	l.pos += l.width
	return r
}

func (l *lexer) replace(s string, start, end int) {
	r := []rune(l.input)
	r = append(r[:start], r[end:]...)

	r = append(r[:start], append([]rune(s), r[start:]...)...)

	l.input = string(r)
	// fmt.Println(l.input[start-10 : start+len(s)+5])
}

func (l *lexer) ignore() {
	l.start = l.pos
}

func (l *lexer) backup() {
	l.pos -= l.width
}

func (l *lexer) peek() rune {
	r := l.next()
	l.backup()
	return r
}

func (l *lexer) skip() {
	l.next()
	l.ignore()
}

func (l *lexer) skipWhitespace() {
	for {
		ch := l.peek()
		if !isWhitespace(ch) {
			break
		}
		l.next()
	}
	l.ignore()
}

func (l *lexer) accept(valid string) bool { // Allow {{ valid }} chars
	if strings.IndexRune(valid, l.next()) >= 0 {
		return true
	}
	l.backup()
	return false
}
func (l *lexer) errorf(format string, args ...interface{}) stateFn {
	l.items <- item{
		ILLEGAL,
		fmt.Sprintf(format, args...),
	}
	return nil
}

func (l *lexer) peekToNewLine() string {
	var s string
	start := l.pos
	for {
		ch := l.next()
		if isEnd(ch) {
			break
		} else {
			s += string(ch)
		}
	}
	l.pos = start
	return s
}

func (l *lexer) acceptToNewline() {
	// l.scanForVariables(false)
	for {
		ch := l.peek()
		if isEnd(ch) {
			return
		}
		l.next()
	}
}

func hasMethodPrefix(s string) bool {
	rxMethod := regexp.MustCompile(`^(OPTIONS|GET|POST|PUT|PATCH|DELETE)`)
	return rxMethod.MatchString(s)
}

// isUrl tests a string to determine if it is a well-structured url or not.
func isUrl(s string) bool {
	if strings.HasPrefix(s, "http") {
		return true
	}
	if s == "" {
		return false
	}
	// checks needed as of Go 1.6 because of change:
	// https://github.com/golang/go/commit/617c93ce740c3c3cc28cdd1a0d712be183d0b328#diff-6c2d018290e298803c0c9419d8739885L195
	// emulate browser and strip the '#' suffix prior to validation. see issue-#237
	if i := strings.Index(s, "#"); i > -1 {
		s = s[:i]
	}

	if len(s) == 0 {
		return false
	}

	url, err := url.ParseRequestURI(s)
	if err != nil || url.Scheme == "" || url.Host == "" {
		return false
	}
	return true
}

func hasVariablePrefix(s string) bool {
	return strings.HasPrefix(s, variablePrefix)
}

func isWhitespace(ch rune) bool {
	return ch == ' ' || ch == '\t' //|| ch == '\n'
}

func isEnd(ch rune) bool {
	return ch == '\n' || ch == eof
}

/*

state should just ++ after \n since we are in line by line mode // maybe not body params

type lexer struct {
	name string // used only for error reports
	input string // the string being scanned
	start int // start position of this item
	pos int // current position in the input
	width int // width of last rune read
	items chan item // channel of scanned items
}

func lex (name, input string) (*lexer, chan item) {
	l := &lexer{
		name: name,
		input: input,
		items: make(chan item),
	}
	go l.run()
	return l, l.items
}

func (l *lexer) run() {
	for state := lextText; state != nil {
		state = state(l)
	}
	close(l.items) // we are done
}

func (l *lexer) emit(t itemType) {
	l.items <- item{t, l.input[l.start:l.pos]}
	l.start = l.pos
}

func (l *lexer) next() (rune int){
	if l.pos >= len(l.input) { // are we done with the input?
		l.width = 0
		return eof
	}
	rune, l.width = utf8.DecodeRuneInstring(l.input[l.pos:]) // Advance uno mas
	l.pos += l.width
	return rune
}

func (l *lexer) ignore() {
	l.start = l.pos
}
func (l *lexer) backup() {
	l.pos -= l.width
}
func (l *lexer) peek() int { // i don't want to advance but it need the input
	rune := l.next()
	l.backup()
	return rune
}
func (l *lexer) accept(valid string) bool { // Allow {{ valid }} chars
		if strings.IndexRune(valid, l.next()) >= 0 {
			return true
		}
		l.backup()
		return false
}
func (l *lexer) errorf(format string, args ...interface{}) stateFn { // Allow {{ valid }} chars
	l.items <- item{
		itemError,
		fmt.Sprintf(format, args...)
	}
	return nil
}
*/
