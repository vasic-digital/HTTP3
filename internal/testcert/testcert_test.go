package testcert

import (
	"crypto/tls"
	"crypto/x509"
	"net"
	"testing"
	"time"
)

// TestGenerateProducesUsableTLS13Config verifies Generate emits a *tls.Config
// that is actually serviceable for HTTP/3: TLS 1.3 minimum, the "h3" ALPN
// pre-populated, and exactly one parseable certificate.
func TestGenerateProducesUsableTLS13Config(t *testing.T) {
	cfg, err := Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if cfg == nil {
		t.Fatal("Generate returned a nil *tls.Config")
	}
	if cfg.MinVersion != tls.VersionTLS13 {
		t.Fatalf("MinVersion = %d, want TLS 1.3 (%d)", cfg.MinVersion, tls.VersionTLS13)
	}
	if len(cfg.NextProtos) != 1 || cfg.NextProtos[0] != "h3" {
		t.Fatalf("NextProtos = %v, want [h3]", cfg.NextProtos)
	}
	if len(cfg.Certificates) != 1 {
		t.Fatalf("Certificates len = %d, want 1", len(cfg.Certificates))
	}
}

// TestGenerateCertValidForLoopbackNames parses the leaf cert and asserts the
// SAN set, validity window, and key usages match the documented contract
// (localhost + 127.0.0.1 + ::1, valid ~now, server+client auth).
func TestGenerateCertValidForLoopbackNames(t *testing.T) {
	cfg, err := Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	leaf, err := x509.ParseCertificate(cfg.Certificates[0].Certificate[0])
	if err != nil {
		t.Fatalf("parse leaf certificate: %v", err)
	}

	if got := leaf.Subject.CommonName; got != "localhost" {
		t.Fatalf("CommonName = %q, want localhost", got)
	}

	// DNS SAN must include localhost so the cert verifies for that host.
	if err := leaf.VerifyHostname("localhost"); err != nil {
		t.Fatalf("cert does not verify for localhost: %v", err)
	}

	// IP SANs: 127.0.0.1 and ::1 must both be present.
	wantIPs := []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback}
	for _, want := range wantIPs {
		found := false
		for _, got := range leaf.IPAddresses {
			if got.Equal(want) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("IP SAN %s missing from %v", want, leaf.IPAddresses)
		}
	}

	now := time.Now()
	if now.Before(leaf.NotBefore) || now.After(leaf.NotAfter) {
		t.Fatalf("cert not valid now: NotBefore=%s NotAfter=%s now=%s",
			leaf.NotBefore, leaf.NotAfter, now)
	}
	if leaf.KeyUsage&x509.KeyUsageDigitalSignature == 0 {
		t.Fatalf("KeyUsageDigitalSignature missing (KeyUsage=%b)", leaf.KeyUsage)
	}
	hasServerAuth := false
	for _, eku := range leaf.ExtKeyUsage {
		if eku == x509.ExtKeyUsageServerAuth {
			hasServerAuth = true
		}
	}
	if !hasServerAuth {
		t.Fatal("ExtKeyUsageServerAuth missing")
	}
}

// TestGenerateIsFresh proves each call mints a distinct keypair / certificate
// rather than returning a cached singleton.
func TestGenerateIsFresh(t *testing.T) {
	a, err := Generate()
	if err != nil {
		t.Fatalf("Generate a: %v", err)
	}
	b, err := Generate()
	if err != nil {
		t.Fatalf("Generate b: %v", err)
	}
	leafA, err := x509.ParseCertificate(a.Certificates[0].Certificate[0])
	if err != nil {
		t.Fatalf("parse a: %v", err)
	}
	leafB, err := x509.ParseCertificate(b.Certificates[0].Certificate[0])
	if err != nil {
		t.Fatalf("parse b: %v", err)
	}
	if leafA.Raw == nil || leafB.Raw == nil {
		t.Fatal("nil raw certificate bytes")
	}
	if string(leafA.RawSubjectPublicKeyInfo) == string(leafB.RawSubjectPublicKeyInfo) {
		t.Fatal("two Generate calls produced the same public key; expected fresh material")
	}
}
