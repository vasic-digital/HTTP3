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

