package rest

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"os"
	"time"

	"github.com/taybart/log"
	"github.com/taybart/rest/lexer"
)

const (
	isConcurrent = true
)

// Rest : client
type Rest struct {
	color  bool
	client *http.Client
	vars   map[string]string
	lexed  []lexer.MetaRequest
}

// New : create new client
func New() *Rest {
	return &Rest{
		color:  true,
		vars:   make(map[string]string),
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

// ReadIO : read ordered requests from io reader
func (r *Rest) ReadIO(buf io.Reader) error {
	scanner := bufio.NewScanner(buf)
	return r.read(scanner, !isConcurrent)
}

// Read : read ordered requests from file
func (r *Rest) Read(fn string) error {
	file, err := os.Open(fn)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	return r.read(scanner, !isConcurrent)
}

// ReadConcurrent : read unordered requests from file
func (r *Rest) ReadConcurrent(fn string) error {
	file, err := os.Open(fn)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	return r.read(scanner, isConcurrent)
}

func (r *Rest) read(scanner *bufio.Scanner, concurrent bool) error {
	lex := lexer.New(concurrent)
	reqs, vars, err := lex.Parse(scanner)
	if err != nil {
		return err
	}

	r.lexed = append(r.lexed, reqs...)
	for k, v := range vars {
		r.vars[k] = v
	}
	return nil
}

// Exec : do all loaded requests
func (r Rest) Exec() ([]string, error) {
	successful := []string{}
	for i := range r.lexed {
		res, err := r.ExecIndex(i)
		if err != nil {
			return successful, err
		}
		successful = append(successful, res)
	}
	return successful, nil
}

// ExecIndex : do specific block in requests
func (r Rest) ExecIndex(i int) (result string, err error) {
	if i > len(r.lexed)-1 {
		err = fmt.Errorf("Block %d does not exist", i)
		return
	}

	log.Debug("Building request block", i)
	req, err := lexer.BuildRequest(r.lexed[i], r.vars)
	if err != nil {
		return
	}
	if req.Skip {
		return
	}
	time.Sleep(req.Delay)
	log.Debugf("Sending request %d to %s\n", i, req.R.URL.String())
	resp, err := r.client.Do(req.R)
	if err != nil {
		return
	}

	log.Debug("Checking expectation")
	err = r.checkExpectation(req, resp)
	if err != nil {
		return
	}

	log.Debug("Take output into runtime vars")
	err = r.takeVariables(resp)
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
			req.R.URL.String(),
			log.Rtd,
			dump,
		)
	} else {
		result = fmt.Sprintf("%s\n%s\n---\n",
			req.R.URL.String(),
			dump,
		)
	}
	return
}

// checkExpectation : ensure request did what is was supposed to
func (r Rest) checkExpectation(req lexer.Request, res *http.Response) error {
	exp := req.Expectation
	if exp.Code == 0 {
		return nil
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("Issue reading body %w", err)
	}
	res.Body.Close()
	res.Body = ioutil.NopCloser(bytes.NewBuffer(body))

	if exp.Code != res.StatusCode {
		return fmt.Errorf("Incorrect status code returned %d != %d\nbody: %s", exp.Code, res.StatusCode, string(body))
	}

	if len(exp.Body) > 0 {
		if !bytes.Equal([]byte(exp.Body), body) {
			return fmt.Errorf("Body does not match expectation\nExpected:\n%s\nGot:\n%s\n", exp.Body, string(body))
		}
	}

	return nil
}

// takeVariables : get outputs if available
func (r *Rest) takeVariables(res *http.Response) (err error) {
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return
	}
	res.Body.Close()
	if len(body) == 0 {
		return
	}

	res.Body = ioutil.NopCloser(bytes.NewBuffer(body)) // put body back

	// TODO: add other return types
	var j map[string]interface{}

	// if the return is not json just ignore it
	if err = json.Unmarshal(body, &j); err != nil {
		err = nil
		return
	}

	for k, v := range r.vars {
		for jk, jv := range j {
			if v == jk { // if json key is a previous value
				r.vars[k] = jv.(string)
			}
		}
	}

	return
}

// IsRestFile : checks if file can be parsed
func (r Rest) IsRestFile(fn string) (bool, error) {
	log.Debugf("Checking if %s is a valid rest file\n", fn)
	file, err := os.Open(fn)
	if err != nil {
		return false, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lex := lexer.New(!isConcurrent)
	_, _, err = lex.Parse(scanner)
	if err != nil {
		return false, fmt.Errorf("Invalid format or malformed file: %w", err)
	}
	log.Debugf("Yay! %s is valid!\n", fn)
	return true, nil
}
