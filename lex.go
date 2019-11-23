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

// TODO isRestFile(), for command line

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
	rxURL           *regexp.Regexp
	rxHeader        *regexp.Regexp
	rxPath          *regexp.Regexp
	rxMethod        *regexp.Regexp
	rxComment       *regexp.Regexp
	rxVarDefinition *regexp.Regexp
	rxVar           *regexp.Regexp

	variables  map[string]string
	concurrent bool
	bch        chan *http.Request
	ech        chan error // unused
}

func newLexer(concurrent bool) lexer {
	return lexer{
		rxURL:           regexp.MustCompile(`(https?)://[^\s/$.?#].[^\s]*`),
		rxHeader:        regexp.MustCompile(`[a-zA-Z-]+: .+`),
		rxMethod:        regexp.MustCompile(`OPTIONS|GET|POST|PUT|DELETE`),
		rxPath:          regexp.MustCompile(`\/.*`),
		rxComment:       regexp.MustCompile(`^[[:space:]]*[#|\/\/]`),
		rxVarDefinition: regexp.MustCompile(`set ([[:word:]\-]+) (.+)`),
		rxVar:           regexp.MustCompile(`\$\{([[:word:]\-]+)\}`),

		variables:  make(map[string]string),
		concurrent: concurrent,
		bch:        make(chan *http.Request),
	}
}

// parse : Parse a rest file and build golang http requests from it
func (l *lexer) parse(scanner *bufio.Scanner) ([]*http.Request, error) {
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
	for i, block := range blocks {
		r, err := l.parseBlock(block)
		if err != nil {
			err = fmt.Errorf("Block %d %w", i, err)
			log.Error(err)
			continue // TODO maybe should super fail
		}
		reqs = append(reqs, r)
	}
	log.Debugf("Parsed %d blocks\n", len(reqs))
	l.variables = make(map[string]string) // purge vars
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
	l.variables = make(map[string]string) // purge vars
	return
}

// parseBlock : Get all parts of request from request block
func (l *lexer) parseBlock(block []string) (*http.Request, error) {
	req := request{
		headers: make(map[string]string),
	}
	state := stateUrl
	for _, ln := range block {
		if l.rxComment.MatchString(ln) {
			log.Debug("Get comment", ln)
			continue
		}
		line, err := l.checkForVariables(ln)
		if err != nil {
			log.Fatal(err)
		}
		switch {
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

		case state == stateBody:
			req.body += line
		}
	}
	log.Debug("Building request")
	r, err := l.buildRequest(req)
	if err == nil {
		if l.concurrent {
			l.bch <- r
		}
		return r, nil
	}
	return nil, err
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
func (l lexer) buildRequest(input request) (req *http.Request, err error) {
	url := fmt.Sprintf("%s%s", input.url, input.path)
	if !isUrl(url) {
		err = fmt.Errorf("url invalid or missing")
		return
	}
	req, err = http.NewRequest(input.method, url, strings.NewReader(input.body))
	if err != nil {
		err = fmt.Errorf("creating request %w", err)
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
