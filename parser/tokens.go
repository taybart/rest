package parser

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

	VAR_REQUEST
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
		"VAR_REQUEST",
	}[t]
}

const (
	eof            = rune(0)
	comment        = "#"
	label          = "label "
	variable       = "set "
	delay          = "delay "
	expectaion     = "expect "
	blockEnd       = "---"
	variablePrefix = "${"
	variableClose  = '}'
)
