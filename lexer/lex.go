package lexer

import (
	"bufio"
	"time"

	"github.com/taybart/log"
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
		rs = l.parseConcurrent(p)
	} else {
		rs = l.parseSerial(p)
	}
	return rs, l.variables, nil
}

// parseBlocks : Parse blocks in the order in which they were given
func (l *Lexer) parseSerial(input []MetaRequest) []MetaRequest {
	log.Debug("Starting to parse blocks in order")
	reqs := []MetaRequest{}
	for _, r := range input {
		lexed := l.parseBlock(r.Block)
		reqs = append(reqs, lexed)
	}
	log.Debugf("Parsed %d blocks\n", len(reqs))
	return reqs
}

// parseBlocksConcurrently : Parse all blocks but don't care about order
func (l *Lexer) parseConcurrent(input []MetaRequest) []MetaRequest {
	log.Debug("Starting to parse blocks concurrently")
	for _, r := range input {
		go l.parseBlock(r.Block)
	}

	reqs := []MetaRequest{}
	for i := 0; i < len(input); i++ {
		r := <-l.bch
		reqs = append(reqs, r)
	}
	log.Debug("Done")
	return reqs
}
