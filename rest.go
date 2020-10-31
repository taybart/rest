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
)

// Rest : client
type Rest struct {
	color    bool
	client   *http.Client
	lexed    requestBatch
	requests []request
}

// New : create new client
func New() *Rest {
	return &Rest{
		color:  true,
		client: http.DefaultClient,
		lexed: requestBatch{
			rtVars: make(map[string]restVar),
		},
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
	return r.read(scanner, false)
}

// Read : read ordered requests from file
func (r *Rest) Read(fn string) error {
	file, err := os.Open(fn)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	return r.read(scanner, false)
}

// ReadConcurrent : read unordered requests from file
func (r *Rest) ReadConcurrent(fn string) error {
	file, err := os.Open(fn)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	return r.read(scanner, true)
}

func (r *Rest) read(scanner *bufio.Scanner, concurrent bool) error {
	lex := newLexer(concurrent)
	reqs, err := lex.parse(scanner)
	if err != nil {
		return err
	}
	r.lexed.requests = append(r.lexed.requests, reqs.requests...)
	for k, v := range reqs.rtVars {
		r.lexed.rtVars[k] = v
	}
	return nil
}

// Exec : do all loaded requests
func (r Rest) Exec() (successful []string, err error) {
	for i, l := range r.lexed.requests {
		var req request
		req, err = buildRequest(l, r.lexed.rtVars)
		if err != nil {
			return
		}
		if req.skip {
			continue
		}

		time.Sleep(req.delay)
		log.Debugf("Sending request %d to %s\n", i, req.r.URL.String())
		var resp *http.Response
		resp, err = r.client.Do(req.r)
		if err != nil {
			return
			// failed = append(failed, err.Error())
			// continue
		}

		err = r.CheckExpectation(req, resp)
		if err != nil {
			// failed = append(failed, err.Error())
			// continue
			return
		}

		err = r.takeVariables(resp, &r.lexed.rtVars)
		if err != nil {
			return
		}

		var dump []byte
		dump, err = httputil.DumpResponse(resp, true)
		if err != nil {
			err = fmt.Errorf("%s\n%w", dump, err)
			return
			// log.Error(err)
			// failed = append(failed, err.Error())
			// continue
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
func (r Rest) ExecIndex(i int) (result string, err error) {
	if i > len(r.requests)-1 {
		err = fmt.Errorf("Block %d does not exist", i)
		return
	}

	req, err := buildRequest(r.lexed.requests[i], map[string]restVar{})
	if err != nil {
		return
	}
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

// CheckExpectation : ensure request did what is was supposed to
func (r Rest) CheckExpectation(req request, res *http.Response) error {
	exp := req.expectation
	if exp.code == 0 {
		return nil
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("Issue reading body %w", err)
	}
	res.Body.Close()
	res.Body = ioutil.NopCloser(bytes.NewBuffer(body))

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
func (r Rest) IsRestFile(fn string) (bool, error) {
	log.Debugf("Checking if %s is a valid rest file\n", fn)
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
	log.Debugf("Yay! %s is valid!\n", fn)
	return true, nil
}

func (rest Rest) takeVariables(res *http.Response, rtVars *map[string]restVar) (err error) {
	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return
	}
	if len(body) == 0 {
		return
	}
	var j map[string]string
	err = json.Unmarshal(body, &j)
	if err != nil {
		return
	}
	for k, v := range *rtVars {
		for jk, jv := range j {
			if v.value == jk {
				(*rtVars)[k] = restVar{
					name:  k,
					value: jv,
				}
			}
		}
	}

	return
}
