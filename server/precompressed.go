package server

import (
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"
)

// precompressed are the encodings a directory can have files compressed ahead
// of time in, best first: x.br (or x.gz) next to x is sent in its place to a
// client that accepts it. They can be compressed as much as you like, once
// (ex. brotli -q 11, which is too slow to do per request).
var precompressed = []struct{ encoding, ext string }{
	{"br", ".br"},
	{"gzip", ".gz"},
}

// staleAfter is how much older than its original a precompressed file can be
// before it's ignored. (Not 0: copying a directory, ex. in a deploy, can write
// x.br a moment before x. A missed rebuild is usually much further off.)
const staleAfter = time.Minute

// servePrecompressed sends the precompressed version of the file r asks for,
// if there's one the client accepts. It's false if there isn't (and nothing
// was written).
//
// A precompressed file is only an alternative to a real one: x.br is used only
// if x exists, and isn't older than it (by more than staleAfter: a stale x.br
// is ignored, so a missed rebuild serves the current file, not an old one).
func servePrecompressed(w http.ResponseWriter, r *http.Request, dir http.Dir) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	// (a range indexes into the uncompressed file)
	if r.Header.Get("Range") != "" {
		return false
	}
	name := path.Clean("/" + r.URL.Path)
	if strings.HasSuffix(r.URL.Path, "/") {
		name = path.Join(name, "index.html") // (what the file server sends for a directory)
	}
	orig, err := dir.Open(name)
	if err != nil {
		return false
	}
	origInfo, err := orig.Stat()
	orig.Close()
	if err != nil || origInfo.IsDir() {
		return false
	}

	for _, p := range precompressed {
		if !acceptsEncoding(r, p.encoding) {
			continue
		}
		f, err := dir.Open(name + p.ext)
		if err != nil {
			continue
		}
		info, err := f.Stat()
		if err != nil || info.IsDir() || origInfo.ModTime().Sub(info.ModTime()) > staleAfter {
			f.Close()
			continue
		}
		h := w.Header()
		ctype := mime.TypeByExtension(path.Ext(name))
		if ctype == "" {
			ctype = "application/octet-stream" // (sniffing would read the compressed bytes)
		}
		h.Set("Content-Type", ctype)
		h.Set("Content-Encoding", p.encoding)
		h.Add("Vary", "Accept-Encoding")
		// (ServeContent leaves the length out w/ a Content-Encoding, but this is
		// the whole file: w/ it, a browser can show a big download's progress)
		h.Set("Content-Length", strconv.FormatInt(info.Size(), 10))
		http.ServeContent(w, r, name, info.ModTime(), f)
		f.Close()
		return true
	}
	return false
}

// acceptsEncoding is whether r's Accept-Encoding allows encoding (ex. "br"):
// it's named (or "*" is), w/o q=0.
func acceptsEncoding(r *http.Request, encoding string) bool {
	accepted := false
	for _, part := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		name, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		name = strings.ToLower(strings.TrimSpace(name))
		if name != encoding && name != "*" {
			continue
		}
		q := 1.0
		for _, param := range strings.Split(params, ";") {
			k, v, ok := strings.Cut(strings.TrimSpace(param), "=")
			if ok && strings.EqualFold(strings.TrimSpace(k), "q") {
				if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
					q = f
				}
			}
		}
		if name == encoding {
			return q > 0 // (the encoding itself wins over "*")
		}
		accepted = q > 0
	}
	return accepted
}
