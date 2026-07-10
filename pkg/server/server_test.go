package server

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"strings"
	"testing"

	"digital.vasic.http3/internal/testcert"
	"github.com/quic-go/quic-go/http3"
)

// must builds a minimal valid Config for table-driven tests.
func must(t *testing.T) Config {
	t.Helper()
	tc, err := testcert.Generate()
	if err != nil {
		t.Fatalf("testcert.Generate: %v", err)
	}
	return Config{
		Addr:    "127.0.0.1:0",
		Handler: http.NewServeMux(),
		TLSConf: tc,
	}
}

func TestConfigValidate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{
			name:    "valid config passes",
			mutate:  func(c *Config) {},
			wantErr: "",
		},
		{
			name:    "missing Addr",
			mutate:  func(c *Config) { c.Addr = "" },
			wantErr: "Addr is required",
		},
		{
			name:    "missing Handler",
			mutate:  func(c *Config) { c.Handler = nil },
			wantErr: "Handler is required",
		},
		{
			name:    "missing TLSConf",
			mutate:  func(c *Config) { c.TLSConf = nil },
			wantErr: "TLSConf is required",
		},
		{
			name: "TLSConf without Certificates and without GetCertificate",
			mutate: func(c *Config) {
				c.TLSConf = &tls.Config{MinVersion: tls.VersionTLS13}
			},
			wantErr: "must provide Certificates, GetCertificate, or GetConfigForClient",
		},
		{
			name: "TLSConf with too-low MinVersion",
			mutate: func(c *Config) {
				c.TLSConf.MinVersion = tls.VersionTLS12
			},
			wantErr: "MinVersion must be tls.VersionTLS13",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := must(t)
			tc.mutate(&cfg)
			err := cfg.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected nil error, got: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got: %v", tc.wantErr, err)
			}
		})
	}
}

// TestConfigValidateAcceptsGetConfigForClientOnly is a regression test for a
// false-negative validation defect: Config.Validate rejected any TLSConf that
// relied solely on GetConfigForClient (no top-level Certificates or
// GetCertificate) even though GetConfigForClient is a first-class,
// quic-go-supported certificate source (see quic-go/http3.ConfigureTLSConfig,
// which explicitly wraps a caller-supplied GetConfigForClient) used for
// per-SNI cert selection and ACME/autocert-style rotation. Before the fix,
// this legitimate, real-world Config was wrongly rejected at construction
// time with "must provide Certificates or GetCertificate".
func TestConfigValidateAcceptsGetConfigForClientOnly(t *testing.T) {
	t.Parallel()
	cfg := must(t)
	cfg.TLSConf = &tls.Config{
		MinVersion: tls.VersionTLS13,
		GetConfigForClient: func(*tls.ClientHelloInfo) (*tls.Config, error) {
			tc, err := testcert.Generate()
			if err != nil {
				return nil, err
			}
			return tc, nil
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() rejected a Config whose only cert source is "+
			"GetConfigForClient: %v", err)
	}
}

// TestNewSucceedsWithGetConfigForClientOnly proves the same invariant holds
// through the full New() construction path, not just Validate() in isolation.
func TestNewSucceedsWithGetConfigForClientOnly(t *testing.T) {
	t.Parallel()
	cfg := must(t)
	cfg.TLSConf = &tls.Config{
		GetConfigForClient: func(*tls.ClientHelloInfo) (*tls.Config, error) {
			tc, err := testcert.Generate()
			if err != nil {
				return nil, err
			}
			return tc, nil
		},
	}
	if _, err := New(cfg); err != nil {
		t.Fatalf("New() rejected a Config whose only cert source is "+
			"GetConfigForClient: %v", err)
	}
}

func TestNewForcesTLS13MinVersionAndH3ALPN(t *testing.T) {
	t.Parallel()
	cfg := must(t)
	cfg.TLSConf.MinVersion = 0   // caller did not set it
	cfg.TLSConf.NextProtos = nil // caller did not set ALPN

	srv, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if srv.cfg.TLSConf.MinVersion != tls.VersionTLS13 {
		t.Fatalf("expected MinVersion forced to TLS13, got %d", srv.cfg.TLSConf.MinVersion)
	}
	if len(srv.cfg.TLSConf.NextProtos) == 0 || srv.cfg.TLSConf.NextProtos[0] != http3.NextProtoH3 {
		t.Fatalf("expected NextProtos[0] == %q, got %v", http3.NextProtoH3, srv.cfg.TLSConf.NextProtos)
	}
}

func TestStartTwiceReturnsErrAlreadyStarted(t *testing.T) {
	t.Parallel()
	cfg := must(t)
	srv, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Drive a fake first start by setting the flag without binding.
	srv.mu.Lock()
	srv.started = true
	srv.mu.Unlock()

	err = srv.Start()
	if !errors.Is(err, ErrAlreadyStarted) {
		t.Fatalf("expected ErrAlreadyStarted, got %v", err)
	}
}

func TestShutdownIsIdempotent(t *testing.T) {
	t.Parallel()
	cfg := must(t)
	srv, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Without Start, Shutdown is a no-op and must not panic.
	if err := srv.Shutdown(context.Background()); err != nil {
		t.Fatalf("first Shutdown: %v", err)
	}
	if err := srv.Shutdown(context.Background()); err != nil {
		t.Fatalf("second Shutdown: %v", err)
	}
}

func TestNewPreservesPreExistingALPN(t *testing.T) {
	t.Parallel()
	cfg := must(t)
	cfg.TLSConf.NextProtos = []string{"h3", "extra"}

	srv, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got := srv.cfg.TLSConf.NextProtos
	if len(got) != 2 || got[0] != "h3" || got[1] != "extra" {
		t.Fatalf("expected [h3 extra], got %v", got)
	}
}

func TestAddrReturnsConfiguredAddress(t *testing.T) {
	t.Parallel()
	cfg := must(t)
	cfg.Addr = "127.0.0.1:42424"
	srv, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if srv.Addr() != "127.0.0.1:42424" {
		t.Fatalf("Addr() = %q, want %q", srv.Addr(), "127.0.0.1:42424")
	}
}
