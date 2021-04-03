package parser

import (
	"bytes"
	"fmt"
	"html/template"
	"io/ioutil"
	"net/url"
	"strconv"

	"github.com/taybart/log"
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
	URI           *url.URL
	Headers       map[string]string
	Method        string
	Path          string
	Body          Body
	Expectaion    Expectation
}

func (b Block) String() string {
	// vars := ""
	// if len(b.Variables) > 0 {
	// 	vars = fmt.Sprintf("\n%s-- Variables set in Block--%s\n", log.Red, log.Rtd)
	// 	for k, v := range b.Variables {
	// 		vars += fmt.Sprintf("  %s: %s\n", k, v)
	// 	}
	// }

	tb := `{{green "~~~~~START~~~~~" }}
{{- if .Label }}
label: {{.Label}}
{{- end}}
uri: {{ .URI.String }}
{{- if .Headers }}
headers: {{ range $key, $value := .Headers }}
  {{ yellow $key }}: {{ yellow $value }}
{{- end}}{{ end}}
method: {{.Method}} {{.Path}}
{{ green "~~~~~~END~~~~~~" }}`

	t := template.Must(template.New("block").Funcs(log.TmplFuncs).Parse(tb))

	var buf bytes.Buffer
	t.Execute(&buf, b)
	return buf.String()
}

type Parser struct {
	variables        map[string]string
	runtimeVariables map[string]bool
	blocks           []Block
	blockIndex       int
	lexer            lexer
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

func (p *Parser) Run() error {
	go p.lexer.Run()

	for i := range p.lexer.items {
		block := &p.blocks[p.blockIndex]
		switch i.token {
		case VAR_REQUEST:
			if block.Variables != nil {
				if v, ok := block.Variables[i.value]; ok {
					p.lexer.cmd <- v
					break
				}
			}
			if v, ok := p.variables[i.value]; ok {
				p.lexer.cmd <- v
				break
			}
			return fmt.Errorf("Found variable with no definition %s", i.value)
		case COMMENT:
			log.Verbosef("%s# %s%s\n", log.Gray, i.value, log.Rtd)
		case LABEL:
			if block.Label != "" {
				return fmt.Errorf("Block already labled")
			}
			label := <-p.lexer.items
			if !isIdent(label) {
				return fmt.Errorf("Expected variable assignment")
			}
			block.Label = label.value
			log.Verbose("label:", label.value)

		case VARIABLE:
			ident := <-p.lexer.items
			if !isIdent(ident) {
				return fmt.Errorf("Expected variable identifier")
			}

			value := <-p.lexer.items
			if value.token != ASSIGN {
				return fmt.Errorf("Expected variable assignment")
			}
			p.variables[ident.value] = value.value // take latest global value

			if block.Variables == nil {
				block.Variables = make(map[string]string)
			}
			block.Variables[ident.value] = value.value

			log.Verbosef("%s%s=%s%s\n", log.Green, ident.value, value.value, log.Rtd)
		case HEADER:
			value := <-p.lexer.items
			if value.token != ASSIGN {
				return fmt.Errorf("Expected header value")
			}

			if block.Headers == nil {
				block.Headers = make(map[string]string)
			}
			block.Headers[i.value] = value.value
			log.Verbosef("%s%s: %s%s\n", log.Yellow, i.value, value.value, log.Rtd)
		case URL:
			log.Verbosef("%sURL -> %s%s\n", log.Blue, i.value, log.Rtd)
			u, err := url.Parse(i.value)
			if err != nil {
				panic(err)
			}
			block.URI = u
		case METHOD:
			// value := <-p.lexer.items
			// if value.token != ASSIGN {
			// 	return fmt.Errorf("Expected header value")
			// }

			if block.Headers == nil {
				block.Headers = make(map[string]string)
			}
			block.Method = i.value
			log.Verbosef("METHOD %s%s%s\n", log.Red, i.value, log.Rtd)
		case EXPECTATION:
			code, err := strconv.Atoi(i.value)
			if err != nil {
				return fmt.Errorf("could not convert expectaion")
			}
			block.Expectaion.Code = code
			log.Verbosef("%sexpect %s%s\n", log.Red, i.value, log.Rtd)
		case BLOCK_END:
			log.Verbosef("%s~~~~~~~END BLOCK %d~~~~~~~~%s\n", log.Blue, p.blockIndex, log.Rtd)
			p.blockIndex++
			p.blocks = append(p.blocks, Block{})
		case TEXT:
			fmt.Println("TEXT", i.value)
		case EOF:
			return p.parse()
		}
	}
	return nil
}

func (p *Parser) parse() error {
	for _, b := range p.blocks {
		fmt.Println(b.String())
	}
	return nil
}

func isIdent(i item) bool {
	return i.token == IDENT
}
