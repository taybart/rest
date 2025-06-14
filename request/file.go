package request

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/taybart/log"
)

func parseFile(filename string) ([]Request, error) {
	file, diags := readFile(filename)
	if diags.HasErrors() {
		log.Infoln("parse")
		writeDiags(map[string]*hcl.File{filename: file}, diags)
		return nil, diags
	}

	var root Root
	diags = gohcl.DecodeBody(file.Body, nil, &root)
	if diags.HasErrors() {
		log.Infoln("decode")
		writeDiags(map[string]*hcl.File{filename: file}, diags)
		return nil, diags
	}

	locals, err := decodeLocals(root)
	if err != nil {
		return nil, err
	}

	ctx := createContext(locals)

	requests := []Request{}
	labels := []string{}
	for _, block := range root.Requests {
		req := Request{Label: block.Label}
		if diags = gohcl.DecodeBody(block.Body, ctx, &req); diags.HasErrors() {
			writeDiags(map[string]*hcl.File{filename: file}, diags)
			return nil, fmt.Errorf("error decoding HCL configuration: %w", diags)
		}
		for _, l := range labels {
			if l == req.Label {
				return nil, fmt.Errorf("labels must be unique: %s", l)
			}
		}
		if err := req.ParseBody(ctx); err != nil {
			return nil, err
		}

		requests = append(requests, req)
		labels = append(labels, req.Label)
	}
	return requests, nil
}

func RunFile(filename string) error {
	requests, err := parseFile(filename)
	if err != nil {
		return err
	}
	for _, req := range requests {
		res, err := req.Do()
		if err != nil {
			log.Errorln(err)
		}
		if res != "" {
			fmt.Println(res)
		}
	}
	return nil
}

func RunLabel(filename string, label string) error {
	requests, err := parseFile(filename)
	if err != nil {
		return err
	}
	for _, req := range requests {
		if req.Label == label {
			res, err := req.Do()
			if err != nil {
				log.Errorln(err)
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
	requests, err := parseFile(filename)
	if err != nil {
		return err
	}
	res, err := requests[block].Do()
	if err != nil {
		log.Errorln(err)
		return err
	}
	if res != "" {
		fmt.Println(res)
	}

	return nil
}
