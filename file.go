// Package rest is a simple REST client
package rest

import (
	"fmt"
	"os"

	"github.com/taybart/rest/file"
	"github.com/taybart/rest/request"
	"github.com/taybart/rest/request/templates"
)

func RunFile(filename string, ignoreFail bool) error {
	rest, err := file.Parse(filename)
	if err != nil {
		return err
	}
	client, err := request.NewClient(rest.Config)
	if err != nil {
		return err
	}
	for _, req := range rest.Requests {
		res, err := client.Do(req)
		if err != nil {
			if !ignoreFail {
				return err
			}
			fmt.Println(err)
		}
		if res != "" {
			fmt.Println(res)
		}
	}
	return nil
}

func RunLabel(filename string, label string) error {
	rest, err := file.Parse(filename)
	if err != nil {
		return err
	}
	client, err := request.NewClient(rest.Config)
	if err != nil {
		return err
	}
	req, ok := rest.Requests[label]
	if !ok {
		return fmt.Errorf("request label not found")
	}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	if res != "" {
		fmt.Println(res)
	}
	return nil
}

func RunBlock(filename string, block int) error {
	rest, err := file.Parse(filename)
	if err != nil {
		return err
	}
	client, err := request.NewClient(rest.Config)
	if err != nil {
		return err
	}

	var todo request.Request
	for _, req := range rest.Requests {
		if req.BlockIndex == block {
			todo = req
			break
		}
	}
	if todo.Label == "" {
		return fmt.Errorf("request block not found")
	}

	res, err := client.Do(todo)
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

func ExportFile(filename, export string, client bool) error {
	if export == "?" || export == "ls" || export == "list" {
		for _, e := range templates.Exports() {
			fmt.Println(e)
		}
		return nil
	}

	rest, err := file.Parse(filename)
	if err != nil {
		return err
	}
	t := templates.Get(export)
	if t == nil {
		return fmt.Errorf(" exporting language (%s) not supported", export)
	}
	treqs := []templates.Request{}
	for _, req := range rest.Requests {
		body := req.Body
		if body == "null" {
			body = ""
		}
		ua := rest.Config.UserAgent
		if ua == request.DefaultConfig().UserAgent {
			ua = ""
		}
		treqs = append(treqs, templates.Request{
			Method:   req.Method,
			URL:      req.URL,
			Headers:  req.Headers,
			Body:     body,
			Query:    req.Query,
			Cookies:  req.Cookies,
			PostHook: req.PostHook,
			Label:    req.Label,
			Delay:    req.Delay,
			Expect:   req.Expect,
			// config
			UserAgent: ua,
		})
	}
	if client {
		return t.ExecuteClient(os.Stdout, treqs)
	}
	for i, req := range treqs {
		err := t.Execute(os.Stdout, req)
		if err != nil {
			return err
		}
		fmt.Printf("\n")
		if i < len(rest.Requests)-1 {
			fmt.Printf("\n")
		}
	}

	return nil
}
