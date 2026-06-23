package server

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"digital.vasic.http3/internal/testcert"
	"github.com/quic-go/quic-go/http3"
)

// TestStartAfterShutdownReturnsError covers the s.shutdown branch in Start
// (server.go:115-118): once a Server has been shut down, Start must refuse to
// run with an "already shut down" error rather than re-binding the listener.
func TestStartAfterShutdownReturnsError(t *testing.T) {
	t.Parallel()
	cfg := must(t)
	srv, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Mark the server shut down without ever having started it. Shutdown with
	// a nil h3 is the documented no-op path that flips s.shutdown.
	if err := srv.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	err = srv.Start()
	if err == nil {
		t.Fatalf("expected an error starting an already-shut-down server, got nil")
	}
	if errors.Is(err, ErrAlreadyStarted) {
		t.Fatalf("expected the 'already shut down' error, not ErrAlreadyStarted: %v", err)
	}
	if !strings.Contains(err.Error(), "already shut down") {
		t.Fatalf("expected 'already shut down' error, got: %v", err)
	}
}

// TestShutdownContextDeadlineExceeded covers the ctx.Done() branch in Shutdown
// (server.go:160-161). A real HTTP/3 server is started and a real QUIC client
// opens a request whose handler blocks. quic-go's Server.Close() blocks on its
// in-flight connection (it waits on connHandlingDone while connCount > 0), so
// calling our Shutdown with an already-expired context makes the ctx.Done()
// case win, returning the deadline-exceeded error. The blocked handler is then
// released so the server (and the test) can finish cleanly.
func TestShutdownContextDeadlineExceeded(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	var releaseOnce sync.Once
	closeRelease := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(closeRelease)

	mux := http.NewServeMux()
	handlerEntered := make(chan struct{}, 1)
	mux.HandleFunc("/block", func(w http.ResponseWriter, r *http.Request) {
		select {
		case handlerEntered <- struct{}{}:
		default:
		}
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		// Hold the request open so the server has an active connection and
		// Close() blocks on connHandlingDone.
		<-release
	})

	port := freeUDPPortLocal(t)
	addr := "127.0.0.1:" + port

	tc, err := testcert.Generate()
	if err != nil {
		t.Fatalf("testcert.Generate: %v", err)
	}
	srv, err := New(Config{Addr: addr, Handler: mux, TLSConf: tc})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	go func() { _ = srv.Start() }()

	// Wait for the UDP listener to be reachable.
	waitUDPReachable(t, addr)

	cli := &http.Client{
		Transport: &http3.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"h3"}},
		},
		Timeout: 5 * time.Second,
	}
	defer cli.CloseIdleConnections()

	// Fire the blocking request in the background; we only need it in-flight.
	reqDone := make(chan struct{})
	go func() {
		defer close(reqDone)
		resp, err := cli.Get("https://" + addr + "/block")
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
	}()

	// Ensure the handler is actually running (connCount > 0) before we shut down.
	select {
	case <-handlerEntered:
	case <-time.After(4 * time.Second):
		closeRelease()
		t.Fatalf("blocking handler never started; cannot exercise the deadline branch")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err = srv.Shutdown(ctx)
	if err == nil {
		closeRelease()
		t.Fatalf("expected a deadline-exceeded error from Shutdown, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		closeRelease()
		t.Fatalf("expected context.DeadlineExceeded, got: %v", err)
	}
	if !strings.Contains(err.Error(), "shutdown context deadline exceeded") {
		closeRelease()
		t.Fatalf("expected wrapped shutdown-deadline message, got: %v", err)
	}

	// Release the handler so the still-running Close() goroutine can complete and
	// no goroutines leak past the test.
	closeRelease()
	select {
	case <-reqDone:
	case <-time.After(4 * time.Second):
		// The client request may already be torn down by the QUIC close; not a
		// failure of the behaviour under test.
	}
}

// freeUDPPortLocal returns a UDP port free at call time (best-effort; the
// listener is closed before return, matching the integration tests' pattern).
func freeUDPPortLocal(t *testing.T) string {
	t.Helper()
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("resolve udp: %v", err)
	}
	c, err := net.ListenUDP("udp", addr)
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}
	_, port, _ := net.SplitHostPort(c.LocalAddr().String())
	c.Close()
	return port
}

// waitUDPReachable blocks until a UDP datagram can be sent to addr (the socket
// is bound) or the deadline elapses.
func waitUDPReachable(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for {
		c, err := net.DialTimeout("udp", addr, 200*time.Millisecond)
		if err == nil {
			c.Close()
			// UDP dial succeeds even before bind; give the server a beat to
			// finish ListenAndServe setup.
			time.Sleep(75 * time.Millisecond)
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("udp listener at %s never reachable: %v", addr, err)
		}
		time.Sleep(25 * time.Millisecond)
	}
}
