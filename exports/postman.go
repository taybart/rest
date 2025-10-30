package exports

import (
	"errors"
	"os"
	"sort"
	"strings"

	"github.com/taybart/rest/exports/postman"
	"github.com/taybart/rest/file"
	"github.com/taybart/rest/request"
	"github.com/taybart/rest/server"
)

type restFile struct {
	Config      request.Config
	Socket      request.Socket
	Server      server.Config
	HCLRequests map[string]*file.HCLRequest
	Requests    map[string]request.Request
	Parser      *file.Parser
}

func (rest *restFile) Request(label string) (request.Request, error) {
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

func parseFile(filename string) (*restFile, error) {
	parser, err := file.NewParser(filename)
	if err != nil {
		return nil, err
	}
	rest := &restFile{
		Parser:      parser,
		Config:      parser.Config,
		HCLRequests: make(map[string]*file.HCLRequest),
		Requests:    make(map[string]request.Request),
	}
	for i, block := range parser.Root.Requests {
		rest.HCLRequests[block.Label] = block
		rest.HCLRequests[block.Label].BlockIndex = i
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
			return nil, err
		}
		rest.Requests[label] = req
	}
	return rest, nil
}

func ToPostmanCollection(filename, label string, block int) error {
	rest, err := parseFile(filename)
	if err != nil {
		return err
	}
	c := postman.CreateCollection(filename, "collection")

	for _, r := range rest.Requests {
		item := &postman.Items{
			Name: r.Label,
			Request: &postman.Request{
				URL:    postman.MustURL(r.URL),
				Method: postman.Method(r.Method),
				Body: &postman.Body{
					Mode: "raw",
					Raw:  r.Body,
				},
			},
		}
		for k, v := range r.Headers {
			item.Request.Header = append(item.Request.Header, &postman.Header{Key: k, Value: v})
		}
		query := make([]*postman.QueryParam, len(r.Query))
		i := 0
		for k, v := range r.Query {
			query[i] = &postman.QueryParam{Key: k, Value: v}
			i += 1
		}
		item.Request.URL.Query = query
		if r.BasicAuth != "" {
			ba := strings.Split(r.BasicAuth, ":")
			item.Request.Auth = postman.CreateAuth(
				postman.Basic,
				postman.CreateAuthParam("username", ba[0]),
				postman.CreateAuthParam("password", ba[1]),
			)
		}
		if r.BearerToken != "" {
			item.Request.Auth = postman.CreateAuth(
				postman.Bearer,
				postman.CreateAuthParam("token", r.BearerToken),
			)
		}
		c.AddItem(item)
	}

	if err := c.Write(os.Stdout, postman.V210); err != nil {
		return err
	}

	return nil
}
