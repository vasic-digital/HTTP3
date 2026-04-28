# CLAUDE.md — digital.vasic.http3

This file guides Claude Code and other agents working in this repository.

## Module purpose

This module wraps `quic-go/http3` so any `net/http.Handler` can serve HTTP/3.
That is the *entire* job. Logging, middleware, observability, rate-limiting,
and security headers are explicitly NOT this module's concern — those live
in their respective `vasic-digital` modules and compose at the handler level.

## Inherited rules (non-negotiable)

- **Sixth Law (Real User Verification).** Tests MUST traverse production
  paths and MUST be provably falsifiable. See `CONSTITUTION.md`.
- **Local-Only CI/CD.** No hosted CI configuration in this repo.
  `scripts/ci.sh` is the single entry point. See `CONSTITUTION.md`.
- **Decoupled Reusable Architecture.** No consumer-specific code.

## Layout

```
.
├── pkg/server/                     # public API (Config, Server)
│   ├── server.go
│   ├── server_test.go              # unit tests (package server)
│   ├── integration_test.go         # real HTTP/3 client roundtrip (package server_test)
│   ├── challenge_test.go           # Sixth Law: H2 vs H3 byte-equivalent (package server_test)
│   └── fuzz_test.go                # Config.Validate fuzz
├── internal/testcert/              # self-signed cert generator (tests only)
├── scripts/ci.sh                   # local CI gate
├── CONSTITUTION.md                 # constitutional rules (read first)
├── README.md                       # consumer-facing overview
├── CLAUDE.md                       # this file
└── AGENTS.md                       # agent guide (mirror of constitution + workflow)
```

## Things to avoid

- Re-exporting `quic-go` types from our API surface.
- Adding logging hooks. The handler is the right place; this module is dumb pipes.
- Adding a `New()` overload that takes individual fields instead of `Config`.
  The struct shape forces forward-compatible additions and labelled args.
- Adding hosted CI config of any flavour. Use `scripts/ci.sh`.
- Importing anything Lava-specific or HelixAgent-specific. If a feature can't
  be motivated as "any Go HTTP service might want this", it doesn't belong here.

## When changing tests

The Sixth Law makes Challenge Tests load-bearing. If you modify
`pkg/server/challenge_test.go::TestCrossBackendParity`, you MUST:

1. Confirm the unmodified test passes.
2. Deliberately mutate the production code (e.g. force every H3 response body
   to "MUTATED").
3. Re-run the test and confirm it fails with a clear diff.
4. Revert the mutation.
5. Record what you mutated and what failure was observed in the PR description.

Step 2–4 MUST be performed for every PR that touches the H3 server's request
dispatch or response writing. Skipping it is a Sixth Law violation.

## Tag/release

Releases tag the module as `vX.Y.Z`. Before tagging:

- `scripts/ci.sh` MUST pass on the commit being tagged.
- The most recent Challenge Test falsifiability run MUST be documented in
  the previous PR's description.

## Useful notes

- HTTP/3 is UDP. Tests that bind a port use `freeUDPPort` in
  `integration_test.go`, NOT `net.Listen("tcp", ...)`.
- The integration tests use `InsecureSkipVerify: true` on the client because
  the test server runs with a self-signed cert from `internal/testcert`.
  This is acceptable in tests only — never in production.
- The unit tests for lifecycle (`TestStartTwiceReturnsErrAlreadyStarted`,
  `TestShutdownIsIdempotent`) live in `package server` (internal) so they can
  poke `srv.mu` directly. The integration and challenge tests live in
  `package server_test` (external) and use only the public API.
