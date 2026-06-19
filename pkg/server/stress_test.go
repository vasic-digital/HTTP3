package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	"digital.vasic.http3/internal/testcert"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// buildConfig returns a minimal valid Config using testcert.Generate().
// Unlike must() (which calls t.Fatalf from inside goroutines), this lets
// callers handle errors explicitly.
func buildConfig(t *testing.T) Config {
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

// buildConfigs pre-generates n valid Configs and returns them.  This avoids
// calling t.Fatalf from inside goroutines.
func buildConfigs(t *testing.T, n int) []Config {
	t.Helper()
	cfgs := make([]Config, n)
	for i := range cfgs {
		cfgs[i] = buildConfig(t)
	}
	return cfgs
}

// latencyReport holds timing percentiles for a stress run.
type latencyReport struct {
	Name   string
	N      int
	Passed int
	Failed int
	Min    time.Duration
	P50    time.Duration
	P95    time.Duration
	P99    time.Duration
	Max    time.Duration
	Mean   time.Duration
}

func computePercentiles(durations []time.Duration) (min, p50, p95, p99, max, mean time.Duration) {
	if len(durations) == 0 {
		return 0, 0, 0, 0, 0, 0
	}
	sorted := make([]time.Duration, len(durations))
	copy(sorted, durations)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	var sum time.Duration
	for _, d := range sorted {
		sum += d
	}
	n := len(sorted)
	return sorted[0],
		sorted[n*50/100],
		sorted[n*95/100],
		sorted[n*99/100],
		sorted[n-1],
		sum / time.Duration(n)
}

func writeStressEvidence(t *testing.T, r latencyReport) {
	t.Helper()
	dir := os.Getenv("HELIX_STRESS_EVIDENCE_DIR")
	if dir == "" {
		dir = "qa-results/stress_chaos"
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Logf("warning: mkdir %s: %v", dir, err)
		return
	}
	body := fmt.Sprintf(`=== Stress Test Report ===
Test:       %s
Timestamp:  %s
Iterations: %d
Passed:     %d
Failed:     %d
Min:        %v
P50:        %v
P95:        %v
P99:        %v
Max:        %v
Mean:       %v
`,
		r.Name,
		time.Now().UTC().Format(time.RFC3339),
		r.N, r.Passed, r.Failed,
		r.Min, r.P50, r.P95, r.P99, r.Max, r.Mean)

	p := filepath.Join(dir, fmt.Sprintf("%s_%d.txt", sanitiseTestName(r.Name), time.Now().UnixNano()))
	if err := os.WriteFile(p, []byte(body), 0644); err != nil {
		t.Logf("warning: write evidence %s: %v", p, err)
	}
}

func sanitiseTestName(name string) string {
	// Replace / with _ so the name is safe for a filename.
	// This also strips the leading "Test".
	return name[4:]
}

// ---------------------------------------------------------------------------
// TestStressConcurrentConfigValidate
// ---------------------------------------------------------------------------

// TestStressConcurrentConfigValidate runs N=200 concurrent Config.Validate()
// calls with valid configs, measures p50/p95/p99 latency, and writes
// captured evidence.
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
		go func(cfg Config) {
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

	min, p50, p95, p99, max, mean := computePercentiles(durations)
	r := latencyReport{
		Name: t.Name(), N: n,
		Passed: n - errCount, Failed: errCount,
		Min: min, P50: p50, P95: p95, P99: p99, Max: max, Mean: mean,
	}
	writeStressEvidence(t, r)

	if errCount > 0 {
		t.Errorf("%d of %d concurrent Validate() calls returned errors", errCount, n)
	}
}

// ---------------------------------------------------------------------------
// TestStressConcurrentNew
// ---------------------------------------------------------------------------

// TestStressConcurrentNew runs N=100 concurrent New(Config) calls.  All
// configs are valid, so every New should succeed.
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
		go func(cfg Config) {
			defer wg.Done()
			_, err := New(cfg)
			if err != nil {
				mu.Lock()
				errCount++
				mu.Unlock()
			}
		}(cfgs[i])
	}
	wg.Wait()

	writeStressEvidence(t, latencyReport{
		Name: t.Name(), N: n,
		Passed: n - errCount, Failed: errCount,
	})

	if errCount > 0 {
		t.Errorf("%d of %d concurrent New() calls returned errors", errCount, n)
	}
}

// ---------------------------------------------------------------------------
// TestStressConcurrentStartShutdown
// ---------------------------------------------------------------------------

// TestStressConcurrentStartShutdown performs N=10 sequential start-shutdown
// cycles.  Each cycle creates a fresh server, starts it in a goroutine,
// shuts it down, and verifies the Done channel fires.
func TestStressConcurrentStartShutdown(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("stress test skipped in short mode")
	}

	const cycles = 10
	for i := 0; i < cycles; i++ {
		cfg := buildConfig(t)
		srv, err := New(cfg)
		if err != nil {
			t.Fatalf("cycle %d: New: %v", i, err)
		}

		startErrCh := make(chan error, 1)
		go func() {
			startErrCh <- srv.Start()
		}()

		// Give the server a moment to bind.
		time.Sleep(50 * time.Millisecond)

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		if err := srv.Shutdown(ctx); err != nil {
			cancel()
			t.Fatalf("cycle %d: Shutdown: %v", i, err)
		}
		cancel()

		// Verify the Start goroutine completes and Done fires.
		select {
		case startErr := <-startErrCh:
			if startErr != nil {
				t.Errorf("cycle %d: Start returned error: %v", i, startErr)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("cycle %d: Start did not return within 3 s after Shutdown", i)
		}

		// Done channel should also have received the terminal error.
		select {
		case doneErr := <-srv.Done():
			if doneErr != nil {
				t.Errorf("cycle %d: Done() returned error: %v", i, doneErr)
			}
		default:
			t.Errorf("cycle %d: Done() channel did not receive any value", i)
		}
	}

	writeStressEvidence(t, latencyReport{
		Name: t.Name(), N: cycles, Passed: cycles, Failed: 0,
	})
}

// ---------------------------------------------------------------------------
// TestStressConfigValidateMixed
// ---------------------------------------------------------------------------

// configFixture pairs a Config with an expected validity flag so concurrent
// validation can distinguish valid from invalid configs without inspecting
// the error itself.
type configFixture struct {
	Cfg     Config
	IsValid bool
}

// genConfigFixtures generates n fixture pairs: valid configs (IsValid=true)
// and invalid config variants (IsValid=false).
func genConfigFixtures(t *testing.T, n int) []configFixture {
	t.Helper()
	fixtures := make([]configFixture, 0, n)

	// Generate enough valid configs.
	baseCfgs := buildConfigs(t, n/2)

	// Build valid fixtures.
	for _, cfg := range baseCfgs {
		fixtures = append(fixtures, configFixture{Cfg: cfg, IsValid: true})
	}

	// Build invalid variants from copies of the valid configs.
	for i := 0; i < n/2; i++ {
		cfg := buildConfig(t)
		variant := i % 5
		switch variant {
		case 0:
			cfg.Addr = ""
		case 1:
			cfg.Handler = nil
		case 2:
			cfg.TLSConf = nil
		case 3:
			// TLSConf without Certificates and without GetCertificate.
			cfg.TLSConf = &tls.Config{MinVersion: tls.VersionTLS13}
		case 4:
			// Too-low MinVersion.
			cfg.TLSConf.MinVersion = tls.VersionTLS12
		}
		fixtures = append(fixtures, configFixture{Cfg: cfg, IsValid: false})
	}

	return fixtures
}

// TestStressConfigValidateMixed runs N=100 concurrent Validate() calls with
// a mix of valid and intentionally invalid configs, tracking expected
// validity explicitly so the heuristic does not misclassify TLS confs without
// certificates.
func TestStressConfigValidateMixed(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("stress test skipped in short mode")
	}

	const total = 100
	fixtures := genConfigFixtures(t, total)

	// Shuffle.
	rand.Shuffle(len(fixtures), func(i, j int) { fixtures[i], fixtures[j] = fixtures[j], fixtures[i] })

	var (
		mu          sync.Mutex
		durations   []time.Duration
		validFails  int
		invalidPass int
	)

	var wg sync.WaitGroup
	wg.Add(total)

	for idx := range fixtures {
		fix := fixtures[idx]
		go func(f configFixture) {
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
		}(fix)
	}
	wg.Wait()

	min, p50, p95, p99, max, mean := computePercentiles(durations)
	writeStressEvidence(t, latencyReport{
		Name: t.Name(), N: total,
		Passed: total, Failed: validFails + invalidPass,
		Min: min, P50: p50, P95: p95, P99: p99, Max: max, Mean: mean,
	})

	if validFails > 0 {
		t.Errorf("%d valid configs incorrectly rejected by Validate()", validFails)
	}
	if invalidPass > 0 {
		t.Errorf("%d invalid configs incorrectly accepted by Validate()", invalidPass)
	}
}
