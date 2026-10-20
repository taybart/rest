package server

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"strings"
	"testing"
	"time"
)

// selfSigned returns a cert and key pem the way `tailscale cert --cert-file - --key-file -` prints them
func selfSigned(t *testing.T, domain string, notAfter time.Time) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: domain},
		DNSNames:     []string{domain},
		NotBefore:    notAfter.Add(-90 * 24 * time.Hour),
		NotAfter:     notAfter,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	out := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	return append(out, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})...)
}

type fakeTailscale struct {
	status   string
	certPEM  []byte
	certErr  error
	certRuns int
}

func (f *fakeTailscale) run(args ...string) ([]byte, error) {
	switch args[0] {
	case "status":
		return []byte(f.status), nil
	case "cert":
		f.certRuns++
		if f.certErr != nil {
			return nil, f.certErr
		}
		return f.certPEM, nil
	}
	return nil, errors.New("unexpected command")
}

func TestTailscaleCertDomain(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name, status, domain, err string
	}{
		{
			name:   "prefers own name",
			status: `{"BackendState":"Running","CertDomains":["other.tail1.ts.net","box.tail1.ts.net"],"Self":{"DNSName":"box.tail1.ts.net."}}`,
			domain: "box.tail1.ts.net",
		},
		{
			name:   "falls back to first cert domain",
			status: `{"BackendState":"Running","CertDomains":["box.tail1.ts.net"],"Self":{"DNSName":"renamed.tail1.ts.net."}}`,
			domain: "box.tail1.ts.net",
		},
		{
			name:   "https not enabled",
			status: `{"BackendState":"Running","CertDomains":null,"Self":{"DNSName":"box.tail1.ts.net."}}`,
			err:    "HTTPS Certificates",
		},
		{
			name:   "not running",
			status: `{"BackendState":"Stopped"}`,
			err:    "not running",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeTailscale{status: tt.status, certPEM: selfSigned(t, tt.domain, now.Add(60*24*time.Hour))}
			ts, err := loadTailscaleCert(f.run, func() time.Time { return now })
			if tt.err != "" {
				if err == nil || !strings.Contains(err.Error(), tt.err) {
					t.Fatalf("expected error containing %q, got %v", tt.err, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if ts.Domain != tt.domain {
				t.Errorf("expected domain %q, got %q", tt.domain, ts.Domain)
			}
		})
	}
}

func TestTailscaleCertRefresh(t *testing.T) {
	now := time.Now()
	clock := func() time.Time { return now }
	f := &fakeTailscale{
		status:  `{"BackendState":"Running","CertDomains":["box.tail1.ts.net"],"Self":{"DNSName":"box.tail1.ts.net."}}`,
		certPEM: selfSigned(t, "box.tail1.ts.net", now.Add(30*24*time.Hour)),
	}
	ts, err := loadTailscaleCert(f.run, clock)
	if err != nil {
		t.Fatal(err)
	}

	first, err := ts.GetCertificate(nil)
	if err != nil || first == nil || first.Leaf.Subject.CommonName != "box.tail1.ts.net" {
		t.Fatalf("expected initial cert, got %v, %v", first, err)
	}
	if f.certRuns != 1 {
		t.Errorf("expected cached cert within refresh window, cli ran %d times", f.certRuns)
	}

	// past the refresh window a renewed cert is picked up
	now = now.Add(tailscaleRefresh + time.Minute)
	f.certPEM = selfSigned(t, "box.tail1.ts.net", now.Add(90*24*time.Hour))
	renewed, err := ts.GetCertificate(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !renewed.Leaf.NotAfter.After(first.Leaf.NotAfter) {
		t.Errorf("expected renewed cert, still expires %v", renewed.Leaf.NotAfter)
	}

	// a failed refresh keeps serving the still-valid cert, and doesn't retry on every handshake
	now = now.Add(tailscaleRefresh + time.Minute)
	f.certErr = errors.New("tailscaled unreachable")
	runs := f.certRuns
	kept, err := ts.GetCertificate(nil)
	if err != nil || kept != renewed {
		t.Fatalf("expected current cert on refresh failure, got %v, %v", kept, err)
	}
	if _, err := ts.GetCertificate(nil); err != nil || f.certRuns != runs+1 {
		t.Errorf("expected one retry per minute, cli ran %d more times, err %v", f.certRuns-runs, err)
	}

	// once the cert has expired a failed refresh is an error
	now = renewed.Leaf.NotAfter.Add(time.Hour)
	if cert, err := ts.GetCertificate(nil); err == nil {
		t.Errorf("expected error with expired cert and failing cli, got %v", cert)
	}
}
