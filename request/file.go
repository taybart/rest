package request

import (
	"fmt"
	"os"

	"github.com/taybart/log"
	"github.com/taybart/rest/request/templates"
)

func RunFile(filename string) error {
	config, requests, err := parseFile(filename)
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
			log.Errorln(err)
		}
		if res != "" {
			fmt.Println(res)
		}
	}
	return nil
}

func RunLabel(filename string, label string) error {
	config, requests, err := parseFile(filename)
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
	config, requests, err := parseFile(filename)
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

func ExportFile(filename, export string) error {
	return fmt.Errorf("request generation not implemented...yet")
	_, requests, err := parseFile(filename)
	if err != nil {
		return err
	}

	curlTmpl := `echo "running {{.Label}}" && \
curl -X {{.Method}} {{.URL}}{{if .Headers}}\{{range $header := .Headers}} 
-H {{ $header }}
{{end}}{{end}} {{if .BodyRaw}}\
-d '{{.BodyRaw}}'{{end}}`
	t := templates.NewVanilla(curlTmpl)
	t.Execute(os.Stdout, requests[0])

	return nil
}
