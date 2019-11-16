package rest

import (
	"bufio"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/taybart/log"
)

const (
	stateUrl = iota + 1
	stateHeaders
	stateMethodPath
	stateBody
)

type request struct {
	url     string
	headers map[string]string
	method  string
	path    string
	body    string
}

type lexer struct {
	rxURL    *regexp.Regexp
	rxHeader *regexp.Regexp
	rxPath   *regexp.Regexp
	rxMethod *regexp.Regexp

	concurrent bool
	bch        chan *http.Request
	ech        chan error // unused
}

func newLexer(concurrent bool) lexer {
	return lexer{
		rxURL:    regexp.MustCompile(`(https?)://[^\s/$.?#].[^\s]*`),
		rxHeader: regexp.MustCompile(`[a-zA-Z-]*: .*`),
		rxMethod: regexp.MustCompile(`OPTIONS|GET|POST|PUT|DELETE`),
		rxPath:   regexp.MustCompile(`\/.*`),

		concurrent: concurrent,
		bch:        make(chan *http.Request),
	}
}

// parse : Parse a rest file and build golang http requests from it
func (l *lexer) parse(scanner *bufio.Scanner) ([]*http.Request, error) {
	// TODO build context blocks and run parse on each
	log.Debug("Lex starting parse")
	blocks := [][]string{}
	block := []string{}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "---" {
			blocks = append(blocks, block)
			block = []string{}
			continue
		}
		block = append(block, line)
	}
	blocks = append(blocks, block)

	log.Debugf("Got %d blocks\n", len(blocks))

	if l.concurrent {
		return l.parseBlocksConcurrently(blocks)
	}
	return l.parseBlocks(blocks)
}

// parseBlocks : Parse blocks in the order in which they were given
func (l *lexer) parseBlocks(blocks [][]string) (reqs []*http.Request, err error) {
	log.Debug("Starting to parse blocks in order")
	for _, block := range blocks {
		r, err := l.parseBlock(block)
		if err != nil {
			log.Error(err)
			continue // TODO maybe should super fail
		}
		reqs = append(reqs, r)
	}
	log.Debugf("Parsed %d blocks\n", len(reqs))
	return
}

// parseBlocksConcurrently : Parse all blocks but don't care about order
func (l *lexer) parseBlocksConcurrently(blocks [][]string) (reqs []*http.Request, err error) {
	log.Debug("Starting to parse blocks concurrently")
	for _, block := range blocks {
		go l.parseBlock(block)
	}

	for i := 0; i < len(blocks); i++ {
		r := <-l.bch
		reqs = append(reqs, r)
	}
	log.Debug("Done")
	return
}

// parseBlock : Get all parts of request from request block
func (l *lexer) parseBlock(block []string) (*http.Request, error) {
	req := request{
		headers: make(map[string]string),
	}
	state := stateUrl
	for _, line := range block {
		switch {
		case l.rxURL.MatchString(line):
			u := l.rxURL.FindString(line)
			if isUrl(u) {
				req.url = u
				log.Debug("Got URL", u)
			}
			state = stateHeaders

		case l.rxMethod.MatchString(line):
			m := l.rxMethod.FindString(line)
			req.method = m
			p := l.rxPath.FindString(line)
			req.path = p
			log.Debug("Got method", m)
			log.Debug("Got path", p)
			state = stateBody

		case l.rxHeader.MatchString(line) && state == stateHeaders:
			sp := strings.Split(line, ":")
			key := strings.TrimSpace(sp[0])
			value := strings.TrimSpace(sp[1])
			req.headers[key] = value
			log.Debugf("Set header %s to %s\n", key, value)

		case state == stateBody:
			req.body += line
		}
	}
	log.Debug("Building request")
	r, err := l.buildRequest(req)
	if err != nil {
		log.Error(err)
	}
	if l.concurrent {
		l.bch <- r
	}
	return r, err
}

// buildRequest : generate http.Request from parsed input
func (l lexer) buildRequest(input request) (req *http.Request, err error) {
	url := fmt.Sprintf("%s%s", input.url, input.path)
	if len(input.body) > 0 {
	}
	req, err = http.NewRequest(input.method, url, strings.NewReader(input.body))
	if err != nil {
		return
	}
	for header, value := range input.headers {
		req.Header.Set(header, value)
	}
	return
}

// isUrl tests a string to determine if it is a well-structured url or not.
func isUrl(toTest string) bool {
	_, err := url.ParseRequestURI(toTest)
	if err != nil {
		return false
	} else {
		return true
	}
}
