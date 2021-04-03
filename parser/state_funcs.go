package parser

import (
	"fmt"
	"strings"
)

// lexLine
func (l *lexer) lexLine() stateFn {
	l.scanForVariables()
	for {
		if strings.HasPrefix(l.input[l.pos:], blockEnd) {
			return l.lexBlockEnd
		} else if strings.HasPrefix(l.input[l.pos:], comment) {
			return l.lexComment
		} else if strings.HasPrefix(l.input[l.pos:], label) {
			return l.lexLabel
		} else if strings.HasPrefix(l.input[l.pos:], variable) {
			return l.lexVariableAssignment
		} else if isUrl(l.peekToNewLine()) {
			return l.lexUrl
		}

		ch := l.next()
		if ch == '\n' {
			return l.lexLine
		}
		if ch == eof {
			break
		}
	}

	if l.pos > l.start {
		l.emit(TEXT)
	}
	l.emit(EOF)
	fmt.Println("~~~~~~~~~~~~")
	fmt.Println(l.input)

	return nil
}

func (l *lexer) scanForVariables() {
	start := l.pos
	varstart := l.start
	// end := l.pos
	varExists := false
	for {
		if strings.HasPrefix(l.input[l.pos:], variablePrefix) {
			varstart = l.pos
			l.pos += len(variablePrefix)
			l.start = l.pos
			varExists = true
		}

		ch := l.peek()
		if ch == variableClose && varExists {
			l.emit(VAR_REQUEST)
			l.next()
			// l.pos = start
			val := <-l.cmd
			// fmt.Printf("%sreplacing %s%s\n", log.Red, val, log.Rtd)
			l.replace(val, varstart, l.pos)
			varExists = false
			break
		}
		if isEnd(ch) {
			l.pos = start
			break
		}
		l.next()
	}
	l.start = start
	l.pos = start
}

func (l *lexer) lexComment() stateFn {
	l.pos += len(comment)
	l.ignore()

	l.acceptToNewline()

	l.emit(COMMENT)
	l.skip() // skip new line

	return l.lexLine
}

func (l *lexer) lexLabel() stateFn {
	l.pos += len(label)
	l.emit(LABEL)

	l.acceptToNewline()
	l.emit(IDENT)
	l.skip() // skip new line
	return l.lexLine
}

func (l *lexer) lexVariableAssignment() stateFn {
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
	l.skip() // skip new line
	return l.lexLine
}

func (l *lexer) lexUrl() stateFn {
	l.acceptToNewline()
	l.emit(URL)
	l.skip() // skip new line
	return l.lexHeader
}

func (l *lexer) lexHeader() stateFn {
	colonSeen := false

	l.scanForVariables()
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
			l.skip() // skip new lint
			return l.lexMethod
		}
		l.next()
	}
}

func (l *lexer) lexMethod() stateFn {
	l.scanForVariables()
	for {
		ch := l.peek()
		if ch == ' ' {
			l.emit(METHOD)
			break
		}

		l.next()
	}
	l.skip() // skip new line
	return l.lexBody
}

func (l *lexer) lexBody() stateFn {
	for {
		if strings.HasPrefix(l.input[l.pos:], blockEnd) {
			return l.lexBlockEnd
		}
		if strings.HasPrefix(l.input[l.pos:], expectaion) {
			return l.lexExpectation
		}
		ch := l.next()
		if ch == eof {
			l.emit(EOF)
			return nil
		}
	}

	l.emit(BODY)

	return l.lexBody
}
func (l *lexer) lexExpectation() stateFn {
	l.pos += len(expectaion)

	l.skipWhitespace()

	l.acceptToNewline()
	l.emit(EXPECTATION)
	l.skip() // skip new line

	return l.lexLine
}

func (l *lexer) lexBlockEnd() stateFn {
	l.pos += len(blockEnd)
	l.acceptToNewline()
	l.emit(BLOCK_END)
	l.skip() // skip new line
	return l.lexLine
}
