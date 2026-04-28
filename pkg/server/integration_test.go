package server_test

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"digital.vasic.http3/internal/testcert"
	"digital.vasic.http3/pkg/server"
	"github.com/quic-go/quic-go/http3"
)

// freeUDPPort grabs a free UDP port by binding then releasing.
//
// HTTP/3 uses UDP so the port allocation must come from the UDP space.
func freeUDPPort(t *testing.T) int {
	t.Helper()
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := conn.LocalAddr().(*net.UDPAddr).Port
	conn.Close()
	return port
}

// startServer boots an http3 server in a goroutine and returns it
// along with its listen address. Cleanup is registered via t.Cleanup.
func startServer(t *testing.T, h http.Handler) (*server.Server, string) {
	t.Helper()
	port := freeUDPPort(t)
	addr := "127.0.0.1:" + strconv.Itoa(port)

	tc, err := testcert.Generate()
	if err != nil {
		t.Fatalf("testcert: %v", err)
	}
	srv, err := server.New(server.Config{
		Addr:    addr,
		Handler: h,
		TLSConf: tc,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	go func() {
		_ = srv.Start()
	}()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})
	// Wait for the listener to bind.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		c, err := net.Dial("udp", addr)
		if err == nil {
			c.Close()
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	return srv, addr
}

// h3Client builds a client speaking HTTP/3 against a self-signed test cert.
func h3Client() *http.Client {
	rt := &http3.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
			NextProtos:         []string{"h3"},
		},
	}
	return &http.Client{Transport: rt, Timeout: 5 * time.Second}
}

// TestRoundTripHelloWorld exercises the full path:
//
//	HTTP/3 client → quic-go transport → server.Server → user handler → response.
//
// Sixth Law falsifiability: deliberately changing the handler's response body
// causes this test to fail with a clear assertion message.
func TestRoundTripHelloWorld(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Test", "yes")
		w.WriteHeader(http.StatusTeapot)
		fmt.Fprint(w, "hello over http3")
	})

	_, addr := startServer(t, mux)
	cli := h3Client()
	defer cli.CloseIdleConnections()

	resp, err := cli.Get("https://" + addr + "/hello")
	if err != nil {
		t.Fatalf("client.Get: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	// Sixth Law primary assertion is on user-visible state.
	if resp.StatusCode != http.StatusTeapot {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusTeapot)
	}
	if got := resp.Header.Get("X-Test"); got != "yes" {
		t.Errorf("X-Test = %q, want %q", got, "yes")
	}
	if string(body) != "hello over http3" {
		t.Errorf("body = %q, want %q", string(body), "hello over http3")
	}

	// Confirm the protocol the client actually negotiated.
	if !strings.HasPrefix(resp.Proto, "HTTP/3") {
		t.Errorf("Proto = %q, want HTTP/3.x", resp.Proto)
	}
}

// TestServerShutdownClosesInFlight verifies Shutdown cleanly stops
// the listener so subsequent dials fail.
func TestServerShutdownClosesListener(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "ok") })

	srv, addr := startServer(t, mux)

	// First request succeeds.
	cli := h3Client()
	resp, err := cli.Get("https://" + addr + "/")
	if err != nil {
		t.Fatalf("first Get: %v", err)
	}
	resp.Body.Close()

	// Shutdown.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	// Second request must fail (server no longer responding).
	cli2 := h3Client()
	defer cli2.CloseIdleConnections()
	ctx2, cancel2 := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancel2()
	req, _ := http.NewRequestWithContext(ctx2, "GET", "https://"+addr+"/", nil)
	resp, err = cli2.Do(req)
	if err == nil {
		resp.Body.Close()
		t.Fatalf("expected request to fail after Shutdown, got status %d", resp.StatusCode)
	}
	// Any non-nil error is acceptable — context-deadline, connection closed,
	// QUIC handshake-timeout, etc.
	if errors.Is(err, http.ErrServerClosed) {
		t.Fatalf("unexpected ErrServerClosed on client side: %v", err)
	}
}

// TestServerLargeBodyRoundTrip exercises a payload bigger than a single
// MTU so the handler/codec path actually streams.
func TestServerLargeBodyRoundTrip(t *testing.T) {
	t.Parallel()
	const payloadSize = 64 * 1024 // 64 KiB
	payload := make([]byte, payloadSize)
	for i := range payload {
		payload[i] = byte(i % 251)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/big", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(payload)
	})

	_, addr := startServer(t, mux)
	cli := h3Client()
	defer cli.CloseIdleConnections()

	resp, err := cli.Get("https://" + addr + "/big")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if len(body) != payloadSize {
		t.Fatalf("body size = %d, want %d", len(body), payloadSize)
	}
	for i := 0; i < payloadSize; i++ {
		if body[i] != byte(i%251) {
			t.Fatalf("byte %d = %d, want %d (payload corrupted)", i, body[i], byte(i%251))
		}
	}
}
