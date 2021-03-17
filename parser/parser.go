package parser

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"strconv"
)

type Body struct {
	Filepath  string
	Filelabel string
	Body      string
}

type Expectation struct {
	Code int
	Body []byte
}

type Block struct {
	SkipExecution bool
	Label         string
	Variables     map[string]string
	UndefVars     map[string]string
	URI           url.URL
	Headers       map[string]string
	Method        string
	Path          string
	Body          Body
	Expectaion    Expectation
}

type Parser struct {
	variables        map[string]string
	runtimeVariables map[string]bool
	blocks           []Block
	blockIndex       int
	lexer            Lexer
}

func New(fn string) Parser {
	b, err := ioutil.ReadFile(fn)
	if err != nil {
		panic(fmt.Errorf("could not open file: %w", err))
	}
	l := newLexer(string(b))
	return Parser{
		lexer:     l,
		blocks:    make([]Block, 1),
		variables: make(map[string]string),
	}

}

func (p *Parser) Parse() error {
	go p.lexer.Run()

	for i := range p.lexer.items {
		// if i.token != COMMENT {
		// 	fmt.Println(log.Green, i.token.String(), log.Rtd, i.value)
		// }

		switch i.token {
		case COMMENT:
			fmt.Println("#", i.value)
		case LABEL:
			if p.blocks[p.blockIndex].Label != "" {
				return fmt.Errorf("Block already labled")
			}
			label := <-p.lexer.items
			if !isIdent(label) {
				return fmt.Errorf("Expected variable assignment")
			}
			p.blocks[p.blockIndex].Label = label.value
			fmt.Println("Block label: ", p.blockIndex, label.value)

		case VARIABLE:
			ident := <-p.lexer.items
			if !isIdent(ident) {
				return fmt.Errorf("Expected variable identifier")
			}

			value := <-p.lexer.items
			if value.token != ASSIGN {
				return fmt.Errorf("Expected variable assignment")
			}
			p.variables[ident.value] = value.value

			if p.blocks[p.blockIndex].Variables == nil {
				p.blocks[p.blockIndex].Variables = make(map[string]string)
			}
			p.blocks[p.blockIndex].Variables[i.value] = value.value

			fmt.Println("VARIABLE", ident.value, "=", value.value)
		case HEADER:
			value := <-p.lexer.items
			if value.token != ASSIGN {
				return fmt.Errorf("Expected header value")
			}

			if p.blocks[p.blockIndex].Headers == nil {
				p.blocks[p.blockIndex].Headers = make(map[string]string)
			}
			p.blocks[p.blockIndex].Headers[i.value] = value.value
			fmt.Println("HEADER", i.value, ":", value.value)
		case EXPECTATION:
			code, err := strconv.Atoi(i.value)
			if err != nil {
				return fmt.Errorf("could not convert expectaion")
			}
			p.blocks[p.blockIndex].Expectaion.Code = code
			fmt.Println("Expect", i.value)
		case BLOCK_END:
			fmt.Println("~~ BLOCK END ~~")
			p.blockIndex++
			p.blocks = append(p.blocks, Block{})
		}
	}
	return nil
}

func isIdent(i item) bool {
	return i.token == IDENT
}
