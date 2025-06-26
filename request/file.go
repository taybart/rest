package request

import (
	"fmt"
	"os"

	"github.com/taybart/rest/request/templates"
)

func RunFile(filename string, ignoreFail bool) error {
	config, requests, _, err := parseFile(filename)
	if err != nil {
		return err
	}
	client, err := NewRequestClient(config)
	if err != nil {
		return err
	}
	for _, req := range requests {
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
	config, requests, _, err := parseFile(filename)
	if err != nil {
		return err
	}
	client, err := NewRequestClient(config)
	if err != nil {
		return err
	}
	for _, req := range requests {
		if req.Label == label {
			res, err := client.Do(req)
			if err != nil {
				return err
			}
			if res != "" {
				fmt.Println(res)
			}
			return nil
		}
	}

	return fmt.Errorf("request label not found")
}

func RunBlock(filename string, block int) error {
	config, requests, _, err := parseFile(filename)
	if err != nil {
		return err
	}
	client, err := NewRequestClient(config)
	if err != nil {
		return err
	}
	res, err := client.Do(requests[block])
	if err != nil {
		return err
	}
	if res != "" {
		fmt.Println(res)
	}
	return nil
}

func RunSocket(socketArg string, filename string) error {
	config, _, socket, err := parseFile(filename)
	if err != nil {
		return err
	}
	if socket == nil {
		return fmt.Errorf("no socket in file")
	}
	client, err := NewRequestClient(config)
	if err != nil {
		return err
	}
	if err := client.DoSocket(socketArg, socket); err != nil {
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

	config, requests, _, err := parseFile(filename)
	if err != nil {
		return err
	}
	t := templates.Get(export)
	if t == nil {
		return fmt.Errorf(" exporting language (%s) not supported", export)
	}
	treqs := []templates.Request{}
	for _, req := range requests {
		body := req.BodyRaw
		if body == "null" {
			body = ""
		}
		ua := config.UserAgent
		if ua == DefaultConfig().UserAgent {
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
		if i < len(requests)-1 {
			fmt.Printf("\n")
		}
	}

	return nil
}
