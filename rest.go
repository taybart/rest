// Package rest is a simple REST client
package rest

import (
	"errors"
	"fmt"
	"os"
	"sort"

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
	rest := &Rest{
		filename: filename,
		Parser:   parser,
		Requests: make(map[string]*file.HCLRequest),
	}
	for i, block := range parser.Root.Requests {
		rest.Requests[block.Label] = block
		rest.Requests[block.Label].BlockIndex = i
	}
	return rest, nil

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
	for _, label := range order {
		// TODO: make sure ctx works the right way here
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
			fmt.Println(err)
		}
		rest.Parser.AddExportsCtx(exports)

		if res != "" {
			fmt.Println(res)
		}
	}
	return nil
}

func (rest *Rest) RunLabel(label string) error {
	req, err := rest.Request(label)
	if err != nil {
		return err
	}
	return rest.run(req)
}

func (rest *Rest) RunIndex(block int) error {
	req, err := rest.RequestByIndex(block)
	if err != nil {
		return err
	}
	return rest.run(req)
}

func (rest *Rest) Run(req request.Request) (string, error) {
	client, err := client.New(rest.Parser.Config)
	if err != nil {
		return "", err
	}

	if req.Skip {
		return "", errors.New("request marked as skip = true")
	}

	res, _, err := client.Do(req)
	if err != nil {
		return "", err
	}
	return res, nil
}
func (rest *Rest) run(req request.Request) error {
	res, err := rest.Run(req)
	if res != "" {
		fmt.Println(res)
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
	if export == "?" || export == "ls" || export == "list" {
		for _, e := range templates.Exports() {
			fmt.Println(e)
		}
		fmt.Println("postman")
		return nil
	}
	if export == "postman" {
		return exports.ToPostmanCollection(rest.filename, label, block)
	}

	t := templates.Get(export)
	if t == nil {
		return fmt.Errorf(" exporting language (%s) not supported", export)
	}
	treqs := map[string]templates.Request{}
	for label := range rest.Requests {
		req, err := rest.Request(label)
		if err != nil {
			return err
		}

		body := req.Body
		if body == "null" {
			body = ""
		}
		ua := rest.Parser.Config.UserAgent
		if ua == request.DefaultConfig().UserAgent {
			ua = ""
		}
		expect := templates.Expect{}
		if req.Expect != nil {
			expect = templates.Expect{
				Status:  req.Expect.Status,
				Body:    req.Expect.Body,
				Headers: req.Expect.Headers,
			}
		}
		treqs[req.Label] = templates.Request{
			Method:  req.Method,
			URL:     req.URL,
			Headers: req.Headers,
			Body:    body,
			Query:   req.Query,
			Cookies: req.Cookies,
			After:   req.After,
			Label:   req.Label,
			Delay:   req.Delay,
			Expect:  expect,
			// BlockIndex: req.BlockIndex,
			// config
			UserAgent: ua,
		}
	}
	return t.Execute(os.Stdout, rest.filename, treqs)
}
