package rest

import (
	"bufio"
	"net/http"
	"net/http/httputil"
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

// SetClient : change default execution client
func (r *Rest) SetClient(c *http.Client) {
	r.client = c
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
	reqs, err := lex.parse(scanner)
	if err != nil {
		return err
	}
	r.requests = append(r.requests, reqs...)
	return nil
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
	reqs, err := lex.parse(scanner)
	if err != nil {
		return err
	}
	r.requests = append(r.requests, reqs...)
	return nil
}

// Exec : do all loaded requests
func (r *Rest) Exec() []string {
	// TODO create error report
	responses := []string{}
	for i, req := range r.requests {
		log.Debugf("Sending request %d to %s\n", i, req.URL.String())
		resp, err := r.client.Do(req)
		if err != nil {
			log.Error(err)
			continue
		}

		dump, err := httputil.DumpResponse(resp, true)
		if err != nil {
			log.Fatal(err)
		}
		responses = append(responses, string(dump))

	}
	r.requests = []*http.Request{} // clear requests
	return responses
}
