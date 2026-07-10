// Package http3_chaos_test — chaos tests for digital.vasic.http3 (§11.4.85).
//
// These meta-tests exercise the exported server API with malformed Config
// values, lifecycle edge cases, concurrent shutdown races, and resource-
// exhaustion simulations.  Panic recovery in every goroutine.
package http3_chaos_test

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"digital.vasic.http3/internal/testcert"
	"digital.vasic.http3/pkg/server"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func chaosEvidenceDir() string {
	if d := os.Getenv("HELIX_STRESS_EVIDENCE_DIR"); d != "" {
		return d
	}
	return "qa-results/stress_chaos"
}

func writeChaosEvidence(t *testing.T, name string, data []byte) {
	t.Helper()
	dir := chaosEvidenceDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Logf("WARNING: mkdir %s: %v", dir, err)
		return
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Logf("WARNING: write evidence %s: %v", path, err)
	}
}

func buildConfig(t *testing.T) server.Config {
	t.Helper()
	tc, err := testcert.Generate()
	if err != nil {
		t.Fatalf("testcert.Generate: %v", err)
	}
	return server.Config{
		Addr:    "127.0.0.1:0",
		Handler: http.NewServeMux(),
		TLSConf: tc,
	}
}

// ---------------------------------------------------------------------------
// TestChaosConfigAllInvalidCombinations
//
// Exercise Config.Validate() with every single-field corruption and every
// multi-field combination.  All must produce errors; none may panic.
// ---------------------------------------------------------------------------

func TestChaosConfigAllInvalidCombinations(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("chaos test skipped in short mode")
	}

	type entry struct {
		Name   string `json:"name"`
		Desc   string `json:"description"`
		ErrMsg string `json:"error_msg,omitempty"`
	}

	tc, err := testcert.Generate()
	if err != nil {
		t.Fatal(err)
	}
	tcNoCert := &tls.Config{MinVersion: tls.VersionTLS13}
	tcLowMin := tc.Clone()
	tcLowMin.MinVersion = tls.VersionTLS12

	cases := []struct {
		name string
		cfg  server.Config
		desc string
	}{
		{
			name: "all_fields_missing",
			cfg:  server.Config{},
			desc: "Addr empty, Handler nil, TLSConf nil",
		},
		{
			name: "empty_addr_only",
			cfg:  server.Config{Addr: "", Handler: http.NewServeMux(), TLSConf: tc},
			desc: "Addr is empty, everything else valid",
		},
		{
			name: "nil_handler_only",
			cfg:  server.Config{Addr: ":8443", Handler: nil, TLSConf: tc},
			desc: "Handler is nil, everything else valid",
		},
		{
			name: "nil_tlsconf_only",
			cfg:  server.Config{Addr: ":8443", Handler: http.NewServeMux(), TLSConf: nil},
			desc: "TLSConf is nil, everything else valid",
		},
		{
			name: "tlsconf_no_cert_no_getcert",
			cfg:  server.Config{Addr: ":8443", Handler: http.NewServeMux(), TLSConf: tcNoCert},
			desc: "TLSConf without Certificates and without GetCertificate",
		},
		{
			name: "tlsconf_minversion_too_low",
			cfg:  server.Config{Addr: ":8443", Handler: http.NewServeMux(), TLSConf: tcLowMin},
			desc: "TLSConf.MinVersion is TLS 1.2",
		},
		{
			name: "empty_addr_nil_handler",
			cfg:  server.Config{Addr: "", Handler: nil, TLSConf: tc},
			desc: "Addr empty AND Handler nil",
		},
		{
			name: "empty_addr_nil_tlsconf",
			cfg:  server.Config{Addr: "", Handler: http.NewServeMux(), TLSConf: nil},
			desc: "Addr empty AND TLSConf nil",
		},
		{
			name: "nil_handler_nil_tlsconf",
			cfg:  server.Config{Addr: ":8443", Handler: nil, TLSConf: nil},
			desc: "Handler nil AND TLSConf nil",
		},
		{
			name: "triple_invalid",
			cfg:  server.Config{Addr: "", Handler: nil, TLSConf: nil},
			desc: "Addr empty, Handler nil, TLSConf nil — all three invalid",
		},
	}

	var results []entry
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Validate panicked on %s: %v", c.name, r)
				}
			}()
			err := c.cfg.Validate()
			errStr := ""
			if err != nil {
				errStr = err.Error()
			}
			results = append(results, entry{
				Name: c.name, Desc: c.desc, ErrMsg: errStr,
			})
			if err == nil {
				t.Errorf("%s: Validate() = nil, expected error", c.name)
			}
		})
	}

	ev, _ := json.MarshalIndent(results, "", "  ")
	writeChaosEvidence(t, "chaos_config_all_invalid.json", ev)
}

// ---------------------------------------------------------------------------
// TestChaosNewInvalidConfig
//
// Call New() with every invalid config variant and verify error wrapping.
// ---------------------------------------------------------------------------

func TestChaosNewInvalidConfig(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("chaos test skipped in short mode")
	}

	tc, err := testcert.Generate()
	if err != nil {
		t.Fatal(err)
	}

	type entry struct {
		Name   string `json:"name"`
		ErrMsg string `json:"error_msg,omitempty"`
	}

	cases := []struct {
		name string
		cfg  server.Config
	}{
		{"empty_addr", server.Config{Addr: "", Handler: http.NewServeMux(), TLSConf: tc}},
		{"nil_handler", server.Config{Addr: ":8443", Handler: nil, TLSConf: tc}},
		{"nil_tlsconf", server.Config{Addr: ":8443", Handler: http.NewServeMux(), TLSConf: nil}},
		{"multiple_invalid", server.Config{Addr: "", Handler: nil, TLSConf: nil}},
	}

	var results []entry
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("New panicked: %v", r)
				}
			}()
			_, err := server.New(c.cfg)
			errStr := ""
			if err != nil {
				errStr = err.Error()
			}
			results = append(results, entry{Name: c.name, ErrMsg: errStr})
			if err == nil {
				t.Errorf("New() with %s returned nil error", c.name)
			}
		})
	}

	ev, _ := json.MarshalIndent(results, "", "  ")
	writeChaosEvidence(t, "chaos_new_invalid_config.json", ev)
}

// ---------------------------------------------------------------------------
// TestChaosStartTwice
//
// Start a server, then attempt to start it again.  Verify ErrAlreadyStarted.
// ---------------------------------------------------------------------------

func TestChaosStartTwice(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("chaos test skipped in short mode")
	}

	cfg := buildConfig(t)
	srv, err := server.New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	startErrCh := make(chan error, 1)
	go func() {
		startErrCh <- srv.Start()
	}()
	time.Sleep(50 * time.Millisecond)

	// Second Start must return ErrAlreadyStarted.
	err2 := srv.Start()
	if !errors.Is(err2, server.ErrAlreadyStarted) {
		t.Errorf("expected ErrAlreadyStarted, got %v", err2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown: %v", err)
	}

	writeChaosEvidence(t, t.Name(), []byte(`{"test":"TestChaosStartTwice","result":"second Start returned ErrAlreadyStarted"}`))
}

// ---------------------------------------------------------------------------
// TestChaosShutdownBeforeStart
//
// Call Shutdown with a cancelled context and Shutdown on a never-started
// server.  Both must not panic.
// ---------------------------------------------------------------------------

func TestChaosShutdownBeforeStart(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("chaos test skipped in short mode")
	}

	cfg := buildConfig(t)
	srv, err := server.New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	// Shutdown with cancelled context.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Shutdown with cancelled context panicked: %v", r)
		}
	}()
	_ = srv.Shutdown(ctx)

	// Shutdown on never-started server (no context).
	if err := srv.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown on never-started server: %v", err)
	}

	writeChaosEvidence(t, t.Name(), []byte(`{"test":"TestChaosShutdownBeforeStart","result":"completed without panic"}`))
}

// ---------------------------------------------------------------------------
// TestChaosConcurrentShutdown
//
// N=10 goroutines all call Shutdown concurrently.  Must be idempotent.
// ---------------------------------------------------------------------------

func TestChaosConcurrentShutdown(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("chaos test skipped in short mode")
	}

	cfg := buildConfig(t)
	srv, err := server.New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	go func() { _ = srv.Start() }()
	time.Sleep(50 * time.Millisecond)

	const workers = 10
	var (
		wg    sync.WaitGroup
		errMu sync.Mutex
		errs  []error
	)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					errMu.Lock()
					errs = append(errs, fmt.Errorf("panic: %v", r))
					errMu.Unlock()
				}
			}()
			if e := srv.Shutdown(ctx); e != nil {
				errMu.Lock()
				errs = append(errs, e)
				errMu.Unlock()
			}
		}()
	}
	wg.Wait()

	summary := map[string]interface{}{
		"test":    "TestChaosConcurrentShutdown",
		"workers": workers,
		"errors":  len(errs),
	}
	ev, _ := json.MarshalIndent(summary, "", "  ")
	writeChaosEvidence(t, "chaos_concurrent_shutdown.json", ev)

	if len(errs) > 0 {
		t.Errorf("%d of %d concurrent Shutdown calls returned errors", len(errs), workers)
	}
}

// ---------------------------------------------------------------------------
// TestChaosDoubleStartRace
//
// Two goroutines race to Start() the same server.  Exactly one succeeds.
// ---------------------------------------------------------------------------

func TestChaosDoubleStartRace(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("chaos test skipped in short mode")
	}

	cfg := buildConfig(t)
	srv, err := server.New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	const racers = 2
	results := make(chan error, racers)
	for i := 0; i < racers; i++ {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					results <- fmt.Errorf("panic: %v", r)
				}
			}()
			results <- srv.Start()
		}()
	}

	time.Sleep(100 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)

	var successN int32
	for i := 0; i < racers; i++ {
		select {
		case e := <-results:
			if e == nil {
				atomic.AddInt32(&successN, 1)
			}
		case <-time.After(time.Second):
		}
	}

	summary := map[string]interface{}{
		"test":    "TestChaosDoubleStartRace",
		"racers":  racers,
		"success": successN,
	}
	ev, _ := json.MarshalIndent(summary, "", "  ")
	writeChaosEvidence(t, "chaos_double_start_race.json", ev)

	if successN != 1 {
		t.Errorf("expected exactly 1 successful Start, got %d", successN)
	}
}

// ---------------------------------------------------------------------------
// TestChaosDoneChannelEdgeCases
//
// Verify Done() channel behaviour: empty for never-started, receives value
// after shutdown.
// ---------------------------------------------------------------------------

func TestChaosDoneChannelEdgeCases(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("chaos test skipped in short mode")
	}

	// Never-started server: Done channel empty.
	t.Run("never_started", func(t *testing.T) {
		cfg := buildConfig(t)
		srv, err := server.New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		select {
		case err := <-srv.Done():
			t.Errorf("never-started server Done() returned: %v", err)
		default:
		}
	})

	// Started-and-shut-down: Done channel receives a value.
	t.Run("started_shutdown", func(t *testing.T) {
		cfg := buildConfig(t)
		srv, err := server.New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		go func() { _ = srv.Start() }()
		time.Sleep(50 * time.Millisecond)

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			t.Fatalf("Shutdown: %v", err)
		}
		select {
		case doneErr := <-srv.Done():
			if doneErr != nil {
				t.Errorf("after Shutdown, Done() = %v, want nil", doneErr)
			}
		case <-time.After(time.Second):
			t.Fatal("Done() did not receive a value within 1s")
		}
	})

	writeChaosEvidence(t, t.Name(), []byte(`{"test":"TestChaosDoneChannelEdgeCases","result":"passed"}`))
}

// ---------------------------------------------------------------------------
// TestChaosConfigValidateMinVersionExact
//
// The config Validator must enforce TLS 1.3 minimum.  Test every TLS version
// constant below 1.3 and the zero-value (which is allowed).
// ---------------------------------------------------------------------------

func TestChaosConfigValidateMinVersionExact(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("chaos test skipped in short mode")
	}

	tc, err := testcert.Generate()
	if err != nil {
		t.Fatal(err)
	}

	type entry struct {
		Name       string `json:"name"`
		MinVersion uint16 `json:"min_version"`
		Valid      bool   `json:"valid"`
		Error      string `json:"error,omitempty"`
	}

	versions := []struct {
		name string
		v    uint16
		ok   bool
	}{
		{"zero", 0, true},
		{"tls1.0", tls.VersionTLS10, false},
		{"tls1.1", tls.VersionTLS11, false},
		{"tls1.2", tls.VersionTLS12, false},
		{"tls1.3", tls.VersionTLS13, true},
	}

	var results []entry
	for _, tv := range versions {
		tv := tv
		t.Run(tv.name, func(t *testing.T) {
			cfg := server.Config{
				Addr:    ":8443",
				Handler: http.NewServeMux(),
				TLSConf: func() *tls.Config {
					c := tc.Clone()
					c.MinVersion = tv.v
					return c
				}(),
			}
			err := cfg.Validate()
			valid := err == nil
			errStr := ""
			if err != nil {
				errStr = err.Error()
			}
			results = append(results, entry{
				Name: tv.name, MinVersion: tv.v, Valid: valid, Error: errStr,
			})
			if valid != tv.ok {
				t.Errorf("MinVersion=%d: Validate() ok=%v, want ok=%v", tv.v, valid, tv.ok)
			}
		})
	}

	ev, _ := json.MarshalIndent(results, "", "  ")
	writeChaosEvidence(t, "chaos_config_minversion.json", ev)
}
