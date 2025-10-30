package rest

import (
	"errors"

	"github.com/taybart/rest/file"
	"github.com/taybart/rest/request"
)

type Rest struct {
	Config request.Config
	// Socket      request.Socket
	// Server      server.Config
	HCLRequests map[string]*file.HCLRequest
	Requests    map[string]request.Request
	Parser      *file.Parser
}

func NewRestFile(filename string) (Rest, error) {
	parser, err := file.NewParser(filename)
	if err != nil {
		return Rest{}, err
	}
	rest := Rest{Parser: parser, Config: parser.Config, HCLRequests: make(map[string]*file.HCLRequest)}
	for i, block := range parser.Root.Requests {
		rest.HCLRequests[block.Label] = block
		rest.HCLRequests[block.Label].BlockIndex = i
	}
	return rest, nil

}

func (rest *Rest) RequestIndex(i int) (request.Request, error) {

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

func (rest *Rest) Request(label string) (request.Request, error) {
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
