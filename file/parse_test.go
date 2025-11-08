package file_test

import (
	"testing"

	"github.com/taybart/rest"
	"github.com/taybart/rest/request"
)

func parse(t *testing.T, filename string, expectedReqs int) *rest.Rest {
	rest, err := rest.NewFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if len(rest.Requests) != expectedReqs {
		t.Fatalf("expected %d request(s), got %d", expectedReqs, len(rest.Requests))
	}
	return rest
}

func TestBasicParse(t *testing.T) {
	rest := parse(t, "../doc/examples/client/basic.rest", 1)

	basic := request.Request{
		Label:   "basic",
		URL:     "http://localhost:18080/__echo__",
		Method:  "POST",
		Headers: map[string]string{"Content-Type": "application/json"},
		Body:    `{"data":"hello world"}`,
	}

	built, err := rest.Request("basic")
	if err != nil {
		t.Fatal(err)
	}
	if !built.Equal(basic) {
		t.Fatalf("expected request to match:\n%s\n%s\n", basic, built)
	}

}

func TestTemplateParse(t *testing.T) {
	rest := parse(t, "../doc/examples/client/template.rest", 1)

	templated := request.Request{
		Label:   "template",
		URL:     "http://localhost:18080/hello-world",
		Method:  "POST",
		Headers: map[string]string{"Content-Type": "application/json"},
		Body:    `{"myName":"foobah"}`,
	}

	built, err := rest.Request("template")
	if err != nil {
		t.Fatal(err)
	}
	if !built.Equal(templated) {
		t.Fatalf("expected request to match:\n%s\n%s\n", templated, built)
	}

}
func TestImportParse(t *testing.T) {
	rest := parse(t, "../doc/examples/client/import.rest", 2)
	built, err := rest.Request("test")
	if err != nil {
		t.Fatal(err)
	}
	headers := built.Headers
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
	rest := parse(t, "../doc/examples/client/socket.rest", 0)
	socket, err := rest.Parser.Socket()
	if err != nil {
		t.Fatal(err)
	}
	if len(socket.Playbook) != 3 {
		t.Fatal("expected 3 socket plays, got", len(socket.Playbook))
	}
	if len(socket.Run.Order) != 4 {
		t.Fatal("expected 4 socket calls in run, got", len(socket.Run.Order))
	}
}

func TestServerParse(t *testing.T) {
	// TODO: test all fields
	rest := parse(t, "../doc/examples/server/basic.rest", 0)
	config, err := rest.Parser.Server()
	if err != nil {
		t.Fatal(err)
	}
	if config.Addr != "localhost:18080" {
		t.Fatal("expected address localhost:18080, got", config.Addr)
	}
	if config.Response.Status != 418 {
		t.Fatal("expected response statuscode to be", 418, "got", config.Response.Status)
	}
	if string(config.Response.Body) != `` {
		t.Fatal("expected body to be empty got:", string(config.Response.Body))
	}
}
