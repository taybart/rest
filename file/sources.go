package file

import (
	"path/filepath"
	"slices"
	"sync"
)

// sources are the files a parse depends on: the rest file, its imports, and
// what read() read. Evaluating read() later (ex. a request block's, per
// request) adds to them, so they're safe to add to concurrently.
type sources struct {
	mu    sync.Mutex
	paths map[string]bool
}

func (s *sources) add(path string) {
	if path == "" || path == "-" {
		return
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.paths == nil {
		s.paths = map[string]bool{}
	}
	s.paths[path] = true
}

// Sources are the (absolute) paths of the files the parse read: the rest
// file, its imports, and read()'s files (so far: read() in a block that's
// evaluated later adds when it is). A change to one of them needs a new parse.
func (p *Parser) Sources() []string {
	for name := range p.Files {
		p.sources.add(name)
	}
	p.sources.mu.Lock()
	defer p.sources.mu.Unlock()
	out := make([]string, 0, len(p.sources.paths))
	for path := range p.sources.paths {
		out = append(out, path)
	}
	slices.Sort(out)
	return out
}
