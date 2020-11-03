package lexer

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/taybart/log"
)

// firstPass : look at blocks to get initial state
func (l *Lexer) firstPass(scanner *bufio.Scanner) (meta []MetaRequest, err error) {
	block := []string{}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "---" { // next block
			meta = append(meta, MetaRequest{
				Block: block,
			})
			block = []string{}
			continue
		}

		block = append(block, line)
		// check for interesting lines
		switch {
		case rxComment.MatchString(line):
			continue
		case rxSkip.MatchString(line):
			continue
		case rxRuntimeVar.MatchString(line):
			if l.concurrent {
				err = fmt.Errorf("found runtime variable but rest is set to run concurrently")
				return
			}
			v := rxRuntimeVar.FindStringSubmatch(line)
			log.Debugf("Found runtime variable %s with return value of %s\n", v[2], v[1])
			l.variables[v[2]] = v[1]
			l.runtimeVariables[v[2]] = true
		case rxVarDefinition.MatchString(line):
			v := rxVarDefinition.FindStringSubmatch(line)
			log.Debugf("Setting %s to %s\n", string(v[1]), string(v[2]))
			l.variables[v[1]] = v[2]
		}
	}
	meta = append(meta, MetaRequest{
		Block: block,
	})

	log.Debugf("Got %d blocks\n", len(meta))

	return
}

// parseBlock : Get all parts of request from request block
func (l *Lexer) parseBlock(block []string) (MetaRequest, error) {
	req := MetaRequest{
		Headers: make(map[string]string),
		Block:   block,
	}
	state := stateUrl
	for i, ln := range block {
		if rxComment.MatchString(ln) {
			log.Debug("Got comment", ln)
			continue
		}
		line, runtime, err := l.checkForUndeclaredVariables(ln)
		if err != nil {
			log.Fatal(err)
		}
		if runtime {
			req.Reinterpret = true
			continue
		}
		switch {
		case rxSkip.MatchString(line):
			req.Skip = true
		case rxRuntimeVar.MatchString(ln):
			continue
		case rxExpect.MatchString(line):
			m := rxExpect.FindStringSubmatch(line)
			if len(m) == 1 {
				log.Errorf("Malformed expectation in block %d [%s]\n", i, line)
				continue
			}
			req.Expectation.Code, err = strconv.Atoi(m[1])
			if err != nil {
				log.Errorf("Cannot parse expected code in block %d [%s]\n", i, line)
				continue
			}
			if len(m) == 3 {
				req.Expectation.Body = m[2]
			}
		case rxDelay.MatchString(line):
			m := rxDelay.FindStringSubmatch(line)
			req.Delay, err = time.ParseDuration(m[1])
			if err != nil {
				log.Errorf("Cannot parse delay in block %d [%s]\n", i, line)
				continue
			}
		case rxURL.MatchString(line):
			u := rxURL.FindString(line)
			log.Debug("Got URL", u)
			if isUrl(u) {
				req.URL = u
				// log.Debug("Got URL", u)
			}
			state = stateHeaders

		case rxMethod.MatchString(line):
			m := rxMethod.FindString(line)
			req.Method = m
			p := rxPath.FindString(line)
			req.Path = p
			log.Debug("Got method", m)
			log.Debug("Got path", p)
			state = stateBody

		case rxHeader.MatchString(line) && state == stateHeaders:
			sp := strings.Split(line, ":")
			key := strings.TrimSpace(sp[0])
			value := strings.TrimSpace(sp[1])
			req.Headers[key] = value
			log.Debugf("Set header %s to %s\n", key, value)

		case rxFile.MatchString(line):
			// fn := rxFile.FindString(line)
			matches := rxFile.FindStringSubmatch(line)
			if isValidFile(matches[1]) {
				req.Filepath = matches[1]
				req.Filelabel = matches[2]
				log.Debug("Got File", req.Filepath, req.Filelabel)
			}
			state = stateHeaders
		case rxLabel.MatchString(line):
			m := rxLabel.FindStringSubmatch(line)
			req.Label = m[1]

		case state == stateBody:
			req.Body += line
		}
	}

	if l.concurrent {
		l.bch <- req
	}
	return req, nil
}

func (l Lexer) checkForUndeclaredVariables(line string) (string, bool, error) {
	tmp := line
	reinterpret := false
	if rxVar.MatchString(line) {
		matches := rxVar.FindAllStringSubmatch(line, -1)
		for _, match := range matches {
			if _, ok := l.runtimeVariables[match[1]]; ok {
				reinterpret = true
				log.Debug(line, "-> NEED RUNTIME VALUE")
				continue
			}
			if v, ok := l.variables[match[1]]; ok {
				tmp = strings.ReplaceAll(tmp, match[0], v)
				continue
			}
			return "", false, fmt.Errorf("Saw variable %s%s%s and did not have a value for it",
				log.Blue, match[1], log.Rtd)
		}
		log.Debug(line, "->", tmp)
	}
	return tmp, reinterpret, nil
}
