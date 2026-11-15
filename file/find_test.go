package file

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolve(t *testing.T) {
	write := func(t *testing.T, p, src string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	server := `server { address = "localhost:8080" }`
	client := `request "a" { url = "http://localhost" }`

	t.Run("one server file", func(t *testing.T) {
		// a folder named like a .rest file, w/ client files and a server
		// file in a subfolder that shouldn't count
		dir := filepath.Join(t.TempDir(), "midnight-lang.rest")
		write(t, filepath.Join(dir, "midnight-lang.rest"), "#!/usr/bin/env rest\n"+server)
		write(t, filepath.Join(dir, "client.rest"), client)
		write(t, filepath.Join(dir, "broken.rest"), "server {")
		write(t, filepath.Join(dir, "notes.txt"), server)
		write(t, filepath.Join(dir, "rest", "other.rest"), server)
		got, err := Resolve(dir)
		if err != nil {
			t.Fatal(err)
		}
		if want := filepath.Join(dir, "midnight-lang.rest"); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("none", func(t *testing.T) {
		dir := t.TempDir()
		write(t, filepath.Join(dir, "client.rest"), client)
		if _, err := Resolve(dir); err == nil || !strings.Contains(err.Error(), "no .rest file") {
			t.Errorf("got %v", err)
		}
	})

	t.Run("ambiguous", func(t *testing.T) {
		dir := t.TempDir()
		write(t, filepath.Join(dir, "a.rest"), server)
		write(t, filepath.Join(dir, "b.rest"), server)
		_, err := Resolve(dir)
		if err == nil || !strings.Contains(err.Error(), "a.rest, b.rest") {
			t.Errorf("got %v", err)
		}
	})

	t.Run("files pass through", func(t *testing.T) {
		for _, p := range []string{"", "-", "missing.rest", "x/y.rest"} {
			if got, err := Resolve(p); err != nil || got != p {
				t.Errorf("Resolve(%q) = %q, %v", p, got, err)
			}
		}
	})
}
