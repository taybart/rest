package server

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/taybart/log"
)

const (
	// how often to ask tailscale for the cert again, it hands back its cached copy until renewal
	tailscaleRefresh = 12 * time.Hour
	// macOS app installs don't always put the cli on the PATH
	tailscaleMacApp = "/Applications/Tailscale.app/Contents/MacOS/Tailscale"
)

// tailscaleCert serves this machine's tailnet cert by asking the tailscale cli for it
type tailscaleCert struct {
	Domain string

	mu      sync.Mutex
	cert    *tls.Certificate
	fetched time.Time
	now     func() time.Time
	run     func(args ...string) ([]byte, error)
}

func newTailscaleCert() (*tailscaleCert, error) {
	bin, err := exec.LookPath("tailscale")
	if err != nil {
		if _, statErr := os.Stat(tailscaleMacApp); statErr != nil {
			return nil, fmt.Errorf("tailscale cli not found on PATH: %w", err)
		}
		bin = tailscaleMacApp
	}
	return loadTailscaleCert(func(args ...string) ([]byte, error) {
		var stdout, stderr bytes.Buffer
		cmd := exec.Command(bin, args...)
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("tailscale %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
		}
		return stdout.Bytes(), nil
	}, time.Now)
}

// loadTailscaleCert finds the tailnet cert domain and fetches the first cert so problems show up at startup
func loadTailscaleCert(run func(args ...string) ([]byte, error), now func() time.Time) (*tailscaleCert, error) {
	out, err := run("status", "--json")
	if err != nil {
		return nil, err
	}
	var status struct {
		BackendState string
		CertDomains  []string
		Self         struct{ DNSName string }
	}
	if err := json.Unmarshal(out, &status); err != nil {
		return nil, fmt.Errorf("could not parse tailscale status: %w", err)
	}
	if status.BackendState != "Running" {
		return nil, fmt.Errorf("tailscale is not running (state: %s)", status.BackendState)
	}
	if len(status.CertDomains) == 0 {
		return nil, errors.New("tailnet has no cert domains, enable MagicDNS and HTTPS Certificates in the tailscale admin console (DNS page)")
	}

	t := &tailscaleCert{Domain: status.CertDomains[0], run: run, now: now}
	// prefer this machine's own name if it is in the list
	self := strings.TrimSuffix(status.Self.DNSName, ".")
	for _, d := range status.CertDomains {
		if d == self {
			t.Domain = d
		}
	}
	if _, err := t.refresh(); err != nil {
		return nil, err
	}
	return t, nil
}

// refresh must be called with mu held (or before the cert is shared)
func (t *tailscaleCert) refresh() (*tls.Certificate, error) {
	// both files to stdout, X509KeyPair picks the right blocks out of the combined pem
	pem, err := t.run("cert", "--cert-file", "-", "--key-file", "-", t.Domain)
	if err != nil {
		return nil, err
	}
	cert, err := tls.X509KeyPair(pem, pem)
	if err != nil {
		return nil, fmt.Errorf("could not parse cert from tailscale: %w", err)
	}
	t.cert = &cert
	t.fetched = t.now()
	return t.cert, nil
}

// GetCertificate is for tls.Config, keeps serving the current cert if a refresh fails and it is still valid
func (t *tailscaleCert) GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.now().Sub(t.fetched) < tailscaleRefresh {
		return t.cert, nil
	}
	cert, err := t.refresh()
	if err != nil {
		// don't shell out on every handshake while tailscale is unhappy, try again in a minute
		t.fetched = t.now().Add(time.Minute - tailscaleRefresh)
		if t.cert != nil && t.cert.Leaf != nil && t.now().Before(t.cert.Leaf.NotAfter) {
			log.Errorf("could not refresh tailscale cert, using current one: %v\n", err)
			return t.cert, nil
		}
		return nil, err
	}
	return cert, nil
}
