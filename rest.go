package rest

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"os"
	"time"

	"github.com/taybart/log"
)

// Rest : client
type Rest struct {
	color    bool
	client   *http.Client
	requests []request
}

// New : create new client
func New() *Rest {
	return &Rest{
		color:  true,
		client: http.DefaultClient,
	}
}

// NoColor : change default execution client
func (r *Rest) NoColor() {
	r.color = false
}

// SetClient : change default execution client
func (r *Rest) SetClient(c *http.Client) {
	r.client = c
}

// ReadBuffer : read ordered requests from file
func (r *Rest) ReadIO(buf io.Reader) error {
	scanner := bufio.NewScanner(buf)
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
func (r *Rest) Exec() (successful, failed []string) {
	// TODO create error report
	for i, req := range r.requests {
		time.Sleep(req.delay)
		log.Debugf("Sending request %d to %s\n", i, req.r.URL.String())
		resp, err := r.client.Do(req.r)
		if err != nil {
			log.Error(err)
			failed = append(failed, err.Error())
			continue
		}

		dump, err := httputil.DumpResponse(resp, true)
		if err != nil {
			log.Error(err)
			failed = append(failed, err.Error())
			continue
		}

		if r.color {
			color := log.Green
			if resp.StatusCode >= 400 {
				color = log.Red
			}

			successful = append(successful, fmt.Sprintf("%s%s%s\n%s\n---\n",
				color,
				req.r.URL.String(),
				log.Rtd,
				dump,
			))
		} else {
			successful = append(successful, fmt.Sprintf("%s\n%s\n---\n",
				req.r.URL.String(),
				dump,
			))
		}
	}

	r.requests = []request{} // clear requests
	return
}

// ExecIndex : do specific block in requests
func (r *Rest) ExecIndex(i int) (result string, err error) {
	req := r.requests[i]
	time.Sleep(req.delay)
	log.Debugf("Sending request %d to %s\n", i, req.r.URL.String())
	resp, err := r.client.Do(req.r)
	if err != nil {
		return
	}

	dump, err := httputil.DumpResponse(resp, true)
	if err != nil {
		return
	}

	if r.color {
		color := log.Green
		if resp.StatusCode >= 400 {
			color = log.Red
		}
		result = fmt.Sprintf("%s%s%s\n%s\n---\n",
			color,
			req.r.URL.String(),
			log.Rtd,
			dump,
		)
	} else {
		result = fmt.Sprintf("%s\n%s\n---\n",
			req.r.URL.String(),
			dump,
		)
	}
	return
}

// IsRestFile : checks if file can be parsed
func (r *Rest) IsRestFile(fn string) (bool, error) {
	file, err := os.Open(fn)
	if err != nil {
		return false, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lex := newLexer(
		false, // concurrent
	)
	_, err = lex.parse(scanner)
	if err != nil {
		return false, fmt.Errorf("Invalid format or malformed file: %w", err)
	}
	return true, nil
}
