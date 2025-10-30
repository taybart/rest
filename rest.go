// Package rest is a simple REST client
package rest

import (
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/taybart/rest/exports"
	"github.com/taybart/rest/exports/templates"
	"github.com/taybart/rest/request"
	"github.com/taybart/rest/server"
)

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
	rest, err := NewRestFile(filename)
	if err != nil {
		return err
	}
	socket, err := rest.Parser.Socket()
	if err != nil {
		return err
	}
	if len(socket.Playbook) == 0 {
		return fmt.Errorf("no socket in file")
	}
	client, err := request.NewClient(rest.Config)
	if err != nil {
		return err
	}
	if err := client.DoSocket(socketArg, socket); err != nil {
		return err
	}
	return nil
}

func RunServerFile(filename string) error {

	rest, err := NewRestFile(filename)
	if err != nil {
		return err
	}
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

	rest, err := NewRestFile(filename)
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
}
