package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"digital.vasic.http3/internal/testcert"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func portFromConfig(cfg Config) string {
	// Since we use "127.0.0.1:0" the port is resolved at bind time.
	// For tests that don't actually bind we just return the configured addr.
	return cfg.Addr
}

func portToAddr(port int) string {
	return "127.0.0.1:" + strconv.Itoa(port)
}

func writeChaosEvidence(t *testing.T, name, detail string) {
	t.Helper()
	dir := os.Getenv("HELIX_STRESS_EVIDENCE_DIR")
	if dir == "" {
		dir = "qa-results/stress_chaos"
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Logf("warning: mkdir %s: %v", dir, err)
		return
	}
	body := fmt.Sprintf(`=== Chaos Test Report ===
Test:       %s
Timestamp:  %s
Detail:     %s
`,
		name,
		time.Now().UTC().Format(time.RFC3339),
		detail,
	)
	p := filepath.Join(dir, fmt.Sprintf("%s_%d.txt", sanitiseTestName(name), time.Now().UnixNano()))
	if err := os.WriteFile(p, []byte(body), 0644); err != nil {
		t.Logf("warning: write evidence %s: %v", p, err)
	}
}

// ---------------------------------------------------------------------------
// TestChaosConfigNilFields
// ---------------------------------------------------------------------------

// TestChaosConfigNilFields tests Config with nil Handler, nil TLSConf, empty
// Addr, TLSConf without Certificates, and TLSConf with MinVersion < TLS 1.3.
// Each variant is validated independently to confirm the expected error.
func TestChaosConfigNilFields(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("chaos test skipped in short mode")
	}

	// Obtain a valid TLS config for the "missing cert" variant.
	tc, err := testcert.Generate()
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{
			name:    "nil_handler",
			cfg:     Config{Addr: ":8443", Handler: nil, TLSConf: tc},
			wantErr: "Handler is required",
		},
		{
			name:    "nil_tlsconf",
			cfg:     Config{Addr: ":8443", Handler: http.NewServeMux(), TLSConf: nil},
			wantErr: "TLSConf is required",
		},
		{
			name:    "empty_addr",
			cfg:     Config{Addr: "", Handler: http.NewServeMux(), TLSConf: tc},
			wantErr: "Addr is required",
		},
		{
			name: "tlsconf_no_certificates",
			cfg: Config{
				Addr:    ":8443",
				Handler: http.NewServeMux(),
				TLSConf: &tls.Config{MinVersion: tls.VersionTLS13},
			},
			wantErr: "must provide Certificates or GetCertificate",
		},
		{
			name: "tlsconf_minversion_too_low",
			cfg: Config{
				Addr:    ":8443",
				Handler: http.NewServeMux(),
				TLSConf: func() *tls.Config {
					tc2, _ := testcert.Generate()
					tc2.MinVersion = tls.VersionTLS12
					return tc2
				}(),
			},
			wantErr: "MinVersion must be tls.VersionTLS13",
		},
	}

	var panicked int
	var correct int
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Validate panicked on %s: %v", tc.name, r)
					panicked++
				}
			}()
			err := tc.cfg.Validate()
			if err == nil {
				t.Errorf("expected error for %s, got nil", tc.name)
				return
			}
			if !errorContains(err.Error(), tc.wantErr) {
				t.Errorf("expected error containing %q, got %q", tc.wantErr, err.Error())
				return
			}
			correct++
		})
	}

	writeChaosEvidence(t, t.Name(),
		fmt.Sprintf("cases=%d correct=%d panicked=%d", len(cases), correct, panicked))
}

// ---------------------------------------------------------------------------
// TestChaosNewInvalidConfig
// ---------------------------------------------------------------------------

// TestChaosNewInvalidConfig calls New() with all possible invalid config
// variants and verifies error wrapping.
func TestChaosNewInvalidConfig(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("chaos test skipped in short mode")
	}

	tc, err := testcert.Generate()
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{
			name: "empty_addr",
			cfg: Config{
				Addr:    "",
				Handler: http.NewServeMux(),
				TLSConf: tc,
			},
			wantErr: "Addr is required",
		},
		{
			name: "nil_handler",
			cfg: Config{
				Addr:    ":8443",
				Handler: nil,
				TLSConf: tc,
			},
			wantErr: "Handler is required",
		},
		{
			name: "nil_tlsconf",
			cfg: Config{
				Addr:    ":8443",
				Handler: http.NewServeMux(),
				TLSConf: nil,
			},
			wantErr: "TLSConf is required",
		},
		{
			name: "multiple_invalid",
			cfg: Config{
				Addr:    "",
				Handler: nil,
				TLSConf: nil,
			},
			// Validate returns on the FIRST error; with Addr:"", Handler:nil,
			// TLSConf:nil, the first check is Addr.
			wantErr: "Addr is required",
		},
	}

	var correct int
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("New panicked: %v", r)
				}
			}()
			_, err := New(tc.cfg)
			if err == nil {
				t.Errorf("New() with %s returned nil error", tc.name)
				return
			}
			if !errorContains(err.Error(), tc.wantErr) {
				t.Errorf("expected error containing %q, got %q", tc.wantErr, err.Error())
				return
			}
			correct++
		})
	}
	writeChaosEvidence(t, t.Name(), fmt.Sprintf("variants=%d correct=%d", len(cases), correct))
}

// ---------------------------------------------------------------------------
// TestChaosStartTwice
// ---------------------------------------------------------------------------

// TestChaosStartTwice starts a server, then attempts to start it again,
// verifying ErrAlreadyStarted is returned.
func TestChaosStartTwice(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("chaos test skipped in short mode")
	}

	cfg := buildConfig(t)
	srv, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	// Start in background.
	startErrCh := make(chan error, 1)
	go func() {
		startErrCh <- srv.Start()
	}()

	// Give the server a moment to bind.
	time.Sleep(50 * time.Millisecond)

	// Second Start must return ErrAlreadyStarted.
	err2 := srv.Start()
	if !errors.Is(err2, ErrAlreadyStarted) {
		t.Errorf("expected ErrAlreadyStarted, got %v", err2)
	}

	// Cleanup.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown: %v", err)
	}

	writeChaosEvidence(t, t.Name(), "second Start returned ErrAlreadyStarted, server then shut down cleanly")
}

// ---------------------------------------------------------------------------
// TestChaosShutdownBeforeStart
// ---------------------------------------------------------------------------

// TestChaosShutdownBeforeStart calls Shutdown with a cancelled context and
// Shutdown on a never-started server.  Both are idempotent and must not
// panic.
func TestChaosShutdownBeforeStart(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("chaos test skipped in short mode")
	}

	cfg := buildConfig(t)
	srv, err := New(cfg)
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

	// Shutdown on never-started server (no context, should also be safe).
	if err := srv.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown on never-started server returned error: %v", err)
	}

	writeChaosEvidence(t, t.Name(), "Shutdown before Start completed without panic or error")
}

// ---------------------------------------------------------------------------
// TestChaosConcurrentShutdown
// ---------------------------------------------------------------------------

// TestChaosConcurrentShutdown spawns N=10 goroutines that all call Shutdown
// with the same context.  The Shutdown must be idempotent (no panic).
func TestChaosConcurrentShutdown(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("chaos test skipped in short mode")
	}

	cfg := buildConfig(t)
	srv, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	// Start in background.
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

	if len(errs) > 0 {
		t.Errorf("%d of %d concurrent Shutdown calls returned errors or panics: %v", len(errs), workers, errs)
	}

	writeChaosEvidence(t, t.Name(),
		fmt.Sprintf("concurrent Shutdown workers=%d errors=%d", workers, len(errs)))
}

// ---------------------------------------------------------------------------
// TestChaosDoubleStart
// ---------------------------------------------------------------------------

// TestChaosDoubleStart simulates a race: two goroutines trying to Start()
// the same server.  Only one should succeed; the other returns
// ErrAlreadyStarted.
//
// We do NOT use a WaitGroup here because the successful Start() call blocks
// in ListenAndServe() until Shutdown closes the listener.  Instead we use a
// buffered result channel and collect results after Shutdown.
func TestChaosDoubleStart(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("chaos test skipped in short mode")
	}

	cfg := buildConfig(t)
	srv, err := New(cfg)
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

	// Wait long enough for both goroutines to have called srv.Start().
	time.Sleep(100 * time.Millisecond)

	// Now shut down so the successful Start() goroutine unblocks.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)

	// Collect results.
	var (
		errs     []error
		successN int
	)
	for i := 0; i < racers; i++ {
		select {
		case e := <-results:
			errs = append(errs, e)
			if e == nil {
				successN++
			}
		case <-time.After(time.Second):
			errs = append(errs, fmt.Errorf("goroutine %d did not return within 1 s", i))
		}
	}

	if successN != 1 {
		t.Errorf("expected exactly 1 successful Start, got %d; errors: %v", successN, errs)
	}

	writeChaosEvidence(t, t.Name(),
		fmt.Sprintf("double_start racers=%d success=%d errors=%d", racers, successN, len(errs)-successN))
}

// ---------------------------------------------------------------------------
// TestChaosDoneChannel
// ---------------------------------------------------------------------------

// TestChaosDoneChannel verifies Done() returns the correct error on shutdown
// vs never-started.  A never-started server's Done channel is open but empty;
// a started-and-shut-down server's Done channel receives nil (or the server
// error).
func TestChaosDoneChannel(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("chaos test skipped in short mode")
	}

	// --- Sub-test: never-started server has an empty Done channel. ---
	t.Run("never_started_done_empty", func(t *testing.T) {
		cfg := buildConfig(t)
		srv, err := New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		select {
		case err := <-srv.Done():
			t.Errorf("never-started server Done() returned: %v", err)
		default:
			// Expected: channel exists but has no value.
		}
	})

	// --- Sub-test: started-and-shut-down server gets a value on Done. ---
	t.Run("started_shutdown_done_receives", func(t *testing.T) {
		cfg := buildConfig(t)
		srv, err := New(cfg)
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
			// After a clean Shutdown, Done should receive nil or
			// http.ErrServerClosed converted to nil (see Start()).
			if doneErr != nil {
				t.Errorf("after Shutdown, Done() returned error: %v", doneErr)
			}
		case <-time.After(time.Second):
			t.Fatal("Done() did not receive a value within 1 s after Shutdown")
		}
	})

	writeChaosEvidence(t, t.Name(), "Done channel behaviour verified for never-started and started-shutdown cases")
}

// ---------------------------------------------------------------------------
// errorContains is a simple substring check.
// ---------------------------------------------------------------------------
func errorContains(s, substr string) bool {
	if len(substr) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
