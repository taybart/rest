package request

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/taybart/log"
)

func RunFile(filename string) error {
	file, diags := readFile(filename)
	if diags.HasErrors() {
		log.Infoln("parse")
		writeDiags(map[string]*hcl.File{filename: file}, diags)
		return diags
	}

	var root Root
	diags = gohcl.DecodeBody(file.Body, nil, &root)
	if diags.HasErrors() {
		log.Infoln("decode")
		writeDiags(map[string]*hcl.File{filename: file}, diags)
		return diags
	}

	locals, err := decodeLocals(root)
	if err != nil {
		return err
	}

	ctx := createContext(locals)

	for _, block := range root.Requests {
		var req Request
		if diags = gohcl.DecodeBody(block.Body, ctx, &req); diags.HasErrors() {
			writeDiags(map[string]*hcl.File{filename: file}, diags)
			return fmt.Errorf("error decoding HCL configuration: %w", diags)
		}
		err := req.Do()
		if err != nil {
			log.Errorln(err)
		}
	}

	return nil
}
