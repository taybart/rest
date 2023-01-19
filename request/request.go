package request

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"

	"github.com/taybart/log"
)

type Request struct {
	URL     string            `hcl:"url"`
	Method  string            `hcl:"method"`
	Port    int               `hcl:"port,optional"`
	Body    string            `hcl:"body,optional"`
	Headers []string          `hcl:"headers,optional"`
	Query   map[string]string `hcl:"query,optional"`
	// extras
	Label  string `hcl:"label,optional"`
	Delay  string `hcl:"delay,optional"`
	Expect int    `hcl:"expect,optional"`
}

func (r Request) String() string {
	headers := ""
	for _, h := range r.Headers {
		headers += fmt.Sprintf("%s\n", h)
	}
	body := ""
	if r.Body != "" {
		body = r.Body
	}
	return fmt.Sprintf("%s %s\n%s\n%s", r.Method, r.URL, headers, body)
}

func (r Request) Do() error {

	if r.Delay != "" {
		delay, err := time.ParseDuration(r.Delay)
		if err != nil {
			return err
		}
		time.Sleep(delay)
	}
	r.Format()

	// fmt.Println(r)
	req, err := http.NewRequest(r.Method, r.URL, strings.NewReader(r.Body))
	if err != nil {
		return err
	}
	for _, h := range r.Headers {
		hdrs := strings.Split(h, ":")
		req.Header.Add(hdrs[0], strings.TrimPrefix(h, hdrs[0]+":"))
	}

	req.Header.Set("User-Agent", "rest-client/2.0")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	if r.Expect != 0 {
		if res.StatusCode != r.Expect {
			return fmt.Errorf("unexpected response code %d != %d", r.Expect, res.StatusCode)
		}
	}
	dumped, err := httputil.DumpResponse(res, true)
	if err != nil {
		return err
	}
	fmt.Println(string(dumped))
	return nil
}

func (r *Request) Format() error {
	if r.Body != "" {
		r.Body = formatJSON(r.Body)
	}
	return nil
}

func formatJSON(toFmt string) string {
	var b bytes.Buffer
	err := json.Compact(&b, []byte(toFmt))
	if err != nil {
		log.Error(err)
		return toFmt
	}
	return b.String()
}
