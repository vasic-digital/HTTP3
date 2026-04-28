# digital.vasic.http3

A generic, reusable Go module that wraps `quic-go/http3` so any
`net/http.Handler` can serve HTTP/3 with a few lines of code.

## Why this module exists

Go's standard library does not serve HTTP/3 / QUIC. `quic-go/http3` does, but
its surface is opinionated and tightly coupled to the QUIC stack. This module
narrows the API down to a single `Config` + `Server` pair so applications that
already use `net/http` (Gin, chi, gorilla/mux, plain `http.ServeMux`) can opt
into HTTP/3 without restructuring their code.

The server is intentionally narrow: it validates a `Config`, exposes a
`Start` / `Shutdown` lifecycle, and forwards everything else to the underlying
`http3.Server`. There is no middleware, no logging, no metrics — those belong
in their own modules and compose at the `http.Handler` level.

## Installation

```bash
go get digital.vasic.http3
```

## Quick start

```go
package main

import (
    "context"
    "crypto/tls"
    "log"
    "net/http"
    "os/signal"
    "syscall"
    "time"

    "digital.vasic.http3/pkg/server"
)

func main() {
    cert, err := tls.LoadX509KeyPair("server.crt", "server.key")
    if err != nil {
        log.Fatal(err)
    }
    tlsConf := &tls.Config{
        Certificates: []tls.Certificate{cert},
        MinVersion:   tls.VersionTLS13,
    }

    mux := http.NewServeMux()
    mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        w.Write([]byte("hello over HTTP/3"))
    })

    srv, err := server.New(server.Config{
        Addr:    ":8443",
        Handler: mux,
        TLSConf: tlsConf,
    })
    if err != nil {
        log.Fatal(err)
    }

    ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
    defer stop()

    go func() {
        if err := srv.Start(); err != nil {
            log.Fatal(err)
        }
    }()
    <-ctx.Done()
    shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    _ = srv.Shutdown(shutCtx)
}
```

## What's in the box

| Package | Responsibility |
|---------|----------------|
| `pkg/server` | `Config`, `Server`, `Start`, `Shutdown`, `Done`, `Addr`. |
| `internal/testcert` | Generates self-signed certs for tests only. Not exported. |

## Constitutional discipline

This module inherits the `vasic-digital` constitutional rules — see
`CONSTITUTION.md`:

- **Sixth Law (Real User Verification).** Every test in this module is
  provably falsifiable. The Challenge Test in `challenge_test.go` exercises
  the same handler over real HTTP/2 and real HTTP/3 transports and demands
  byte-equivalent response bodies, status codes, and user-visible headers.
- **Local-Only CI/CD.** No hosted CI configuration ships in this repo.
  `scripts/ci.sh` is the single local entry point that runs every gate
  (vet, build, test with race detector, fuzz, gosec, govulncheck).
- **Decoupled Reusable Architecture.** This module does not depend on any
  Lava-specific code. It depends only on `quic-go/quic-go` and the Go
  standard library. Any Lava-side glue lives in the consuming repo, not here.

## Testing

```bash
scripts/ci.sh                     # full local CI gate
go test ./...                     # tests only
go test -race ./...               # tests with race detector
go test -fuzz=. -fuzztime=30s ./pkg/server   # fuzz validators
```

## License

MIT — see `LICENSE`.
