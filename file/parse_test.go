package file_test

import (
	"testing"

	"github.com/taybart/rest/file"
	"github.com/taybart/rest/request"
)

func parse(t *testing.T, filename string, expectedReqs int) file.Rest {
	rest, err := file.Parse(filename)
	if err != nil {
		t.Fatal(err)
	}
	if len(rest.Requests) != expectedReqs {
		t.Fatalf("expected %d request(s), got %d", expectedReqs, len(rest.Requests))
	}
	return rest
}

func TestBasicParse(t *testing.T) {
	rest := parse(t, "../doc/examples/basic.rest", 1)

	basic := request.Request{
		Label:   "basic",
		URL:     "http://localhost:18080/hello-world",
		Method:  "POST",
		Headers: map[string]string{"Content-Type": "application/json"},
		Body:    `{"data":"hello world"}`,
	}

	if !rest.Requests["basic"].Equal(basic) {
		t.Fatalf("expected request to match:\n%s\n%s\n", basic, rest.Requests["basic"])
	}

}
func TestTemplateParse(t *testing.T) {
	rest := parse(t, "../doc/examples/template.rest", 1)

	templated := request.Request{
		Label:   "template",
		URL:     "http://localhost:18080/hello-world",
		Method:  "POST",
		Headers: map[string]string{"Content-Type": "application/json"},
		Body:    `{"myName":"foobah"}`,
	}

	if !rest.Requests["template"].Equal(templated) {
		t.Fatalf("expected request to match:\n%s\n%s\n", templated, rest.Requests["template"])
	}

}
func TestImportParse(t *testing.T) {
	rest := parse(t, "../doc/examples/import.rest", 2)
	headers := rest.Requests["test"].Headers
	if headers["X-imported-header"] != "success" {
		t.Fatal(`unexpected import result expected "success" got:`,
			headers["X-imported-header"])
	}
	if headers["X-imported-local"] != "success" {
		t.Fatal(`unexpected import result expected "success" got:`,
			headers["X-imported-local"])
	}
}
func TestSocketParse(t *testing.T) {
	rest := parse(t, "../doc/examples/socket.rest", 0)
	if len(rest.Socket.Playbook) != 3 {
		t.Fatal("expected 3 socket plays, got", len(rest.Socket.Playbook))
	}
	if len(rest.Socket.Run.Order) != 4 {
		t.Fatal("expected 4 socket calls in run, got", len(rest.Socket.Run.Order))
	}
}
