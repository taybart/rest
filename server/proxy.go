package server

import (
	"crypto/tls"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/taybart/log"
)

// Proxy forwards requests to another server. In HCL it's either a target
// string (proxy = "localhost:8081") or a proxy { } block with these fields.
type Proxy struct {
	// Target is host:port (http assumed) or a full URL; its path is prepended
	// to the request path.
	Target string `json:"target" hcl:"target"`
	// Strip removes this prefix from the request path before forwarding, at
	// a segment boundary: "/api" turns "/api/users" into "/users" but leaves
	// "/apiary" alone.
	Strip string `json:"strip,omitempty" hcl:"strip,optional"`
	// Headers are set on the outbound request, replacing any existing value.
	// An empty value removes the header. "Host" overrides the host rewrite.
	Headers map[string]string `json:"headers,omitempty" hcl:"headers,optional"`
	// Insecure skips TLS certificate verification for the target, e.g. a
	// local backend with a self-signed cert.
	Insecure bool `json:"insecure,omitempty" hcl:"insecure,optional"`
}

// ProxyHandler forwards requests to p.Target. The outbound Host header is the
// target's host (the original goes in X-Forwarded-Host), and the server's
// read/write deadlines are lifted so streams, long downloads and websockets
// aren't cut off.
func (s *Server) ProxyHandler(p *Proxy) http.HandlerFunc {
	u := s.MustParseURL(p.Target)
	strip := strings.TrimSuffix(p.Strip, "/")
	rp := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			if strip != "" {
				stripPrefix(pr.Out.URL, strip)
			}
			pr.SetURL(u)
			pr.Out.Header["X-Forwarded-For"] = pr.In.Header["X-Forwarded-For"]
			pr.SetXForwarded()
			for k, v := range p.Headers {
				switch {
				case strings.EqualFold(k, "Host"):
					pr.Out.Host = v
				case v == "":
					pr.Out.Header.Del(k)
				default:
					pr.Out.Header.Set(k, v)
				}
			}
		},
	}
	if p.Insecure {
		t := http.DefaultTransport.(*http.Transport).Clone()
		t.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		rp.Transport = t
		log.Warnf("proxy %s: TLS certificate verification disabled (insecure = true)\n", p.Target)
	}
	return func(w http.ResponseWriter, r *http.Request) {
		rc := http.NewResponseController(w)
		_ = rc.SetReadDeadline(time.Time{})
		_ = rc.SetWriteDeadline(time.Time{})
		rp.ServeHTTP(w, r)
	}
}

// stripPrefix removes prefix from u's path only at a segment boundary.
func stripPrefix(u *url.URL, prefix string) {
	cut := func(path string) (string, bool) {
		rest, ok := strings.CutPrefix(path, prefix)
		if !ok || (rest != "" && rest[0] != '/') {
			return path, false
		}
		if rest == "" {
			rest = "/"
		}
		return rest, true
	}
	path, ok := cut(u.Path)
	if !ok {
		return
	}
	u.Path = path
	if u.RawPath != "" {
		if raw, ok := cut(u.RawPath); ok {
			u.RawPath = raw
		} else {
			u.RawPath = ""
		}
	}
}
