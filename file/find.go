package file

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

// Resolve turns a directory into the one .rest file directly in it with a server
// block (ex. `rest -s -f /var/www/site.com`). Files in subdirectories (ex.
// imports) aren't considered. Anything that isn't a directory is returned
// as is.
func Resolve(path string) (string, error) {
	if path == "" || path == "-" {
		return path, nil
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return path, nil // reading it reports the error
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return "", err
	}
	found := []string{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".rest") {
			continue
		}
		fp := filepath.Join(path, e.Name())
		if hasServerBlock(fp) {
			found = append(found, fp)
		}
	}
	sort.Strings(found)
	switch len(found) {
	case 1:
		return found[0], nil
	case 0:
		return "", fmt.Errorf("no .rest file with a server block in %s", path)
	default:
		names := make([]string, len(found))
		for i, f := range found {
			names[i] = filepath.Base(f)
		}
		return "", fmt.Errorf("more than one .rest file with a server block in %s (%s), pick one with -f",
			path, strings.Join(names, ", "))
	}
}

// hasServerBlock checks for a top level server block without evaluating anything,
// a file that doesn't parse doesn't have one
func hasServerBlock(fp string) bool {
	src, err := os.ReadFile(fp)
	if err != nil {
		return false
	}
	f, diags := hclsyntax.ParseConfig(stripShebang(src), fp, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return false
	}
	body, ok := f.Body.(*hclsyntax.Body)
	if !ok {
		return false
	}
	for _, b := range body.Blocks {
		if b.Type == "server" {
			return true
		}
	}
	return false
}
