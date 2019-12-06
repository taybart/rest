package rest

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
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

		err = r.CheckExpectation(req, resp)
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
	if i > len(r.requests)-1 {
		err = fmt.Errorf("Block %d does not exist", i)
		return
	}
	req := r.requests[i]
	time.Sleep(req.delay)
	log.Debugf("Sending request %d to %s\n", i, req.r.URL.String())
	resp, err := r.client.Do(req.r)
	if err != nil {
		return
	}
	err = r.CheckExpectation(req, resp)
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

func (r *Rest) CheckExpectation(req request, res *http.Response) error {
	exp := req.expectation
	if exp.code == 0 {
		return nil
	}

	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("Issue reading body %w", err)
	}

	if exp.code != res.StatusCode {
		return fmt.Errorf("Incorrect status code returned %d != %d\nbody: %s", exp.code, res.StatusCode, string(body))
	}

	if len(exp.body) > 0 {
		if !bytes.Equal([]byte(exp.body), body) {
			return fmt.Errorf("Body does not match expectation\nExpected:\n%s\nGot:\n%s\n", exp.body, string(body))
		}
	}

	return nil
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
