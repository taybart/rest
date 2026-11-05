// Package rest is a simple REST client
package rest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/url"
	"os"
	"sort"
	"strings"

	"github.com/taybart/rest/client"
	"github.com/taybart/rest/exports"
	"github.com/taybart/rest/exports/templates"
	"github.com/taybart/rest/file"
	"github.com/taybart/rest/request"
	"github.com/taybart/rest/server"
)

type Rest struct {
	Parser   *file.Parser
	Requests map[string]*file.HCLRequest
	filename string
}

func NewFile(filename string) (*Rest, error) {
	parser, err := file.NewParser(filename)
	if err != nil {
		return nil, err
	}
	return newRest(filename, parser), nil
}

// NewSource builds a Rest from in-memory source instead of a file on disk,
// hcl diagnostics are written to diagOut
func NewSource(name string, src []byte, diagOut io.Writer) (*Rest, error) {
	parser, err := file.NewParserFromSource(name, src, diagOut)
	if err != nil {
		return nil, err
	}
	return newRest(name, parser), nil
}

func newRest(filename string, parser *file.Parser) *Rest {
	rest := &Rest{
		filename: filename,
		Parser:   parser,
		Requests: make(map[string]*file.HCLRequest),
	}
	for i, block := range parser.Root.Requests {
		rest.Requests[block.Label] = block
		rest.Requests[block.Label].BlockIndex = i
	}
	return rest
}

func (rest *Rest) RequestByIndex(i int) (request.Request, error) {
	var ret *file.HCLRequest
	for _, r := range rest.Requests {
		if r.BlockIndex == i {
			ret = r
			break
		}
	}
	if ret == nil {
		return request.Request{}, errors.New("request not found")
	}

	return rest.Parser.Request(ret)
}

func (rest *Rest) Request(label string) (request.Request, error) {
	req, ok := rest.Requests[label]
	if !ok {
		return request.Request{}, errors.New("request label not found")
	}
	return rest.Parser.Request(req)
}

func (rest *Rest) RunFile(ignoreFail bool) error {
	return rest.RunFileTo(os.Stdout, ignoreFail)
}

// RunFileTo runs every request in the file, writing results and after hook
// output to w
func (rest *Rest) RunFileTo(w io.Writer, ignoreFail bool) error {

	// make sure to run blocks in order of appearance
	order := make([]string, 0, len(rest.Requests))
	for k := range rest.Requests {
		order = append(order, k)
	}

	// Sort keys by BlockIndex
	sort.Slice(order, func(i, j int) bool {
		return rest.Requests[order[i]].BlockIndex < rest.Requests[order[j]].BlockIndex
	})

	client, err := client.New(rest.Parser.Config)
	if err != nil {
		return err
	}
	client.Out = w
	for _, label := range order {
		req, err := rest.Request(label)
		if err != nil {
			return err
		}

		if req.Skip {
			// TODO: what to do for usability, should probably warn user
			// log.Warn("skipping", req.Label)
			continue
		}

		res, exports, err := client.Do(req)
		if err != nil {
			if !ignoreFail {
				return err
			}
			fmt.Fprintln(w, err)
		}
		rest.Parser.AddExportsCtx(exports)

		if res != "" {
			fmt.Fprintln(w, res)
		}
	}
	return nil
}

func (rest *Rest) RunLabel(label string) error {
	return rest.RunLabelTo(os.Stdout, label)
}

// RunLabelTo runs one request, writing its result and after hook output to w
func (rest *Rest) RunLabelTo(w io.Writer, label string) error {
	req, err := rest.Request(label)
	if err != nil {
		return err
	}
	return rest.run(w, req)
}

func (rest *Rest) RunIndex(block int) error {
	req, err := rest.RequestByIndex(block)
	if err != nil {
		return err
	}
	return rest.run(os.Stdout, req)
}

func (rest *Rest) run(w io.Writer, req request.Request) error {
	client, err := client.New(rest.Parser.Config)
	if err != nil {
		return err
	}
	client.Out = w

	if req.Skip {
		return errors.New("request marked as skip = true")
	}

	res, _, err := client.Do(req)
	if res != "" {
		fmt.Fprintln(w, res)
	}
	return err
}

func (rest *Rest) RunSocket(socketArg string) error {
	socket, err := rest.Parser.Socket()
	if err != nil {
		return err
	}
	if len(socket.Playbook) == 0 {
		return fmt.Errorf("no socket in file")
	}
	client, err := client.New(rest.Parser.Config)
	if err != nil {
		return err
	}
	if err := client.DoSocket(socketArg, socket); err != nil {
		return err
	}
	return nil
}

func (rest *Rest) RunServer() error {
	config, err := rest.Parser.Server()
	if err != nil {
		return err
	}
	if config.Addr == "" {
		return errors.New("missing required server block")
	}

	s := server.New(config)
	return s.Serve()
}

func (rest *Rest) Export(export, label string, block int) error {
	return rest.ExportTo(os.Stdout, export, label, block)
}

// ExportTo renders requests as snippets using a built-in template (curl, go,
// js), a template file path, or postman. With no label or block (-1) every
// request is exported in file order.
func (rest *Rest) ExportTo(w io.Writer, export, label string, block int) error {
	if export == "?" || export == "ls" || export == "list" {
		for _, e := range templates.Builtins() {
			fmt.Fprintln(w, e)
		}
		fmt.Fprintln(w, "postman")
		return nil
	}
	if export == "postman" {
		return exports.ToPostmanCollection(rest.filename, label, block)
	}

	t, err := templates.Get(export)
	if err != nil {
		return err
	}

	treqs := []templates.Request{}
	for _, hreq := range rest.orderedRequests() {
		if label != "" && label != hreq.Label {
			continue
		}
		if block >= 0 && block != hreq.BlockIndex {
			continue
		}
		req, err := rest.Parser.Request(hreq)
		if err != nil {
			return err
		}
		// same as running: skipped requests drop out of the whole file and
		// asking for one directly is an error
		if req.Skip {
			if label != "" || block >= 0 {
				return fmt.Errorf("%s: request marked as skip = true", req.Label)
			}
			continue
		}
		treq, err := toTemplateRequest(req, rest.Parser.Config.UserAgent)
		if err != nil {
			return fmt.Errorf("%s: %w", req.Label, err)
		}
		treqs = append(treqs, treq)
	}
	if len(treqs) == 0 {
		if label != "" {
			return fmt.Errorf("request label %q not found", label)
		}
		if block >= 0 {
			return fmt.Errorf("request block %d not found", block)
		}
		return errors.New("no requests to export (all marked skip = true?)")
	}
	return t.Execute(w, treqs)
}

func (rest *Rest) orderedRequests() []*file.HCLRequest {
	reqs := make([]*file.HCLRequest, 0, len(rest.Requests))
	for _, r := range rest.Requests {
		reqs = append(reqs, r)
	}
	sort.Slice(reqs, func(i, j int) bool { return reqs[i].BlockIndex < reqs[j].BlockIndex })
	return reqs
}

// toTemplateRequest splits a parsed request into the parts templates get,
// templates.Request.URL/Headers recombine them into what rest would send
func toTemplateRequest(req request.Request, userAgent string) (templates.Request, error) {
	u, err := url.Parse(req.URL)
	if err != nil {
		return templates.Request{}, err
	}
	// same merge as request.Build: inline params, query block wins
	query := u.Query()
	for k, v := range req.Query {
		query.Set(k, v)
	}
	u.RawQuery = ""
	u.ForceQuery = false

	auth := templates.Auth{}
	if req.BasicAuth != "" {
		user, pass, ok := strings.Cut(req.BasicAuth, ":")
		if !ok {
			return templates.Request{}, errors.New("malformed basic auth value should be -> user:password")
		}
		auth = templates.Auth{Type: "basic", User: user, Password: pass}
	}
	// rest sends two Authorization headers if both are set, snippets just use bearer
	if req.BearerToken != "" {
		auth = templates.Auth{Type: "bearer", Token: req.BearerToken}
	}
	if userAgent == request.DefaultConfig().UserAgent {
		userAgent = ""
	}

	body := req.Body
	if body == "null" {
		body = ""
	}
	if body != "" && strings.HasPrefix(strings.ToLower(req.Headers["Content-Type"]), "application/json") {
		var buf bytes.Buffer
		if json.Compact(&buf, []byte(body)) == nil {
			body = buf.String()
		}
	}

	expect := templates.Expect{Status: req.ExpectStatus}
	if req.Expect != nil {
		expect.Headers = req.Expect.Headers
		expect.Body = req.Expect.Body
		if req.Expect.Status != 0 {
			expect.Status = req.Expect.Status
		}
	}

	method := strings.ToUpper(req.Method)
	if method == "" {
		method = "GET"
	}
	headers := map[string]string{}
	maps.Copy(headers, req.Headers)
	return templates.Request{
		Label:          req.Label,
		Method:         method,
		BaseURL:        u.String(),
		Query:          query,
		RequestHeaders: headers,
		Auth:           auth,
		UserAgent:      userAgent,
		Cookies:        req.Cookies,
		Body:           body,
		Expect:         expect,
	}, nil
}
