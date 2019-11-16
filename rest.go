package rest

import (
	"bufio"
	"net/http"
	"os"

	"github.com/taybart/log"
)

// Rest : client
type Rest struct {
	client   *http.Client
	requests []*http.Request
}

// New : create new client
func New() *Rest {
	return &Rest{
		client: http.DefaultClient,
	}
}

// Read : read ordered requests from file
func (r *Rest) Read(fn string) error {
	file, err := os.Open(fn)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lex := newLexer(
		false, // concurrent
	)
	r.requests, err = lex.parse(scanner)
	return err
}

// ReadConcurrent : read unordered requests from file
func (r *Rest) ReadConcurrent(fn string) error {
	file, err := os.Open(fn)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lex := newLexer(
		true, // concurrent
	)
	r.requests, err = lex.parse(scanner)
	return err
}

// Exec : do all loaded requests
func (r *Rest) Exec() {
	// TODO create error report
	for i, req := range r.requests {
		log.Infof("Sending request %d to %s\n", i, req.URL.String())
		r.client.Do(req)
	}
}
