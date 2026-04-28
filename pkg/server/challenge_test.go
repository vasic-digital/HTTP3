package server_test

// Challenge test for digital.vasic.http3.
//
// The user-visible promise of this module is: "any net/http.Handler will
// produce the same response over HTTP/3 as it would over HTTP/2." If the
// implementation silently mangles headers, mis-routes paths, or drops body
// bytes, this test must fail.
//
// Sixth Law clause 4: a green Challenge Test means the wire actually works,
// not "the wiring compiles". This test starts BOTH a real HTTP/2 server
// (httptest.NewTLSServer) AND a real HTTP/3 server (server.New) bound to
// the same http.Handler, then runs the same set of exchanges against both
// and demands byte-equivalent response bodies, identical status codes, and
// identical user-visible headers.

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/quic-go/quic-go/http3"
)

func challengeHandler() http.Handler {
	mux := http.NewServeMux()

	// Plain text echo of method, path, and a query value.
	mux.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("X-Echo", "1")
		fmt.Fprintf(w, "method=%s path=%s q=%s", r.Method, r.URL.Path, r.URL.Query().Get("q"))
	})

	// JSON-ish response with deterministic body.
	mux.HandleFunc("/json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Json", "1")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintf(w, `{"hello":"world","n":%d}`, 42)
	})

	// Status-only with empty body.
	mux.HandleFunc("/no-content", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// Reads request body and echoes its length.
	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(w, "received=%d", len(body))
	})

	return mux
}

type response struct {
	status  int
	body    string
	headers map[string]string // user-visible only
}

func snapshot(resp *http.Response) (response, error) {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return response{}, err
	}
	// Capture only handler-set headers — protocol-specific ones differ.
	pickKeys := []string{"Content-Type", "X-Echo", "X-Json"}
	hdrs := make(map[string]string, len(pickKeys))
	for _, k := range pickKeys {
		if v := resp.Header.Get(k); v != "" {
			hdrs[k] = v
		}
	}
	return response{
		status:  resp.StatusCode,
		body:    string(body),
		headers: hdrs,
	}, nil
}

func diffHeaders(a, b map[string]string) string {
	keys := map[string]struct{}{}
	for k := range a {
		keys[k] = struct{}{}
	}
	for k := range b {
		keys[k] = struct{}{}
	}
	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	var diffs []string
	for _, k := range sorted {
		av := a[k]
		bv := b[k]
		if av != bv {
			diffs = append(diffs, fmt.Sprintf("%s: h2=%q h3=%q", k, av, bv))
		}
	}
	if len(diffs) == 0 {
		return ""
	}
	return strings.Join(diffs, "\n  ")
}

// TestCrossBackendParity is the Sixth-Law-mandated Challenge Test.
//
// To verify falsifiability before merging, an author MUST temporarily change
// the H3 server's handler dispatch (e.g. force every response body to be
// "MUTATED") and observe this test fail with a clear diff. After confirming
// the breakage is detected, revert the mutation. This procedure MUST be
// recorded in the PR description per Sixth Law clause 2.
func TestCrossBackendParity(t *testing.T) {
	t.Parallel()

	handler := challengeHandler()

	// HTTP/2 backend.
	h2srv := httptest.NewUnstartedServer(handler)
	h2srv.TLS = &tls.Config{NextProtos: []string{"h2"}}
	h2srv.StartTLS()
	t.Cleanup(h2srv.Close)
	h2cli := h2srv.Client()
	h2cli.Timeout = 5 * time.Second

	// HTTP/3 backend.
	_, h3addr := startServer(t, handler)
	h3cli := h3Client()
	defer h3cli.CloseIdleConnections()

	cases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"echo-get", "GET", "/echo?q=lava", ""},
		{"json-get", "GET", "/json", ""},
		{"no-content-get", "GET", "/no-content", ""},
		{"upload-post-tiny", "POST", "/upload", "abc"},
		{"upload-post-empty", "POST", "/upload", ""},
		{"upload-post-medium", "POST", "/upload", strings.Repeat("x", 4096)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h2URL := h2srv.URL + tc.path
			h3URL := "https://" + h3addr + tc.path

			h2resp, err := doRequest(h2cli, tc.method, h2URL, tc.body)
			if err != nil {
				t.Fatalf("h2 request: %v", err)
			}
			h3resp, err := doRequest(h3cli, tc.method, h3URL, tc.body)
			if err != nil {
				t.Fatalf("h3 request: %v", err)
			}
			h2snap, err := snapshot(h2resp)
			if err != nil {
				t.Fatalf("h2 snapshot: %v", err)
			}
			h3snap, err := snapshot(h3resp)
			if err != nil {
				t.Fatalf("h3 snapshot: %v", err)
			}

			if h2snap.status != h3snap.status {
				t.Errorf("status mismatch: h2=%d h3=%d", h2snap.status, h3snap.status)
			}
			if h2snap.body != h3snap.body {
				t.Errorf("body mismatch:\n  h2=%q\n  h3=%q", h2snap.body, h3snap.body)
			}
			if d := diffHeaders(h2snap.headers, h3snap.headers); d != "" {
				t.Errorf("header mismatch:\n  %s", d)
			}
		})
	}
}

func doRequest(cli *http.Client, method, url, body string) (*http.Response, error) {
	var br io.Reader
	if body != "" {
		br = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, br)
	if err != nil {
		return nil, err
	}
	return cli.Do(req)
}

// silence unused import warning if http3 package is referenced only here.
var _ = http3.NextProtoH3
