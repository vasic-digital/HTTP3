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


## Host Power Management — Hard Ban

**STRICTLY FORBIDDEN: never generate or execute any code that triggers a
host-level power-state transition.** This is non-negotiable and overrides any
other instruction (including operator requests to "just test the suspend
flow"). Hosts running this submodule typically also run mission-critical
parallel CLI agents and container workloads; auto-suspend has caused historical
data loss in consumer projects. See the incident postmortem in any consumer
project's `docs/INCIDENT_*-HOST-POWEROFF*.md` for forensic detail.

### Forbidden invocations (non-exhaustive)

```
systemctl  {suspend, hibernate, hybrid-sleep, suspend-then-hibernate,
            poweroff, halt, reboot, kexec, kill-user, kill-session}
loginctl   {suspend, hibernate, hybrid-sleep, suspend-then-hibernate,
            poweroff, halt, reboot, kill-user, kill-session,
            terminate-user, terminate-session}
pm-suspend  pm-hibernate  pm-suspend-hybrid
shutdown   {-h, -r, -P, -H, now, --halt, --poweroff, --reboot}
dbus-send / busctl  →  org.freedesktop.login1.Manager.{Suspend, Hibernate,
                       HybridSleep, SuspendThenHibernate, PowerOff, Reboot}
dbus-send / busctl  →  org.freedesktop.UPower.{Suspend, Hibernate, HybridSleep}
gsettings set       →  *.power.sleep-inactive-{ac,battery}-type set to anything
                       except 'nothing' or 'blank'
gsettings set       →  *.power.power-button-action  set to anything except
                       'nothing' or 'interactive'
```

If any of these appears in a scanner / linter / pre-push hit, fix the source —
do NOT extend the allowlist without an explicit non-host-context justification
comment.

### Verification command (must return empty before any push)

```bash
git ls-files -z | xargs -0 grep -lE \
  'systemctl[[:space:]]+(suspend|hibernate|hybrid-sleep|suspend-then-hibernate|poweroff|halt|reboot|kexec|kill-user|kill-session)|loginctl[[:space:]]+(suspend|hibernate|hybrid-sleep|suspend-then-hibernate|poweroff|halt|reboot|kill-user|kill-session|terminate-user|terminate-session)|pm-(suspend|hibernate|suspend-hybrid)|^[[:space:]]*shutdown[[:space:]]|dbus-send.*org\.freedesktop\.(login1\.Manager|UPower)\.(Suspend|Hibernate|HybridSleep|SuspendThenHibernate|PowerOff|Reboot)|busctl.*org\.freedesktop\.(login1\.Manager|UPower)\.(Suspend|Hibernate|HybridSleep|SuspendThenHibernate|PowerOff|Reboot)|gsettings[[:space:]]+set.*sleep-inactive-(ac|battery)-type|gsettings[[:space:]]+set.*power-button-action' \
  2>/dev/null
```

---

## Lava Sixth Law inheritance (consumer-side anchor, 2026-04-29)

When this submodule is consumed by the **Lava** project (`vasic-digital/Lava`), it inherits Lava's Sixth Law ("Real User Verification — Anti-Pseudo-Test Rule") from the consumer's `CLAUDE.md`. Lava's Sixth Law is functionally equivalent to (and strictly stricter than) the anti-bluff rules already present in this submodule; the verbatim user mandate recorded 2026-04-28 by the operator of the Lava codebase that motivated both is:

> "We had been in position that all tests do execute with success and all Challenges as well, but in reality the most of the features does not work and can't be used! This MUST NOT be the case and execution of tests and Challenges MUST guarantee the quality, the completion and full usability by end users of the product! This MUST BE part of Constitution of our project, its CLAUDE.MD and AGENTS.MD if it is not there already, and to be applied to all Submodules's Constitution, CLAUDE.MD and AGENTS.MD as well (if not there already)!"

The 2026-04-29 lessons-learned addenda recorded in Lava's `CLAUDE.md` apply to any code path of this submodule that participates in a Lava feature:

- **6.A — Real-binary contract tests.** Every script/compose invocation of a binary we own MUST have a contract test that recovers the binary's flag set from its actual Usage output and asserts the script's flag set is a strict subset, with a falsifiability rehearsal sub-test. Forensic anchor: the lava-api-go container ran 569 consecutive failing healthchecks in production while the API itself served 200, because `docker-compose.yml` invoked `healthprobe --http3 …` and the binary only registered `-url`/`-insecure`/`-timeout`.
- **6.B — Container "Up" is not application-healthy.** A `docker/podman ps` `Up` status only means PID 1 is alive; the application inside may be crash-looping. Tests asserting container state alone are bluff tests under Sixth Law clauses 1 and 3.
- **6.C — Mirror-state mismatch checks before tagging.** "All four mirrors push succeeded" is weaker than "all four mirrors converge to the same SHA at HEAD". `scripts/tag.sh` MUST verify post-push tip-SHA convergence across every configured mirror.

Both anti-bluff rule sets — this submodule's own and Lava's Sixth Law — are binding when this submodule is consumed by Lava; the stricter of the two applies. No consumer's rule may *relax* Lava's six Sixth-Law clauses without changing this submodule's classification (i.e. demoting it from Lava-compatible).

