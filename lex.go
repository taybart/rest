package rest

import (
	"bufio"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/taybart/log"
)

const (
	stateUrl = iota + 1
	stateHeaders
	stateMethodPath
	stateBody
)

type expectation struct {
	code int
	body string
}

type metaRequest struct {
	label       string
	url         string
	headers     map[string]string
	method      string
	path        string
	body        string
	delay       time.Duration
	expectation expectation
}

type request struct {
	label       string
	r           *http.Request
	delay       time.Duration
	expectation expectation
}

type lexer struct {
	rxURL           *regexp.Regexp
	rxHeader        *regexp.Regexp
	rxPath          *regexp.Regexp
	rxMethod        *regexp.Regexp
	rxComment       *regexp.Regexp
	rxVarDefinition *regexp.Regexp
	rxVar           *regexp.Regexp
	rxDelay         *regexp.Regexp
	rxExpect        *regexp.Regexp
	rxLabel         *regexp.Regexp

	variables  map[string]string
	concurrent bool
	bch        chan request
	ech        chan error // unused
}

func newLexer(concurrent bool) lexer {
	return lexer{
		rxURL:           regexp.MustCompile(`^(https?)://[^\s/$.?#].[^\s]*`),
		rxHeader:        regexp.MustCompile(`[a-zA-Z-]+: .+`),
		rxMethod:        regexp.MustCompile(`^(OPTIONS|GET|POST|PUT|DELETE)`),
		rxPath:          regexp.MustCompile(`\/.*`),
		rxComment:       regexp.MustCompile(`^[[:space:]]*[#|\/\/]`),
		rxVarDefinition: regexp.MustCompile(`^set ([[:word:]\-]+) (.+)`),
		rxVar:           regexp.MustCompile(`\$\{([[:word:]\-]+)\}`),
		rxDelay:         regexp.MustCompile(`^delay (\d+(ns|us|µs|ms|s|m|h))$`),
		rxExpect:        regexp.MustCompile(`^expect (\d+) ?(.*)`),
		rxLabel:         regexp.MustCompile(`^label (.*)`),

		variables:  make(map[string]string),
		concurrent: concurrent,
		bch:        make(chan request),
	}
}

// parse : Parse a rest file and build golang http requests from it
func (l *lexer) parse(scanner *bufio.Scanner) ([]request, error) {
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
func (l *lexer) parseBlocks(blocks [][]string) (reqs []request, err error) {
	log.Debug("Starting to parse blocks in order")
	for i, block := range blocks {
		r, e := l.parseBlock(block)
		if e != nil {
			err = fmt.Errorf("block %d: %w", i, e)
			// log.Error(e)
			continue // TODO maybe should super fail
		}
		reqs = append(reqs, r)
	}
	log.Debugf("Parsed %d blocks\n", len(reqs))
	l.variables = make(map[string]string) // purge vars
	return
}

// parseBlocksConcurrently : Parse all blocks but don't care about order
func (l *lexer) parseBlocksConcurrently(blocks [][]string) (reqs []request, err error) {
	log.Debug("Starting to parse blocks concurrently")
	for _, block := range blocks {
		go l.parseBlock(block)
	}

	for i := 0; i < len(blocks); i++ {
		r := <-l.bch
		reqs = append(reqs, r)
	}
	log.Debug("Done")
	l.variables = make(map[string]string) // purge vars
	return
}

// parseBlock : Get all parts of request from request block
func (l *lexer) parseBlock(block []string) (request, error) {
	req := metaRequest{
		headers: make(map[string]string),
	}
	state := stateUrl
	for i, ln := range block {
		if l.rxComment.MatchString(ln) {
			log.Debug("Get comment", ln)
			continue
		}
		line, err := l.checkForVariables(ln)
		if err != nil {
			log.Fatal(err)
		}
		switch {
		case l.rxExpect.MatchString(line):
			m := l.rxExpect.FindStringSubmatch(line)
			if len(m) == 1 {
				log.Errorf("Malformed expectation in block %d [%s]\n", i, line)
				continue
			}
			req.expectation.code, err = strconv.Atoi(m[1])
			if err != nil {
				log.Errorf("Cannot parse expected code in block %d [%s]\n", i, line)
				continue
			}
			if len(m) == 3 {
				req.expectation.body = m[2]
			}
		case l.rxDelay.MatchString(line):
			m := l.rxDelay.FindStringSubmatch(line)
			req.delay, err = time.ParseDuration(m[1])
			if err != nil {
				log.Errorf("Cannot parse delay in block %d [%s]\n", i, line)
				continue
			}
		case l.rxVarDefinition.MatchString(line):
			v := l.rxVarDefinition.FindStringSubmatch(line)
			log.Debugf("Setting %s to %s\n", string(v[1]), string(v[2]))
			l.variables[v[1]] = v[2]
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

		case l.rxLabel.MatchString(line):
			m := l.rxLabel.FindStringSubmatch(line)
			req.label = m[1]

		case state == stateBody:
			req.body += line
		}
	}
	log.Debug("Building request")
	r, err := l.buildRequest(req)
	if err != nil {
		return request{}, err
	}

	if l.concurrent {
		l.bch <- r
	}
	return r, nil
}

func (l lexer) checkForVariables(line string) (string, error) {
	tmp := line
	if l.rxVar.MatchString(line) {
		matches := l.rxVar.FindAllStringSubmatch(line, -1)
		for _, match := range matches {
			if value, ok := l.variables[match[1]]; ok {
				tmp = strings.ReplaceAll(tmp, match[0], value)
			} else {
				return "", fmt.Errorf("Saw variable %s%s%s and did not have a value for it",
					log.Blue, match[1], log.Rtd)
			}
			log.Debug(line, "->", tmp)
		}
	}
	return tmp, nil
}

// buildRequest : generate http.Request from parsed input
func (l lexer) buildRequest(input metaRequest) (req request, err error) {
	url := fmt.Sprintf("%s%s", input.url, input.path)
	if !isUrl(url) {
		err = fmt.Errorf("url invalid or missing")
		return
	}
	if input.method == "" {
		err = fmt.Errorf("missing method")
		return
	}
	r, err := http.NewRequest(input.method, url, strings.NewReader(input.body))
	if err != nil {
		err = fmt.Errorf("creating request %w", err)
		return
	}
	for header, value := range input.headers {
		r.Header.Set(header, value)
	}
	req.r = r
	req.delay = 0
	if input.delay > 0 {
		req.delay = input.delay
	}
	req.expectation = input.expectation
	req.label = input.label

	err = l.validateRequest(req)
	if err != nil {
		err = fmt.Errorf("Invalid request %w", err)
		return
	}
	return
}

// validateRequest : checks if request is complete
func (l lexer) validateRequest(req request) error {
	if req.r.URL.String() == "" {
		return fmt.Errorf("No URL found in request")
	}
	if req.r.Method == "" {
		return fmt.Errorf("No method found in request")
	}
	return nil
}

// isUrl tests a string to determine if it is a well-structured url or not.
func isUrl(toTest string) bool {
	_, err := url.ParseRequestURI(toTest)
	return err == nil
}
