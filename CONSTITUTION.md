# digital.vasic.http3 — Constitution

This module is part of the `vasic-digital` ecosystem and inherits its
non-negotiable rules. Operators consuming this module MUST be aware of
the discipline encoded here before pinning a hash.

## Inherited rules

The following rules are **constitutional law** in this module by the same
authority as in their original repos. Modifying them in this module is
forbidden; submodule constitutions may add stricter rules but MUST NOT
relax these.

- **Sixth Law — Real User Verification (Anti-Pseudo-Test Rule).**
  Every test MUST traverse the production code path the user's action
  triggers; MUST be provably falsifiable (author breaks the feature,
  observes failure, documents it before merge); MUST primary-assert on
  user-visible state, not on mock interaction counts; Challenge Tests are
  the load-bearing release gate. The full text of the rule lives in the
  parent project's `/CLAUDE.md` and applies recursively to this module.

- **Local-Only CI/CD.** This module does NOT use, and MUST NOT add,
  GitHub Actions, GitLab pipelines, CircleCI, or any other hosted CI
  service. The single local entry point is `scripts/ci.sh`. Forbidden
  files: `.github/workflows/*`, `.gitlab-ci.yml`, `.circleci/*`,
  `azure-pipelines.yml`, `bitbucket-pipelines.yml`, `Jenkinsfile`.

- **Decoupled Reusable Architecture.** This module is an artifact of that
  rule. It exists precisely so consumers don't reinvent HTTP/3 wiring per
  project. Anything we add here MUST remain product-agnostic — no
  Lava-specific, no HelixAgent-specific, no consumer-specific assumptions.

## Module-specific rules

- **Surface stability.** `Config` and `Server` are the public surface.
  Adding a field to `Config` is a backwards-compatible change provided
  the zero value preserves prior behavior. Removing or renaming a field
  is a major-version event.

- **No re-export of `quic-go` types in our public API.** Consumers who
  need quic-go-level tuning import quic-go directly and pass a
  `*quic.Config` into `Config.QUICConfig`. We do not wrap, alias, or
  shadow quic-go's types — that would couple our API surface to theirs
  in a way that creates churn whenever quic-go evolves.

- **TLS 1.3 minimum is enforced in `New`, not just documented.** HTTP/3
  is TLS-1.3-only on the wire; failing late at handshake time is a worse
  developer experience than failing at construction. `New` MUST upgrade
  `TLSConf.MinVersion` to `tls.VersionTLS13` if unset, and MUST reject a
  caller-supplied `MinVersion < tls.VersionTLS13`.

- **No logging in this module.** Logging is the consumer's concern;
  injecting a logger here would either tie us to a specific logging
  library or force us to invent yet another logger interface. The
  consumer's HTTP middleware is the right place.

## Verification before tagging

Before any release tag is cut on this module:

1. `scripts/ci.sh` MUST be run on the exact commit being tagged and pass.
2. The Challenge Test (`pkg/server/challenge_test.go::TestCrossBackendParity`)
   MUST be falsified at least once — the author MUST have temporarily
   mutated the H3 server's response, observed the test fail, and reverted
   the mutation. The PR description MUST record this verification.
3. The `go vet`, `go test -race`, fuzz (≥30 s), `gosec`, and `govulncheck`
   gates MUST all be clean.

## Mirror policy

Per the parent project's Decoupled Reusable Architecture rule, this
module is mirrored to:

- `git@github.com:vasic-digital/HTTP3.git` (primary)
- `git@gitlab.com:vasic-digital/HTTP3.git` (mirror)

Pushes go to both. GitFlic and GitVerse are not currently in this
module's mirror set because programmatic CLI control over those hosts
is not yet wired for `vasic-digital`-org repos.
