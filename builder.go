package rest

import (
	"bytes"
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

type request struct {
	label       string
	skip        bool
	r           *http.Request
	delay       time.Duration
	expectation expectation
	outputs     map[string]string
}

// type builder struct{}

// buildRequest : generate http.Request from parsed input
func buildRequest(input metaRequest, variables map[string]restVar) (req request, err error) {
	if err = isValidMetaRequest(input); err != nil {
		return
	}

	if input.reinterpret {
		log.Debug("Re-interpreting request", variables)
		l := newLexer(false)
		l.variables = variables
		input, err = l.parseBlock(input.block)
		if err != nil {
			return
		}
	}

	var r *http.Request
	url := fmt.Sprintf("%s%s", input.url, input.path)
	if !input.skip { // don't validate if skipped
		if !isUrl(url) {
			err = fmt.Errorf("url invalid or missing")
			return
		}
		if input.method == "" {
			err = fmt.Errorf("missing method")
			return
		}

		var body io.Reader
		body, err = buildBody(&input)
		if err != nil {
			err = fmt.Errorf("creating body %w", err)
			return
		}
		r, err = http.NewRequest(input.method, url, body)
		if err != nil {
			err = fmt.Errorf("creating request %w", err)
			return
		}
		for header, value := range input.headers {
			r.Header.Set(header, value)
		}
	}
	req = request{
		label:       input.label,
		skip:        input.skip,
		delay:       input.delay,
		expectation: input.expectation,

		r: r,
	}

	if !req.skip {
		err = isValidRequest(req)
		if err != nil {
			err = fmt.Errorf("invalid request %w", err)
			return
		}
	}
	return
}

func buildFileBody(input *metaRequest) (body *bytes.Buffer, err error) {
	file, err := os.Open(input.filepath)
	if err != nil {
		return
	}
	defer file.Close()

	body = &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile(input.filelabel, filepath.Base(input.filepath))
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
	input.headers["Content-Type"] = writer.FormDataContentType()
	return body, err
}

func buildBody(input *metaRequest) (body io.Reader, err error) {
	if input.filepath != "" {
		return buildFileBody(input)
	}

	// unknown body
	if input.body != "" {
		body = strings.NewReader(input.body)
		return
	}
	return
}

// isValidMetaRequest : checks if request is complete
func isValidMetaRequest(req metaRequest) error {
	if req.url == "" {
		return fmt.Errorf("No URL found in request")
	}
	if req.method == "" {
		return fmt.Errorf("No method found in request")
	}
	if req.filepath != "" && req.headers["Content-Type"] == "" {
		return fmt.Errorf("Content-Type not set for request with file")
	}
	if req.filepath != "" && req.filelabel == "" {
		return fmt.Errorf("file %s not labeled in request (ex file://path label)", req.filepath)
	}
	return nil
}

// isValidRequest : checks if request is complete
func isValidRequest(req request) error {
	if req.r.URL.String() == "" {
		return fmt.Errorf("No URL found in request")
	}
	if req.r.Method == "" {
		return fmt.Errorf("No method found in request")
	}
	return nil
}

// isValidFile checks if file should be consumed
func isValidFile(s string) bool {
	if s == "" {
		return false
	}
	// check file path for ..
	// check if file exists and is not dir
	return true
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
