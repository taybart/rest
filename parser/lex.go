package parser

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

/*
label LABEL
DELAY
URL
HEADER
METHOD {path}
BODY
EXPECTATION
*/

type Token int

const (
	ILLEGAL Token = iota
	EOF

	COMMENT
	LABEL
	VARIABLE
	HEADER
	EXPECTATION

	IDENT
	ASSIGN
	DECL

	METHOD
	URL
	BODY
	BLOCK_END
	TEXT
)

func (t Token) String() string {
	return []string{
		"ILLEGAL",
		"EOF",
		"COMMENT",
		"LABEL",
		"VARIABLE",
		"HEADER",
		"EXPECTATION",
		"IDENT",
		"ASSIGN",
		"DECL",
		"METHOD",
		"URL",
		"BODY",
		"BLOCK_END",
		"TEXT",
	}[t]
}

const (
	eof        = rune(0)
	comment    = "#"
	label      = "label "
	variable   = "set "
	delay      = "delay "
	expectaion = "expect "
	blockEnd   = "---"
)

type stateFn func() stateFn

type item struct {
	token Token
	value string
}

type Lexer struct {
	name  string // used only for error reports
	input string // the string being scanned
	start int    // start position of this item
	pos   int    // current position in the input
	width int    // width of last rune read
	items chan item
}

func newLexer(input string) Lexer {
	return Lexer{
		input: input,
		items: make(chan item),
	}
}

// Parse : Get all parts of request from request block
func (l *Lexer) Run() {
	done := make(chan bool)
	for state := l.lexText; state != nil; {
		state = state()
	}
	close(l.items) // we are done
	<-done
}

func (l *Lexer) lexText() stateFn {
	for {
		if strings.HasPrefix(l.input[l.pos:], blockEnd) {
			return l.lexBlockEnd
		} else if strings.HasPrefix(l.input[l.pos:], comment) {
			return l.lexComment
		} else if strings.HasPrefix(l.input[l.pos:], label) {
			return l.lexLabel
		} else if strings.HasPrefix(l.input[l.pos:], variable) {
			return l.lexVariableAssignment
			// } else if hasMethodPrefix(l.input[l.pos:]) {
			// 	return l.lexMethod
		} else if isUrl(l.peekToNewLine()) {
			return l.lexUrl
		}

		ch := l.next()

		if ch == eof {
			break
		}
	}

	if l.pos > l.start {
		l.emit(TEXT)
	}
	l.emit(EOF)

	return nil
}

func (l *Lexer) lexBlockEnd() stateFn {
	l.pos += len(blockEnd)
	l.acceptToNewline()
	l.emit(BLOCK_END)
	return l.lexText
}

func (l *Lexer) lexComment() stateFn {
	l.pos += len(comment)
	l.ignore()

	l.acceptToNewline()

	l.emit(COMMENT)

	return l.lexText
}

func (l *Lexer) lexLabel() stateFn {
	l.pos += len(label)
	l.emit(LABEL)

	l.acceptToNewline()
	l.emit(IDENT)
	return l.lexText
}

func (l *Lexer) lexVariableAssignment() stateFn {
	l.pos += len(variable)
	l.ignore()
	l.emit(VARIABLE)

	for {
		ch := l.peek()
		if isWhitespace(ch) {
			l.emit(IDENT)
			l.skipWhitespace() // skip space
			break
		}
		l.next()
	}

	l.acceptToNewline()
	l.emit(ASSIGN)
	return l.lexText
}

func (l *Lexer) lexUrl() stateFn {
	l.acceptToNewline()
	l.emit(URL)
	l.skip()
	return l.lexHeader
}

func (l *Lexer) lexHeader() stateFn {
	colonSeen := false
	for {
		ch := l.peek()
		if ch == ':' {
			l.emit(HEADER)
			l.skip()
			colonSeen = true
			l.skipWhitespace()
		}
		if isEnd(ch) {
			if colonSeen {
				l.emit(ASSIGN)
				return l.lexHeader
			}
			return l.lexMethod
		}
		l.next()
	}
}

func (l *Lexer) lexMethod() stateFn {
	l.acceptToNewline()

	l.emit(METHOD)

	return l.lexBody
}

func (l *Lexer) lexBody() stateFn {
	for {
		if strings.HasPrefix(l.input[l.pos:], blockEnd) {
			return l.lexBlockEnd
		}
		if strings.HasPrefix(l.input[l.pos:], expectaion) {
			return l.lexExpectation
		}
		ch := l.next()
		if ch == eof {
			return nil
		}
	}

	l.emit(BODY)

	return l.lexBody
}
func (l *Lexer) lexExpectation() stateFn {
	l.pos += len(expectaion)

	l.skipWhitespace()

	l.acceptToNewline()
	l.emit(EXPECTATION)

	return l.lexText
}

func (l *Lexer) emit(t Token) {
	l.items <- item{t, l.input[l.start:l.pos]}
	l.start = l.pos
}

func (l *Lexer) next() (r rune) {
	if l.pos >= len(l.input) { // are we done with the input?
		l.width = 0
		return eof
	}
	r, l.width = utf8.DecodeRuneInString(l.input[l.pos:]) // Advance uno mas
	l.pos += l.width
	return r
}

func (l *Lexer) ignore() {
	l.start = l.pos
}

func (l *Lexer) backup() {
	l.pos -= l.width
}

func (l *Lexer) peek() rune {
	r := l.next()
	l.backup()
	return r
}

func (l *Lexer) skip() {
	l.next()
	l.ignore()
}

func (l *Lexer) skipWhitespace() {
	for {
		ch := l.peek()
		if !isWhitespace(ch) {
			break
		}
		l.next()
	}
	l.ignore()
}

func (l *Lexer) accept(valid string) bool { // Allow {{ valid }} chars
	if strings.IndexRune(valid, l.next()) >= 0 {
		return true
	}
	l.backup()
	return false
}
func (l *Lexer) errorf(format string, args ...interface{}) stateFn {
	l.items <- item{
		ILLEGAL,
		fmt.Sprintf(format, args...),
	}
	return nil
}

func (l *Lexer) peekToNewLine() string {
	var s string
	for {
		ch := l.next()
		if isEnd(ch) {
			break
		} else {
			s += string(ch)
		}
	}
	l.backup()
	return s
}

func (l *Lexer) acceptToNewline() {
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
