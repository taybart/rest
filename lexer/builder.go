package lexer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/taybart/log"
)

type Request struct {
	Label       string
	Skip        bool
	R           *http.Request
	Delay       time.Duration
	Expectation Expectation
}

// type builder struct{}

// BuildRequest : generate http.Request from parsed input
func BuildRequest(input MetaRequest, variables map[string]string) (req Request, err error) {
	if input.Reinterpret {
		log.Debug("Re-interpreting request", variables)
		l := New(false)
		l.variables = variables
		input = l.parseBlock(input.Block)
	}

	if err = isValidMetaRequest(input); err != nil {
		return
	}

	var r *http.Request
	url := fmt.Sprintf("%s%s", input.URL, input.Path)
	if !input.Skip { // don't validate if skipped
		if !isUrl(url) {
			err = fmt.Errorf("url invalid or missing")
			return
		}
		if input.Method == "" {
			err = fmt.Errorf("missing method")
			return
		}

		var body io.Reader
		body, err = buildBody(&input)
		if err != nil {
			err = fmt.Errorf("creating body %w", err)
			return
		}
		r, err = http.NewRequest(input.Method, url, body)
		if err != nil {
			err = fmt.Errorf("creating request %w", err)
			return
		}
		for header, value := range input.Headers {
			r.Header.Set(header, value)
		}
	}
	req = Request{
		Label:       input.Label,
		Skip:        input.Skip,
		Delay:       input.Delay,
		Expectation: input.Expectation,
		R:           r,
	}

	if !req.Skip {
		err = isValidRequest(req)
		if err != nil {
			err = fmt.Errorf("invalid request %w", err)
			return
		}
	}
	return
}

func buildFileBody(input *MetaRequest) (body *bytes.Buffer, err error) {
	file, err := os.Open(input.Filepath)
	if err != nil {
		return
	}
	defer file.Close()

	body = &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile(input.Filelabel, filepath.Base(input.Filepath))
	if err != nil {
		return
	}
	_, err = io.Copy(part, file)

	/* for key, val := range params {
		_ = writer.WriteField(key, val)
	} */
	err = writer.Close()
	if err != nil {
		return
	}
	input.Headers["Content-Type"] = writer.FormDataContentType()
	return body, err
}

func buildBody(input *MetaRequest) (body io.Reader, err error) {
	if input.Filepath != "" {
		return buildFileBody(input)
	}

	if input.Body != "" {
		switch input.Headers["Content-Type"] {
		case "application/json":
			var b bytes.Buffer
			err = json.Compact(&b, []byte(input.Body))
			if err != nil {
				err = fmt.Errorf("json in body is malformed: [%w]", err)
				return
			}
			body = bytes.NewReader(b.Bytes())
			// fmt.Println(b.String())
		default: // unknown body
			body = strings.NewReader(input.Body)
		}
	}
	return
}

// isValidMetaRequest : checks if request is complete
func isValidMetaRequest(req MetaRequest) error {
	if req.URL == "" {
		return fmt.Errorf("No URL found in request")
	}
	if req.Method == "" {
		return fmt.Errorf("No method found in request")
	}
	if req.Filepath != "" && req.Headers["Content-Type"] == "" {
		return fmt.Errorf("Content-Type not set for request with file")
	}
	if req.Filepath != "" && req.Filelabel == "" {
		return fmt.Errorf("file %s not labeled in request (ex file://path label)", req.Filepath)
	}
	return nil
}

// isValidRequest : checks if request is complete
func isValidRequest(req Request) error {
	if req.R.URL.String() == "" {
		return fmt.Errorf("No URL found in request")
	}
	if req.R.Method == "" {
		return fmt.Errorf("No method found in request")
	}
	return nil
}

// isValidFile checks if file should be consumed
func isValidFile(fn string) bool {
	if fn == "" {
		return false
	}
	info, err := os.Stat(fn)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

// isUrl tests a string to determine if it is a well-structured url or not.
func isUrl(s string) bool {
	if s == "" {
		return false
	}
	// checks needed as of Go 1.6 because of change:
	// https://github.com/golang/go/commit/617c93ce740c3c3cc28cdd1a0d712be183d0b328#diff-6c2d018290e298803c0c9419d8739885L195
	// emulate browser and strip the '#' suffix prior to validation. see issue-#237
	if i := strings.Index(s, "#"); i > -1 {
		s = s[:i]
	}

	if len(s) == 0 {
		return false
	}

	url, err := url.ParseRequestURI(s)
	if err != nil || url.Scheme == "" || url.Host == "" {
		return false
	}
	return true
}
