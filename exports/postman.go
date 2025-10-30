package exports

import (
	"os"
	"strings"

	"github.com/taybart/rest/exports/postman"
	"github.com/taybart/rest/file"
)

func ToPostmanCollection(filename, label string, block int) error {
	rest, err := file.Parse(filename)
	if err != nil {
		return err
	}
	c := postman.CreateCollection(filename, "collection")

	g := c.AddItemGroup(filename)
	for _, r := range rest.Requests {
		item := &postman.Items{
			Name: r.Label,
			Request: &postman.Request{
				URL:    &postman.URL{Raw: r.URL},
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
			item.Auth = postman.CreateAuth(postman.Basic, postman.CreateAuthParam(ba[0], ba[1]))
		}
		if r.BearerToken != "" {
			// fmt.Println("BEARER", r.BearerToken)
			item.Auth = postman.CreateAuth(postman.Bearer, postman.CreateAuthParam("bearer", r.BearerToken))
		}
		g.AddItem(item)
	}

	err = c.Write(os.Stdout, postman.V210)

	if err != nil {
		return err
	}

	return nil
}
