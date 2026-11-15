package file_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/taybart/rest/file"
)

// a parse's sources: the file, its imports, and what read() read, but not
// what pages require at runtime
func TestSources(t *testing.T) {
	dir := t.TempDir()
	write := func(name, src string) string {
		p := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	page := write("src/app.mn", "return <p>hi</p>")
	imp := write("rest/hn.rest", `request "hn" { url = "http://localhost" }`)
	main := write("site.rest", `imports = ["./rest/hn.rest"]
server {
  address = "localhost:0"
  handler "GET" "/" {
    page = read("`+page+`")
  }
}`)
	p, err := file.NewParser(main)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Server(); err != nil {
		t.Fatal(err)
	}
	got := p.Sources()
	for _, want := range []string{main, imp, page} {
		if !slices.Contains(got, want) {
			t.Errorf("missing %s in %v", want, got)
		}
	}
	if len(got) != 3 {
		t.Errorf("got %v", got)
	}
}
