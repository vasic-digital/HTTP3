// Package server provides an HTTP/3 server that wraps quic-go/http3 and
// accepts any net/http.Handler.
//
// The package's job is intentionally narrow: validate a Config, expose a
// Start/Shutdown lifecycle, and let the handler do everything else.
// Consumers who need quic-go-specific tuning may pass a *quic.Config in
// Config.QUICConfig; the server forwards it to the underlying http3.Server
// without reinterpretation.
package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
)

// Config configures a Server.
//
// Required:
//
//   - Addr     — UDP listen address, e.g. ":8443" or "0.0.0.0:443".
//   - Handler  — any net/http.Handler (gin, mux, custom, …).
//   - TLSConf  — fully populated *tls.Config; HTTP/3 requires TLS 1.3,
//     so TLSConf.MinVersion MUST be tls.VersionTLS13 if set. A
//     certificate source is required: Certificates, GetCertificate,
//     or GetConfigForClient (e.g. for per-SNI selection or
//     ACME/autocert-style rotation — quic-go's http3.Server honors
//     GetConfigForClient the same way net/http does).
//
// Optional:
//
//   - QUICConfig — quic-go-level knobs (handshake idle timeout, keep-alive,
//     stream limits, congestion control). Nil = quic-go default.
//     Per-request read/write timeouts are handler-level concerns
//     in HTTP/3 — wrap your handler with http.TimeoutHandler or
//     use a context-aware middleware. The QUIC layer's idle and
//     keep-alive timers live on quic.Config, not on this struct.
type Config struct {
	Addr       string
	Handler    http.Handler
	TLSConf    *tls.Config
	QUICConfig *quic.Config
}

// Validate returns an error if the Config is unfit to serve HTTP/3.
// It must be safe to call before Start. Validate is also called inside New.
func (c Config) Validate() error {
	if c.Addr == "" {
		return errors.New("http3: Config.Addr is required")
	}
	if c.Handler == nil {
		return errors.New("http3: Config.Handler is required")
	}
	if c.TLSConf == nil {
		return errors.New("http3: Config.TLSConf is required (HTTP/3 mandates TLS 1.3)")
	}
	if len(c.TLSConf.Certificates) == 0 && c.TLSConf.GetCertificate == nil && c.TLSConf.GetConfigForClient == nil {
		return errors.New("http3: Config.TLSConf must provide Certificates, GetCertificate, or GetConfigForClient")
	}
	if c.TLSConf.MinVersion != 0 && c.TLSConf.MinVersion < tls.VersionTLS13 {
		return errors.New("http3: Config.TLSConf.MinVersion must be tls.VersionTLS13 or unset")
	}
	return nil
}

// Server is a running HTTP/3 server.
//
// One Server instance manages a single UDP listener and a single
// http3.Server. Calling Start more than once returns ErrAlreadyStarted.
// Shutdown is idempotent.
type Server struct {
	cfg      Config
	mu       sync.Mutex
	h3       *http3.Server
	started  bool
	shutdown bool
	doneCh   chan error

	// closeOnce guards the single h3.Close() invocation; closeDone is
	// closed once that invocation returns, and closeErr holds its result.
	// Every Shutdown caller — first or Nth, concurrent or sequential —
	// waits on closeDone (bounded by its own ctx) instead of returning as
	// soon as it observes `shutdown == true`, so a concurrent caller can
	// never observe a false "done" while the real close is still in
	// flight (see TestShutdownConcurrentCallersWaitForRealCompletion).
	closeOnce sync.Once
	closeDone chan struct{}
	closeErr  error
}

// ErrAlreadyStarted is returned when Start is called more than once on
// the same Server.
var ErrAlreadyStarted = errors.New("http3: server already started")

// New constructs a Server from a validated Config. New does NOT bind the
// listener; call Start (or ListenAndServe) to do that.
func New(cfg Config) (*Server, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	// Force TLS 1.3 minimum even if caller left MinVersion zero — HTTP/3
	// is TLS-1.3-only on the wire and we want to fail loudly here rather
	// than at handshake time.
	tlsConf := cfg.TLSConf.Clone()
	tlsConf.MinVersion = tls.VersionTLS13
	if !nextProtoIncludesH3(tlsConf.NextProtos) {
		tlsConf.NextProtos = append([]string{http3.NextProtoH3}, tlsConf.NextProtos...)
	}
	cfg.TLSConf = tlsConf
	return &Server{cfg: cfg, doneCh: make(chan error, 1), closeDone: make(chan struct{})}, nil
}

// Start binds the UDP listener and serves in the current goroutine. It
// returns when the server is shut down or an unrecoverable error occurs.
//
// Start may be called only once. If the caller wants a non-blocking
// start, run Start in a goroutine and use Shutdown to stop.
func (s *Server) Start() error {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return ErrAlreadyStarted
	}
	if s.shutdown {
		s.mu.Unlock()
		return errors.New("http3: server already shut down")
	}
	s.started = true

	s.h3 = &http3.Server{
		Addr:            s.cfg.Addr,
		Handler:         s.cfg.Handler,
		TLSConfig:       s.cfg.TLSConf,
		QUICConfig:      s.cfg.QUICConfig,
		EnableDatagrams: false,
	}
	s.mu.Unlock()

	err := s.h3.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		err = nil
	}
	s.doneCh <- err
	return err
}

// Shutdown closes the underlying http3.Server. It is idempotent and safe
// to call from any goroutine — concurrently or repeatedly. Exactly one
// underlying h3.Close() runs (guarded by closeOnce); every caller, first
// or Nth, waits for that SAME close to actually finish and observes its
// SAME result, each bounded independently by its own ctx. A caller never
// receives a premature nil merely because another caller already marked
// the server as shutting down — that would let a concurrent goroutine
// falsely believe the server had stopped while a close was still in
// flight (draining an active connection, for example).
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	s.shutdown = true
	h3 := s.h3
	s.mu.Unlock()

	if h3 == nil {
		// Start was never called; nothing to close.
		return nil
	}

	s.closeOnce.Do(func() {
		go func() {
			s.closeErr = h3.Close()
			close(s.closeDone)
		}()
	})

	select {
	case <-s.closeDone:
		return s.closeErr
	case <-ctx.Done():
		return fmt.Errorf("http3: shutdown context deadline exceeded: %w", ctx.Err())
	}
}

// Done returns a channel that receives the server's terminal error
// (or nil for clean shutdown) when Start returns. The channel is
// buffered with capacity 1.
func (s *Server) Done() <-chan error { return s.doneCh }

// Addr returns the configured listen address. After Start, this is
// the same string the listener was bound to.
func (s *Server) Addr() string { return s.cfg.Addr }

func nextProtoIncludesH3(protos []string) bool {
	for _, p := range protos {
		if p == http3.NextProtoH3 {
			return true
		}
	}
	return false
}
