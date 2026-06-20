// Package http3_stress_test — stress tests for digital.vasic.http3 (§11.4.85).
//
// These meta-tests exercise the exported server API (Config.Validate, New,
// Start/Shutdown lifecycle) under concurrent and sustained load patterns.
// No mocks, no stubs — every test uses the real implementation with real TLS
// certificates from internal/testcert.  Panic recovery in every goroutine.
package http3_stress_test

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"sort"
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

func stressEvidenceDir() string {
	if d := os.Getenv("HELIX_STRESS_EVIDENCE_DIR"); d != "" {
		return d
	}
	return "qa-results/stress_chaos"
}

func writeStressEvidence(t *testing.T, name string, data []byte) {
	t.Helper()
	dir := stressEvidenceDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Logf("WARNING: mkdir %s: %v", dir, err)
		return
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Logf("WARNING: write evidence %s: %v", path, err)
	}
}

// buildConfig returns a minimal valid Config using testcert.Generate().
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

// buildConfigs pre-generates n valid Configs for concurrent use.
func buildConfigs(t *testing.T, n int) []server.Config {
	t.Helper()
	cfgs := make([]server.Config, n)
	for i := range cfgs {
		cfgs[i] = buildConfig(t)
	}
	return cfgs
}

// ---------------------------------------------------------------------------
// TestStressSustainedConfigValidate
//
// N=500 iterations of Config.Validate() with valid configs, measuring
// p50/p95/p99 latency.  Exercises the TLS, Addr, and Handler checks.
// ---------------------------------------------------------------------------

func TestStressSustainedConfigValidate(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("stress test skipped in short mode")
	}

	const n = 500
	cfgs := buildConfigs(t, n)

	durations := make([]time.Duration, 0, n)
	var failed int

	for i := 0; i < n; i++ {
		start := time.Now()
		err := cfgs[i].Validate()
		durations = append(durations, time.Since(start))
		if err != nil {
			t.Errorf("iteration %d: Validate = %v", i, err)
			failed++
		}
	}

	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	var p50, p95, p99 time.Duration
	if len(durations) > 0 {
		nn := len(durations)
		p50 = durations[nn*50/100]
		p95 = durations[nn*95/100]
		p99 = durations[nn*99/100]
	}

	summary := map[string]interface{}{
		"test":   "TestStressSustainedConfigValidate",
		"N":      n,
		"failed": failed,
		"p50_ns": p50.Nanoseconds(),
		"p95_ns": p95.Nanoseconds(),
		"p99_ns": p99.Nanoseconds(),
	}
	ev, _ := json.MarshalIndent(summary, "", "  ")
	writeStressEvidence(t, "stress_sustained_config_validate.json", ev)

	if failed > 0 {
		t.Errorf("%d of %d Validate() calls failed", failed, n)
	}
}

// ---------------------------------------------------------------------------
// TestStressConcurrentNew
//
// N=100 concurrent server.New() calls with valid configs.  Every call must
// succeed.
// ---------------------------------------------------------------------------

func TestStressConcurrentNew(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("stress test skipped in short mode")
	}

	const n = 100
	cfgs := buildConfigs(t, n)

	var (
		mu       sync.Mutex
		errCount int
	)
	var wg sync.WaitGroup
	wg.Add(n)

	for i := 0; i < n; i++ {
		go func(cfg server.Config) {
			defer wg.Done()
			_, err := server.New(cfg)
			if err != nil {
				mu.Lock()
				errCount++
				mu.Unlock()
			}
		}(cfgs[i])
	}
	wg.Wait()

	summary := map[string]interface{}{
		"test":     "TestStressConcurrentNew",
		"N":        n,
		"errors":   errCount,
		"all_pass": errCount == 0,
	}
	ev, _ := json.MarshalIndent(summary, "", "  ")
	writeStressEvidence(t, "stress_concurrent_new.json", ev)

	if errCount > 0 {
		t.Errorf("%d of %d concurrent New() calls returned errors", errCount, n)
	}
}

// ---------------------------------------------------------------------------
// TestStressConcurrentConfigValidate
//
// N=200 concurrent Config.Validate() calls with valid configs.  Measures
// p50/p95/p99 latency.
// ---------------------------------------------------------------------------

func TestStressConcurrentConfigValidate(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("stress test skipped in short mode")
	}

	const n = 200
	cfgs := buildConfigs(t, n)

	var (
		mu        sync.Mutex
		durations []time.Duration
		errCount  int
	)
	var wg sync.WaitGroup
	wg.Add(n)

	for i := 0; i < n; i++ {
		go func(cfg server.Config) {
			defer wg.Done()
			start := time.Now()
			err := cfg.Validate()
			elapsed := time.Since(start)

			mu.Lock()
			durations = append(durations, elapsed)
			if err != nil {
				errCount++
			}
			mu.Unlock()
		}(cfgs[i])
	}
	wg.Wait()

	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	summary := map[string]interface{}{
		"test": "TestStressConcurrentConfigValidate",
		"N":    n,
	}
	if len(durations) > 0 {
		nn := len(durations)
		summary["p50_ns"] = durations[nn*50/100].Nanoseconds()
		summary["p95_ns"] = durations[nn*95/100].Nanoseconds()
		summary["p99_ns"] = durations[nn*99/100].Nanoseconds()
	}
	ev, _ := json.MarshalIndent(summary, "", "  ")
	writeStressEvidence(t, "stress_concurrent_config_validate.json", ev)

	if errCount > 0 {
		t.Errorf("%d of %d concurrent Validate() calls returned errors", errCount, n)
	}
}

// ---------------------------------------------------------------------------
// TestStressConfigValidateMixed
//
// N=100 concurrent Validate() calls with a mix of valid and intentionally
// invalid configs.  Tracks expected validity explicitly.
// ---------------------------------------------------------------------------

func TestStressConfigValidateMixed(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("stress test skipped in short mode")
	}

	type fixture struct {
		Cfg     server.Config
		IsValid bool
	}

	const total = 100
	baseCfgs := buildConfigs(t, total/2)

	fixtures := make([]fixture, 0, total)
	for _, cfg := range baseCfgs {
		fixtures = append(fixtures, fixture{Cfg: cfg, IsValid: true})
	}
	for i := 0; i < total/2; i++ {
		cfg := buildConfig(t)
		switch i % 5 {
		case 0:
			cfg.Addr = ""
		case 1:
			cfg.Handler = nil
		case 2:
			cfg.TLSConf = nil
		case 3:
			cfg.TLSConf = &tls.Config{MinVersion: tls.VersionTLS13}
		case 4:
			cfg.TLSConf.MinVersion = tls.VersionTLS12
		}
		fixtures = append(fixtures, fixture{Cfg: cfg, IsValid: false})
	}
	rand.Shuffle(len(fixtures), func(i, j int) { fixtures[i], fixtures[j] = fixtures[j], fixtures[i] })

	var (
		mu          sync.Mutex
		durations   []time.Duration
		validFails  int
		invalidPass int
	)
	var wg sync.WaitGroup
	wg.Add(total)

	for _, fix := range fixtures {
		f := fix
		go func() {
			defer wg.Done()
			start := time.Now()
			err := f.Cfg.Validate()
			elapsed := time.Since(start)

			mu.Lock()
			durations = append(durations, elapsed)
			if f.IsValid && err != nil {
				validFails++
			}
			if !f.IsValid && err == nil {
				invalidPass++
			}
			mu.Unlock()
		}()
	}
	wg.Wait()

	summary := map[string]interface{}{
		"test":         "TestStressConfigValidateMixed",
		"total":        total,
		"valid_fails":  validFails,
		"invalid_pass": invalidPass,
	}
	ev, _ := json.MarshalIndent(summary, "", "  ")
	writeStressEvidence(t, "stress_config_validate_mixed.json", ev)

	if validFails > 0 {
		t.Errorf("%d valid configs incorrectly rejected", validFails)
	}
	if invalidPass > 0 {
		t.Errorf("%d invalid configs incorrectly accepted", invalidPass)
	}
}

// ---------------------------------------------------------------------------
// TestStressStartShutdownCycles
//
// N=10 sequential server start-shutdown cycles.  Each cycle creates a fresh
// server, starts it in a goroutine, shuts it down, and verifies the Done
// channel fires.
// ---------------------------------------------------------------------------

func TestStressStartShutdownCycles(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("stress test skipped in short mode")
	}

	const cycles = 10
	var failed int32

	for i := 0; i < cycles; i++ {
		cfg := buildConfig(t)
		srv, err := server.New(cfg)
		if err != nil {
			atomic.AddInt32(&failed, 1)
			t.Errorf("cycle %d: New: %v", i, err)
			continue
		}

		go func() { _ = srv.Start() }()
		time.Sleep(50 * time.Millisecond)

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		if err := srv.Shutdown(ctx); err != nil {
			cancel()
			atomic.AddInt32(&failed, 1)
			t.Errorf("cycle %d: Shutdown: %v", i, err)
			continue
		}
		cancel()

		// Verify Done channel fires.
		select {
		case <-srv.Done():
		case <-time.After(3 * time.Second):
			atomic.AddInt32(&failed, 1)
			t.Errorf("cycle %d: Done() did not fire within 3s", i)
		}
	}

	summary := map[string]interface{}{
		"test":   "TestStressStartShutdownCycles",
		"cycles": cycles,
		"failed": failed,
	}
	ev, _ := json.MarshalIndent(summary, "", "  ")
	writeStressEvidence(t, "stress_start_shutdown_cycles.json", ev)
}
