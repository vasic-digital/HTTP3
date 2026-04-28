# AGENTS.md — digital.vasic.http3

A focused agent guide for `digital.vasic.http3`. For a richer overview of
the surrounding ecosystem, see the parent project's `AGENTS.md`.

## What this module is

`digital.vasic.http3` is a thin HTTP/3 server wrapper. It accepts any
`net/http.Handler` and exposes it over QUIC via `quic-go/http3`. Nothing
else.

## Tech stack

| Layer | Choice |
|-------|--------|
| Language | Go 1.24+ (required transitively by quic-go) |
| QUIC / HTTP/3 | `github.com/quic-go/quic-go` v0.59.x |
| TLS | Go stdlib `crypto/tls` (TLS 1.3 only) |
| Tests | Go stdlib `testing` (unit + integration + Challenge + fuzz) |
| Static analysis | `go vet`, `gosec`, `govulncheck` |
| CI | **Local only**, via `scripts/ci.sh` |

## Local CI gate

The single source of truth for "is this commit shippable":

```bash
scripts/ci.sh
```

It runs, in order:
1. `go mod tidy` and verify clean diff
2. `go vet ./...`
3. `go build ./...`
4. `go test -race -count=1 ./...`
5. `go test -fuzz=. -fuzztime=30s ./pkg/server` (FuzzConfigValidate)
6. `gosec` (if installed)
7. `govulncheck ./...` (if installed)

A green `scripts/ci.sh` is necessary but not sufficient for a release tag —
the Sixth Law also requires a documented falsifiability run of the Challenge
Test (see `CONSTITUTION.md`).

## Workflow

This module follows the Direct-to-main workflow per parent-project policy:

1. Branch off `main`.
2. Make changes.
3. Run `scripts/ci.sh` until green.
4. For any change touching the H3 dispatch or response writing, run the
   Challenge Test falsifiability procedure (see `CLAUDE.md`).
5. Commit. Push to `main` on both `github` and `gitlab` remotes.

## Public API surface

| Symbol | Stability | Notes |
|--------|-----------|-------|
| `server.Config` | Stable. New fields may be added; existing fields' zero values must preserve prior behavior. | |
| `server.Config.Validate()` | Stable. Adding new validation rules is a minor-version event. | |
| `server.New(Config)` | Stable. | |
| `server.Server.Start()` | Stable. Blocks until shutdown. | |
| `server.Server.Shutdown(ctx)` | Stable. Idempotent. | |
| `server.Server.Done() <-chan error` | Stable. | |
| `server.Server.Addr() string` | Stable. | |
| `server.ErrAlreadyStarted` | Stable. | |

## Things to avoid

- **Hosted CI.** Forbidden by `CONSTITUTION.md`.
- **Re-exporting quic-go types.** Consumers import quic-go directly for
  any quic-go-level configuration; we do not wrap.
- **Logging or metrics in this module.** Consumer's middleware concern.
- **Lava- or other-consumer-specific code.** This is a generic primitive.

---

## Host Machine Stability Directive (Critical Constraint)

**IT IS FORBIDDEN to directly or indirectly cause the host machine to:**
- Suspend, hibernate, or enter standby mode
- Sign out the currently logged-in user
- Terminate the user session or running processes
- Trigger any power-management event that interrupts active work

Long-running tests (e.g. fuzz with extended `-fuzztime`, load tests if added
later) MUST be run inside `screen`/`tmux` or with explicit timeouts. Never
run a fuzz job that could exhaust system memory and OOM-kill the desktop
session; bound `-fuzztime` and use `-parallel=N` deliberately.
