package server

import (
	"compress/gzip"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
)

// don't bother compressing bodies smaller than this (when the size is known up front)
const gzipMinSize = 1024

// gzipResponseWriter compresses the response only when it is worth it. The decision is made
// lazily, once the handler has set its headers: already-compressed media, partial content and
// tiny bodies pass through untouched, keeping their Content-Length (and range/resume support).
type gzipResponseWriter struct {
	http.ResponseWriter
	r       *http.Request
	gz      *gzip.Writer
	status  int
	decided bool
}

// WriteHeader defers the status until the first write, when the content type is known
func (w *gzipResponseWriter) WriteHeader(code int) {
	if w.decided {
		return
	}
	if code >= 100 && code <= 199 {
		w.ResponseWriter.WriteHeader(code)
		return
	}
	w.status = code
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	if !w.decided {
		if w.Header().Get("Content-Type") == "" && len(b) > 0 {
			w.Header().Set("Content-Type", http.DetectContentType(b))
		}
		w.decide()
	}
	if w.gz != nil {
		return w.gz.Write(b)
	}
	return w.ResponseWriter.Write(b)
}

// ReadFrom keeps the sendfile fast path for large uncompressed files
func (w *gzipResponseWriter) ReadFrom(src io.Reader) (int64, error) {
	if !w.decided && w.Header().Get("Content-Type") == "" {
		// need the first bytes to sniff a content type, go through Write
		return io.Copy(struct{ io.Writer }{w}, src)
	}
	w.decide()
	if w.gz != nil {
		return io.Copy(w.gz, src)
	}
	if rf, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		return rf.ReadFrom(src)
	}
	return io.Copy(w.ResponseWriter, src)
}

func (w *gzipResponseWriter) Flush() {
	if !w.decided {
		w.decide()
	}
	if w.gz != nil {
		w.gz.Flush()
	}
	_ = http.NewResponseController(w.ResponseWriter).Flush()
}

// Unwrap lets http.ResponseController reach the underlying writer (e.g. to clear deadlines)
func (w *gzipResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *gzipResponseWriter) decide() {
	w.decided = true
	h := w.Header()
	if w.shouldCompress() {
		h.Del("Content-Length")
		h.Set("Content-Encoding", "gzip")
		w.gz = gzip.NewWriter(w.ResponseWriter)
	}
	if isCompressible(h.Get("Content-Type")) {
		h.Add("Vary", "Accept-Encoding")
	}
	w.ResponseWriter.WriteHeader(w.status)
}

func (w *gzipResponseWriter) shouldCompress() bool {
	h := w.Header()
	if w.r.Method == http.MethodHead || w.status != http.StatusOK {
		return false
	}
	if h.Get("Content-Encoding") != "" || h.Get("Content-Range") != "" {
		return false
	}
	if n, err := strconv.Atoi(h.Get("Content-Length")); err == nil && n < gzipMinSize {
		return false
	}
	return isCompressible(h.Get("Content-Type"))
}

// close finishes the gzip stream, and writes the header if the handler never wrote a body
func (w *gzipResponseWriter) close() {
	if !w.decided {
		w.decide()
	}
	if w.gz != nil {
		w.gz.Close()
	}
}

func isCompressible(contentType string) bool {
	mt, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	if strings.HasPrefix(mt, "text/") || strings.HasSuffix(mt, "+json") || strings.HasSuffix(mt, "+xml") {
		return true
	}
	switch mt {
	case "application/json", "application/javascript", "application/x-javascript",
		"application/xml", "application/wasm", "application/x-ndjson",
		"image/svg+xml", "font/ttf", "font/otf", "application/vnd.ms-fontobject":
		return true
	}
	return false
}

func gzipHandler(fn http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// byte ranges index into the uncompressed file, so never mix them with gzip
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") || r.Header.Get("Range") != "" {
			fn(w, r)
			return
		}
		gzr := &gzipResponseWriter{ResponseWriter: w, r: r, status: http.StatusOK}
		defer gzr.close()
		fn(gzr, r)
	}
}
