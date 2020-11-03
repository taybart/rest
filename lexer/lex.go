package lexer

import (
	"bufio"
	"fmt"
	"regexp"
	"time"

	"github.com/taybart/log"
)

const (
	stateUrl = iota + 1
	stateHeaders
	stateMethodPath
	stateBody
)

var (
	rxLabel         = regexp.MustCompile(`^label (.*)`)
	rxSkip          = regexp.MustCompile(`^skip\s*$`)
	rxDelay         = regexp.MustCompile(`^delay (\d+(ns|us|µs|ms|s|m|h))$`)
	rxVarDefinition = regexp.MustCompile(`^set ([[:word:]\-]+) (.+)`)
	rxURL           = regexp.MustCompile(`^(https?)://[^\s/$.?#]*[^\s]*$`)
	rxHeader        = regexp.MustCompile(`[a-zA-Z-]+: .+`)
	rxMethod        = regexp.MustCompile(`^(OPTIONS|GET|POST|PUT|DELETE)`)
	rxPath          = regexp.MustCompile(`\/.*`)
	rxFile          = regexp.MustCompile(`^file://([/a-zA-Z0-9\-_\.]+)[\s+]?([a-zA-Z0-9]+)?$`)
	rxVar           = regexp.MustCompile(`\$\{([[:word:]\-]+)\}`)
	rxExpect        = regexp.MustCompile(`^expect (\d+) ?(.*)`)
	rxComment       = regexp.MustCompile(`^[[:space:]]*[#|\/\/]`)
	rxRuntimeVar    = regexp.MustCompile(`^take ([[:word:]]+) as ([[:word:]\-]+)`)
)

type Expectation struct {
	Code int
	Body string
}
type MetaRequest struct {
	Label       string
	Skip        bool
	URL         string
	Headers     map[string]string
	Method      string
	Path        string
	Body        string
	Filepath    string
	Filelabel   string
	Delay       time.Duration
	Expectation Expectation
	Reinterpret bool
	Block       []string
}

type Lexer struct {
	variables        map[string]string
	runtimeVariables map[string]bool
	concurrent       bool
	bch              chan MetaRequest
}

func New(concurrent bool) Lexer {
	return Lexer{
		variables:        make(map[string]string),
		runtimeVariables: make(map[string]bool),
		concurrent:       concurrent,
		bch:              make(chan MetaRequest),
	}
}

// parse : Parse a rest file and build golang http requests from it
func (l *Lexer) Parse(scanner *bufio.Scanner) (requests []MetaRequest, variables map[string]string, err error) {
	log.Debug("\nLex starting parse...")

	p, err := l.firstPass(scanner)
	if err != nil {
		return
	}

	var rs []MetaRequest
	if l.concurrent {
		rs, err = l.parseConcurrent(p)
	} else {
		rs, err = l.parseSerial(p)
	}
	return rs, l.variables, err
}

// parseBlocks : Parse blocks in the order in which they were given
func (l *Lexer) parseSerial(input []MetaRequest) (reqs []MetaRequest, err error) {
	log.Debug("Starting to parse blocks in order")
	for i, r := range input {
		lexed, e := l.parseBlock(r.Block)
		if e != nil {
			err = fmt.Errorf("block %d: %w", i, e)
			// log.Error(e)
			continue // TODO maybe should super fail
		}
		reqs = append(reqs, lexed)
	}
	log.Debugf("Parsed %d blocks\n", len(reqs))
	return
}

// parseBlocksConcurrently : Parse all blocks but don't care about order
func (l *Lexer) parseConcurrent(input []MetaRequest) (reqs []MetaRequest, err error) {
	log.Debug("Starting to parse blocks concurrently")
	for _, r := range input {
		go l.parseBlock(r.Block)
	}

	for i := 0; i < len(input); i++ {
		r := <-l.bch
		reqs = append(reqs, r)
	}
	log.Debug("Done")
	return
}
