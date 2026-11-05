package file_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/taybart/rest"
	"github.com/taybart/rest/server"
)

func parseServer(t *testing.T, content string) (server.Config, error) {
	t.Helper()
	filename := filepath.Join(t.TempDir(), "server.rest")
	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	r, err := rest.NewFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	return r.Parser.Server()
}

func TestParseProxy(t *testing.T) {
	t.Setenv("PROXY_TOKEN", "tok")
	config, err := parseServer(t, `
server {
  address = ":0"
  proxy   = "localhost:8081"
  handler "*" "/api/" {
    proxy {
      target   = "https://api.example.com"
      strip    = "/api"
      insecure = true
      headers = {
        Authorization = "Bearer ${env("PROXY_TOKEN")}"
        Cookie        = ""
        X-Version     = 2
      }
    }
  }
  handler "GET" "/mock" {
    response {
      status = 200
    }
  }
}
`)
	if err != nil {
		t.Fatal(err)
	}
	if config.Proxy == nil || config.Proxy.Target != "localhost:8081" {
		t.Fatalf("server proxy: %+v", config.Proxy)
	}
	want := &server.Proxy{
		Target:   "https://api.example.com",
		Strip:    "/api",
		Insecure: true,
		Headers:  map[string]string{"Authorization": "Bearer tok", "Cookie": "", "X-Version": "2"},
	}
	if !reflect.DeepEqual(config.Handlers[0].Proxy, want) {
		t.Fatalf("handler proxy:\n got %+v\nwant %+v", config.Handlers[0].Proxy, want)
	}
	if config.Handlers[1].Proxy != nil {
		t.Fatalf("handler without proxy should be nil, got %+v", config.Handlers[1].Proxy)
	}
}

func TestParseProxyErrors(t *testing.T) {
	for _, c := range []struct {
		proxy string
		err   string
	}{
		{"proxy = \"\"", "target is empty"},
		{"proxy = 5", "proxy = expects a target string, got number"},
		{"proxy = { target = \"x\" }", "use a proxy { } block for options"},
		{"proxy {\n  target = \"\"\n}", "target is empty"},
		{"proxy {\n  target = \"x\"\n  strip = \"api\"\n}", "strip must start with /"},
		{"proxy = \"a\"\nproxy {\n  target = \"x\"\n}", "not both"},
		// HCL itself rejects these (details are printed as diagnostics)
		{"proxy {\n  strip = \"/api\"\n}", "error decoding"},
		{"proxy {\n  target = \"x\"\n  prefix = \"/api\"\n}", "error decoding"},
	} {
		_, err := parseServer(t, `
server {
  address = ":0"
  handler "GET" "/x" {
    `+c.proxy+`
  }
}
`)
		if err == nil || !strings.Contains(err.Error(), c.err) {
			t.Errorf("%s: expected error containing %q, got %v", c.proxy, c.err, err)
		}
	}
}
