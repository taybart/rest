package rest

import (
	"bufio"
	"fmt"
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

type restVar struct {
	name    string
	value   string
	runtime bool
}

type expectation struct {
	code int
	body string
}
type metaRequest struct {
	label           string
	skip            bool
	url             string
	headers         map[string]string
	method          string
	path            string
	body            string
	filepath        string
	filelabel       string
	delay           time.Duration
	expectation     expectation
	reinterpret     bool
	reinterpretVars []restVar
	block           []string
}
type requestBatch struct {
	requests []metaRequest
	rtVars   map[string]restVar
}

type lexer struct {
	variables  map[string]restVar
	concurrent bool
	bch        chan metaRequest
}

func newLexer(concurrent bool) lexer {
	return lexer{
		variables:  make(map[string]restVar),
		concurrent: concurrent,
		bch:        make(chan metaRequest),
	}
}

// parse : Parse a rest file and build golang http requests from it
func (l *lexer) parse(scanner *bufio.Scanner) (requests requestBatch, err error) {
	log.Debug("\nLex starting parse...")
	blocks := [][]string{}
	block := []string{}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "---" { // next block
			blocks = append(blocks, block)
			block = []string{}
			continue
		}
		block = append(block, line)

	}
	blocks = append(blocks, block)

	log.Debugf("Got %d blocks\n", len(blocks))
	p, err := l.firstPass(blocks)
	if err != nil {
		return
	}

	var rs []metaRequest
	if l.concurrent {
		rs, err = l.parseConcurrent(p)
	} else {
		rs, err = l.parseSerial(p)
	}
	if err != nil {
		return
	}
	rtVars := make(map[string]restVar)
	for k, v := range l.variables {
		if v.runtime {
			log.Debugf("var: %s is runtime\n", k)
		}
		rtVars[k] = v
	}
	l.purgeVars()
	return requestBatch{
		requests: rs,
		rtVars:   rtVars,
	}, nil
}

func (l *lexer) firstPass(blocks [][]string) (meta []metaRequest, err error) {
	for i, b := range blocks {
		for _, ln := range b {
			switch {
			case rxSkip.MatchString(ln):
				continue
			case rxRuntimeVar.MatchString(ln):
				if l.concurrent {
					err = fmt.Errorf("found runtime variable but rest is set to run concurrently")
					return
				}
				v := rxRuntimeVar.FindStringSubmatch(ln)
				log.Debugf("Found runtime variable %s with return value of %s\n", v[2], v[1])
				l.variables[v[2]] = restVar{
					name:    v[2],
					value:   v[1],
					runtime: true,
				}
			}
		}
		log.Debug("First pass on block", i)
		meta = append(meta, metaRequest{
			block: b,
		})
	}
	return
}

// parseBlocks : Parse blocks in the order in which they were given
func (l *lexer) parseSerial(input []metaRequest) (reqs []metaRequest, err error) {
	log.Debug("Starting to parse blocks in order")
	for i, r := range input {
		lexed, e := l.parseBlock(r.block)
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
func (l *lexer) parseConcurrent(input []metaRequest) (reqs []metaRequest, err error) {
	log.Debug("Starting to parse blocks concurrently")
	for _, r := range input {
		go l.parseBlock(r.block)
	}

	for i := 0; i < len(input); i++ {
		r := <-l.bch
		reqs = append(reqs, r)
	}
	log.Debug("Done")
	return
}

// parseBlock : Get all parts of request from request block
func (l *lexer) parseBlock(block []string) (metaRequest, error) {
	req := metaRequest{
		headers: make(map[string]string),
	}
	state := stateUrl
	for i, ln := range block {
		if rxComment.MatchString(ln) {
			log.Debug("Get comment", ln)
			continue
		}
		line, runtime, err := l.checkForUndeclaredVariables(ln)
		if err != nil {
			log.Fatal(err)
		}
		if runtime {
			req.block = block
			req.reinterpret = true
			continue
		}
		switch {
		case rxSkip.MatchString(line):
			req.skip = true
		case rxRuntimeVar.MatchString(ln):
			continue
		case rxExpect.MatchString(line):
			m := rxExpect.FindStringSubmatch(line)
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
		case rxDelay.MatchString(line):
			m := rxDelay.FindStringSubmatch(line)
			req.delay, err = time.ParseDuration(m[1])
			if err != nil {
				log.Errorf("Cannot parse delay in block %d [%s]\n", i, line)
				continue
			}
		case rxVarDefinition.MatchString(line):
			v := rxVarDefinition.FindStringSubmatch(line)
			log.Debugf("Setting %s to %s\n", string(v[1]), string(v[2]))
			l.variables[v[1]] = restVar{
				name:  v[1],
				value: v[2],
			}
		case rxURL.MatchString(line):
			u := rxURL.FindString(line)
			if isUrl(u) {
				req.url = u
				log.Debug("Got URL", u)
			}
			state = stateHeaders

		case rxMethod.MatchString(line):
			m := rxMethod.FindString(line)
			req.method = m
			p := rxPath.FindString(line)
			req.path = p
			log.Debug("Got method", m)
			log.Debug("Got path", p)
			state = stateBody

		case rxHeader.MatchString(line) && state == stateHeaders:
			sp := strings.Split(line, ":")
			key := strings.TrimSpace(sp[0])
			value := strings.TrimSpace(sp[1])
			req.headers[key] = value
			log.Debugf("Set header %s to %s\n", key, value)

		case rxFile.MatchString(line):
			// fn := rxFile.FindString(line)
			matches := rxFile.FindStringSubmatch(line)
			if isValidFile(matches[1]) {
				req.filepath = matches[1]
				req.filelabel = matches[2]
				log.Debug("Got File", req.filepath, req.filelabel)
			}
			state = stateHeaders
		case rxLabel.MatchString(line):
			m := rxLabel.FindStringSubmatch(line)
			req.label = m[1]

		case state == stateBody:
			req.body += line
		}
	}
	log.Debug("Building request")

	if l.concurrent {
		l.bch <- req
	}
	return req, nil
}

func (l lexer) checkForUndeclaredVariables(line string) (string, bool, error) {
	tmp := line
	reinterpret := false
	if rxVar.MatchString(line) {
		matches := rxVar.FindAllStringSubmatch(line, -1)
		for _, match := range matches {
			if l.variables[match[1]].runtime {
				tmp = l.variables[match[1]].value
				reinterpret = true
				log.Debug(line, "-> NEED RUNTIME VALUE")
				continue
			}
			if v, ok := l.variables[match[1]]; ok {
				tmp = strings.ReplaceAll(tmp, match[0], v.value)
				return tmp, false, nil
			}
			log.Debug(line, "->", tmp)
			return "", false, fmt.Errorf("Saw variable %s%s%s and did not have a value for it",
				log.Blue, match[1], log.Rtd)
		}
	}
	return tmp, reinterpret, nil
}

func (l *lexer) purgeVars() {
	l.variables = make(map[string]restVar)
}
