// Package rest is a simple REST client
package rest

import (
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/taybart/rest/exports"
	"github.com/taybart/rest/exports/templates"
	"github.com/taybart/rest/file"
	"github.com/taybart/rest/request"
	"github.com/taybart/rest/server"
)

type RestFile struct {
	Config      request.Config
	Socket      request.Socket
	Server      server.Config
	HCLRequests map[string]*file.HCLRequest
	Requests    map[string]request.Request
	Parser      *file.Parser
}

func NewRestFile(filename string) (RestFile, error) {
	parser, err := file.NewParser(filename)
	if err != nil {
		return RestFile{}, err
	}
	rest := RestFile{Parser: parser, Config: parser.Config, HCLRequests: make(map[string]*file.HCLRequest)}
	for i, block := range parser.Root.Requests {
		rest.HCLRequests[block.Label] = block
		rest.HCLRequests[block.Label].BlockIndex = i
	}
	return rest, nil

}

func (rest *RestFile) RequestIndex(i int) (request.Request, error) {

	var todo *file.HCLRequest
	for _, req := range rest.HCLRequests {
		if req.BlockIndex == i {
			todo = req
			break
		}
	}
	if todo == nil {
		return request.Request{}, errors.New("request not found")
	}

	req, err := rest.Parser.Request(todo)
	if err != nil {
		return req, err
	}
	return req, nil
}

func (rest *RestFile) Request(label string) (request.Request, error) {
	hreq, ok := rest.HCLRequests[label]
	if !ok {
		return request.Request{}, errors.New("request label not found")
	}
	req, err := rest.Parser.Request(hreq)
	if err != nil {
		return req, err
	}
	return req, nil
}

func RunClientFile(filename string, ignoreFail bool) error {
	rest, err := NewRestFile(filename)
	if err != nil {
		return err
	}

	client, err := request.NewClient(rest.Config)
	if err != nil {
		return err
	}
	// make sure to run blocks in order of appearance
	order := make([]string, 0, len(rest.HCLRequests))
	for k := range rest.HCLRequests {
		order = append(order, k)
	}

	// Sort keys by BlockIndex
	sort.Slice(order, func(i, j int) bool {
		return rest.HCLRequests[order[i]].BlockIndex < rest.HCLRequests[order[j]].BlockIndex
	})

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

func RunClientLabel(filename string, label string) error {
	rest, err := NewRestFile(filename)
	if err != nil {
		return err
	}

	client, err := request.NewClient(rest.Config)
	if err != nil {
		return err
	}
	req, err := rest.Request(label)
	if err != nil {
		return err
	}
	if req.Skip {
		fmt.Println("skipping", req.Label)
		return nil
	}
	res, _, err := client.Do(req)
	if err != nil {
		return err
	}
	if res != "" {
		fmt.Println(res)
	}
	return nil
}

func RunClientBlock(filename string, block int) error {
	rest, err := NewRestFile(filename)
	if err != nil {
		return err
	}

	client, err := request.NewClient(rest.Config)
	if err != nil {
		return err
	}

	req, err := rest.RequestIndex(block)
	if err != nil {
		return err
	}

	if req.Skip {
		fmt.Println("skipping", req.Label)
		return nil
	}

	res, _, err := client.Do(req)
	if err != nil {
		return err
	}
	if res != "" {
		fmt.Println(res)
	}
	return nil
}

func RunSocket(socketArg string, filename string) error {
	rest, err := file.Parse(filename)
	if err != nil {
		return err
	}
	if len(rest.Socket.Playbook) == 0 {
		return fmt.Errorf("no socket in file")
	}
	client, err := request.NewClient(rest.Config)
	if err != nil {
		return err
	}
	if err := client.DoSocket(socketArg, rest.Socket); err != nil {
		return err
	}
	return nil
}

func ExportFile(filename, export, label string, block int) error {
	if export == "?" || export == "ls" || export == "list" {
		for _, e := range templates.Exports() {
			fmt.Println(e)
		}
		return nil
	}
	if export == "postman" {
		return exports.ToPostmanCollection(filename, label, block)
	}

	rest, err := file.Parse(filename)
	if err != nil {
		return err
	}
	t := templates.Get(export)
	if t == nil {
		return fmt.Errorf(" exporting language (%s) not supported", export)
	}
	treqs := map[string]templates.Request{}
	for _, req := range rest.Requests {
		body := req.Body
		if body == "null" {
			body = ""
		}
		ua := rest.Config.UserAgent
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
			Method:   req.Method,
			URL:      req.URL,
			Headers:  req.Headers,
			Body:     body,
			Query:    req.Query,
			Cookies:  req.Cookies,
			PostHook: req.PostHook,
			Label:    req.Label,
			Delay:    req.Delay,
			Expect:   expect,
			// BlockIndex: req.BlockIndex,
			// config
			UserAgent: ua,
		}
	}
	return t.Execute(os.Stdout, filename, treqs)
	// if label != "" {
	// 	req, ok := treqs[label]
	// 	if !ok {
	// 		return fmt.Errorf("request label not found")
	// 	}
	// 	return t.Execute(os.Stdout, req)
	// }
	// if block >= 0 {
	// 	for _, req := range treqs {
	// 		if req.BlockIndex == block {
	// 			return t.Execute(os.Stdout, req)
	// 		}
	// 	}
	// 	return errors.New("request block not found")
	// }
	// count := 0
	// for _, req := range treqs {
	// 	err := t.Execute(os.Stdout, req)
	// 	if err != nil {
	// 		return err
	// 	}
	// 	fmt.Printf("\n")
	// 	if count < len(rest.Requests)-1 {
	// 		fmt.Printf("\n")
	// 	}
	// 	count += 1
	// }
	// return nil
}

func RunServerFile(filename string) error {

	rest, err := NewRestFile(filename)
	if err != nil {
		return err
	}
	servConf, err := rest.Parser.ParseServer()
	if err != nil {
		return err
	}
	if servConf.Addr == "" {
		return errors.New("missing required server block")
	}

	s := server.New(servConf)
	return s.Serve()
}
