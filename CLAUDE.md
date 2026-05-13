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


## Lava Seventh Law inheritance (Anti-Bluff Enforcement, 2026-04-30)

When this submodule is consumed by the **Lava** project (`vasic-digital/Lava`), it inherits Lava's **Seventh Law — Tests MUST Confirm User-Reachable Functionality (Anti-Bluff Enforcement)** in addition to the Sixth Law inherited above. The Seventh Law was added to Lava's `CLAUDE.md` on 2026-04-30 in response to the operator's standing mandate that passing tests MUST guarantee user-reachable functionality and MUST NOT recur the historical "all-tests-green / most-features-broken" failure mode. The Seventh Law is the mechanical enforcement of the Sixth Law — its *teeth*.

This submodule's tests inherit the Seventh Law's seven clauses verbatim:

1. **Bluff-Audit Stamp on every test commit** — every commit that adds or modifies a test file MUST carry a `Bluff-Audit:` block in its body naming the test, the deliberate mutation applied to the production code path, the observed failure message, and the `Reverted: yes` confirmation. Pre-push hooks reject test commits that lack the stamp.
2. **Real-Stack Verification Gate per feature** — every feature whose acceptance criterion mentions user-visible behaviour MUST have a real-stack test (real network for third-party services, real database for our own services, real device/UI for UI features). Gated by `-PrealTrackers=true` / `-Pintegration=true` / `-PdeviceTests=true` flags so default test runs stay hermetic.
3. **Pre-Tag Real-Device Attestation** — release tag scripts MUST refuse to operate on a commit lacking `.lava-ci-evidence/<tag>/real-device-attestation.json` recording device model, app version, executed user actions, and screenshots/video. There is no exception.
4. **Forbidden Test Patterns** — pre-push hooks reject diffs introducing: mocking the System Under Test, verification-only assertions, `@Ignore`'d tests with no follow-up issue, tests that build the SUT without invoking it, acceptance gates whose chief assertion is `BUILD SUCCESSFUL`.
5. **Recurring Bluff Hunt** — once per development phase, 5 random `*Test.kt` / `*_test.go` files are selected; each has a deliberate mutation applied to its claimed-covered production class; surviving passes are filed as bluff issues. Output recorded under `.lava-ci-evidence/bluff-hunt/<date>.json`.
6. **Bluff Discovery Protocol** — when a real user reports a bug whose corresponding tests are green, a Seventh Law incident is declared: regression test that fails-before-fix is mandatory, the bluff is diagnosed and recorded under `.lava-ci-evidence/sixth-law-incidents/<date>.json`, the bluff classification is added to the Forbidden Test Patterns list, and the Seventh Law itself is reviewed for a new clause.
7. **Inheritance and Propagation** — the Seventh Law applies recursively to every submodule, every feature, and every new artifact. Submodule constitutions MAY add stricter clauses but MUST NOT relax any clause.

The authoritative verbatim text lives in the parent Lava `CLAUDE.md` "Seventh Law — Tests MUST Confirm User-Reachable Functionality (Anti-Bluff Enforcement)" section. Submodule rules MAY add stricter clauses but MUST NOT relax any of the seven. Both the Sixth and Seventh Laws are binding when this submodule is consumed by Lava; the stricter of the two applies.

## Clauses 6.I and 6.J (added 2026-05-04, inherited per 6.F)

- **Clause 6.I — Multi-Emulator Container Matrix as Real-Device Equivalent** — see root `/CLAUDE.md` §6.I. Real-stack verification, where this submodule's work requires it (per 6.G clause 5 / Sixth Law clause 5 / Seventh Law clause 3), is satisfied ONLY by the project's container-bound multi-emulator matrix where the consuming Lava feature touches the UI; for pure-library code paths covered here, real-stack means real implementations of all dependencies (real database, real HTTP socket, real cache backend, real timer, real filesystem) at the boundary the library claims to cover — not mocks of those dependencies. A single passing emulator (or single happy-path test) is NOT the gate.
- **Clause 6.J — Anti-Bluff Functional Reality Mandate** — see root `/CLAUDE.md` §6.J. Every test, every Challenge Test, and every CI gate touched by this submodule MUST do exactly one job: confirm the feature it claims to cover actually works for an end user, end-to-end, on the gating matrix. CI green is necessary, never sufficient. Adding a test the author cannot execute against the gating matrix is itself a bluff. Tests must guarantee the product works — anything else is theatre.

## Clauses 6.K and 6.L (added 2026-05-04, inherited per 6.F)

- **Clause 6.K — Builds-Inside-Containers Mandate** — see root `/CLAUDE.md` §6.K. Every release-artifact build MUST run inside the project's container-bound build path (anchored on `vasic-digital/Containers`'s build orchestration: `cmd/distributed-build` + `pkg/distribution` + `pkg/runtime`), not on the developer's bare host. Local incremental dev builds on the host are permitted for iteration; the gate, the release-artifact build, and the build whose output goes through the emulator matrix (clause 6.I) MUST go through Containers. The accompanying 6.K-debt entry tracks the package additions (`pkg/emulator/`, `pkg/vm/`) that are owed.
- **Clause 6.L — Anti-Bluff Functional Reality Mandate (Operator's Standing Order)** — see root `/CLAUDE.md` §6.L. Every test, every Challenge Test, every CI gate has exactly one job: confirm the feature works for a real user end-to-end on the gating matrix. CI green is necessary, never sufficient. Tests must guarantee the product works — anything else is theatre. The operator has invoked this mandate TWENTY TIMES across two working days; the repetition itself is the forensic record. The 10th invocation (2026-05-05, immediately after Phase 7 readiness was reported, when the operator commissioned the full rebuild-and-test-everything cycle for tag Lava-Android-1.2.3): "Rebuild Go API and client app(s), put new builds into releases dir (with properly updated version codes) and execute all existing tests and Challenges!". If you find yourself rationalizing a "small exception" — STOP. There are no small exceptions. The Internet Archive stuck-on-loading bug, the broken post-login navigation, the credential leak in C2, the bluffed C1-C8 — these are what "small exceptions" produce.

## Clause 6.M (added 2026-05-04 evening, inherited per 6.F)

- **Clause 6.M — Host-Stability Forensic Discipline** — see root `/CLAUDE.md` §6.M. Every perceived-instability event during a session that touches this submodule MUST be classified into Class I (verifiable host event), Class II (resource pressure), or Class III (operator-perceived without forensic evidence) AND audited via the 7-step forensic protocol (uptime+who, journalctl logind events, kernel critical events, free -h, df -h, forbidden-command grep across tracked files, container state inventory). Findings recorded under `.lava-ci-evidence/sixth-law-incidents/<date>-<slug>.json`. **Container-runtime safety analysis (recorded once in root §6.M, referenced forever):** rootless Podman has NO host-level power-management privileges; rootful Docker is not installed on the operator's primary host. Container operations cannot cause Class I host events on the audited host configuration. A perceived-instability event without an audit record is itself a Seventh Law violation under clause 6.J ("tests must guarantee the product works" — applied recursively to incident response).

## Clause 6.N (added 2026-05-05, inherited per 6.F)

- **Clause 6.N — Bluff-Hunt Cadence Tightening + Production Code Coverage** — see root `/CLAUDE.md` §6.N. Beyond the Seventh Law clause 5 baseline (5 random `*Test.kt` files every 2-4 weeks), bluff hunts now fire IN-cycle on three triggers: (1) per operator anti-bluff-mandate invocation — first/day full 5+2, subsequent same-day lighter 1-2 file incident-response; (2) per matrix-runner/gate change (pre-push enforced via §6.N-debt — owed); (3) per phase-gating attestation file added (pre-push enforced via §6.N-debt — owed). Bluff hunts MUST also sample production code: 2 files per phase from gate-shaping code (canonical list in root §6.N.2: `scripts/tag.sh` helpers, `scripts/check-constitution.sh`, `Submodules/Containers/pkg/emulator/`, `Submodules/Containers/cmd/emulator-matrix/`, the matrix runner's `writeAttestation` function) plus 0-2 from broader CI-touched code. Conceptual filter: "would a bug here be invisible to existing tests?". Forensic anchor: 2026-05-05 ultrathink-driven discovery of the 7-day-old `pkg/emulator/Boot()` port-collision bluff that was invisible to all existing test-only bluff hunts. §6.N-debt tracks the pre-push hook implementation owed via the Group A-prime spec (next brainstorming target).

## Clause 6.O (added 2026-05-05, inherited per 6.F)

- **Clause 6.O — Crashlytics-Resolved Issue Coverage Mandate** — see root `/CLAUDE.md` §6.O. Every Crashlytics-recorded issue (fatal OR non-fatal) closed/resolved by any commit MUST gain (a) a validation test in the language of the crashing surface that reproduces the conditions, (b) a Challenge Test under `app/src/androidTest/kotlin/lava/app/challenges/` (client) or `tests/e2e/` (server) that drives the same user-facing path, and (c) a closure log at `.lava-ci-evidence/crashlytics-resolved/<date>-<slug>.md` recording the issue ID, root-cause analysis, fix commit SHA, and links to the tests. `scripts/tag.sh` MUST refuse release tags whose CHANGELOG mentions Crashlytics fixes without matching closure logs. Marking a Crashlytics issue "closed" in the Console requires the test coverage to land first — never close-mark before the regression-immunity tests exist. Forensic anchor: 2026-05-05, 2 Crashlytics-recorded crashes within minutes of the first Firebase-instrumented APK distribution (Lava-Android-1.2.3-1023, commit `e9de508`); post-mortem at `.lava-ci-evidence/crashlytics-resolved/2026-05-05-firebase-init-hardening.md`. The operator's ELEVENTH §6.L invocation made this clause load-bearing.

## Clause 6.P (added 2026-05-05, inherited per 6.F)

- **Clause 6.P — Distribution Versioning + Changelog Mandate** — see root `/CLAUDE.md` §6.P. Every distribute action (Firebase App Distribution, container registry pushes, releases/ snapshots, scripts/tag.sh) MUST: (1) carry a strictly increasing versionCode (no re-distribution of already-published codes); (2) include a CHANGELOG entry — canonical file `CHANGELOG.md` at repo root + per-version snapshot at `.lava-ci-evidence/distribute-changelog/<channel>/<version>-<code>.md`; (3) inject the changelog into the App Distribution release-notes via `--release-notes`. `scripts/firebase-distribute.sh` REFUSES to operate when current versionCode ≤ last-distributed versionCode for the channel, OR when CHANGELOG.md lacks an entry for the current version, OR when the per-version snapshot file is missing. `scripts/tag.sh` enforces the same gates pre-tag. Re-distributing the same versionCode is forbidden across distribute sessions; idempotent retry within a single session is permitted. Forensic anchor: 2026-05-05 23:11 operator's TWELFTH §6.L invocation: "when distributing new build it must have version code bigger by at least one then the last version code available for download (already distribited). Every distributed build MUST CONTAIN changelog with the details what it includes compared to previous one we have published!"

## Clause 6.Q (added 2026-05-05, inherited per 6.F)

- **Clause 6.Q — Compose Layout Antipattern Guard** — see root `/CLAUDE.md` §6.Q. Forbids nesting vertically-scrolling lazy layouts (LazyColumn, LazyVerticalGrid, LazyVerticalStaggeredGrid) inside parents giving unbounded vertical space (verticalScroll, unbounded wrapContentHeight, LinearLayout-with-weight wrapper). Equivalent rule horizontally for LazyRow / LazyHorizontalGrid / LazyHorizontalStaggeredGrid. Per-feature structural tests + Compose UI Challenge Tests on the §6.I matrix are the load-bearing acceptance gates. Forensic anchor: 2026-05-05 23:51 operator-reported "Opening Trackers from Settings crashes the app" — TrackerSelectorList used LazyColumn nested in TrackerSettingsScreen's Column(verticalScroll). Closure log: `.lava-ci-evidence/crashlytics-resolved/2026-05-05-tracker-settings-nested-scroll.md`. Pattern guard: `feature/tracker_settings/src/test/.../TrackerSelectorListLazyColumnRegressionTest.kt`. The operator THIRTEENTH §6.L invocation triggered this clause.


## §6.R — No-Hardcoding Mandate (inherited 2026-05-06, per §6.F)

See root `/CLAUDE.md` §6.R. No connection address, port, header field name, credential, key, salt, secret, schedule, algorithm parameter, or domain literal in tracked source code. Every such value MUST come from `.env` (gitignored), generated config, runtime env var, or mounted file. Submodule MAY add stricter rules but MUST NOT relax.

## §6.S — Continuation Document Maintenance Mandate (inherited 2026-05-06, per §6.F)

See root `/CLAUDE.md` §6.S. The file `docs/CONTINUATION.md` (in the parent Lava repo) is the single-file source-of-truth handoff document for resuming work across any CLI session. Every commit that changes phase status, lands a new spec/plan, bumps a submodule pin, ships a release artifact, discovers/resolves a known issue, or implements an operator scope directive MUST update `docs/CONTINUATION.md` in the SAME COMMIT. The §0 "Last updated" line MUST track HEAD. Submodule MAY add stricter rules (e.g., maintain its own CONTINUATION) but MUST NOT relax this clause.

## §6.T — Universal Quality Constraints (inherited 2026-05-06, per §6.F)

See root `/CLAUDE.md` §6.T. All four sub-points (Reproduction-Before-Fix, Resource Limits for Tests & Challenges, No-Force-Push, Bugfix Documentation) apply verbatim. This submodule MAY add stricter rules but MUST NOT relax any of §6.T.1–§6.T.4.

## §6.U — No sudo/su Mandate (inherited 2026-05-08, per §6.F)

See root `/CLAUDE.md` §6.U. Every use of `sudo` or `su` is strictly forbidden. Operations requiring elevated privileges MUST use container-based solutions from the `vasic-digital/Containers` submodule or be provided by local project/Submodule dependencies that build automatically. The pre-push hook rejects files containing `sudo ` or `su ` patterns. This submodule MAY add stricter rules but MUST NOT relax.

## §6.V — Container Emulators Mandate (inherited 2026-05-08, per §6.F)

See root `/CLAUDE.md` §6.V. Every Android emulator instance for Challenge Tests / UI verification MUST run inside a container managed by the `vasic-digital/Containers` submodule. Rootless Podman/Docker only. All tests execute inside containers. The §6.I matrix (API 28/30/34/latest, phone/tablet/TV) runs inside container-bound emulators. This submodule MAY add stricter rules but MUST NOT relax.

## §6.W — GitHub + GitLab Only Remotes (inherited 2026-05-08, per §6.F)

See root `/CLAUDE.md` §6.W. Only GitHub (`vasic-digital/*`, `HelixDevelopment/*`) and GitLab (`vasic-digital/*`, `HelixDevelopment/*`) are permitted as Git remotes. GitFlic, GitVerse, and all other providers are forbidden. The 4-mirror model is replaced by 2-mirror (GitHub + GitLab). This submodule MAY add stricter rules but MUST NOT relax.

## §6.X — Container-Submodule Emulator Wiring Mandate (inherited 2026-05-13, per §6.F)

See root `/CLAUDE.md` §6.X. Every Android emulator instance the project depends on for testing MUST execute its emulator process INSIDE a podman/docker container managed by `Submodules/Containers/`, NOT be host-direct-launched by Containers-submodule code that runs on the host. The Containers submodule's `pkg/runtime/` (rootless podman/docker auto-detection) brings the container up; `pkg/emulator/` orchestrates the AVD lifecycle inside it. Lava-side `scripts/run-emulator-tests.sh` is thin glue forwarding to the Containers CLI. The container-bound path is the gate — host-direct emulators are permitted for workstation iteration only. §6.X-debt tracks the wiring implementation owed to `Submodules/Containers/`. This submodule MAY add stricter rules but MUST NOT relax.

