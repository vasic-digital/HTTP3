# QWEN.md — Qwen Code context for this module

This file is read by Qwen Code as its module-context file. It is the Qwen Code
counterpart of CLAUDE.md and AGENTS.md for this module, and it is a pointer:
there is one canonical agent-instruction file per scope.

## Read CLAUDE.md — it is mandatory

This module's canonical agent-instruction file is CLAUDE.md in this directory.
Before doing any work in this module, open and read CLAUDE.md and this module's
CONSTITUTION.md in full. Every rule there binds Qwen Code exactly as it binds
Claude Code.

This file is a plain-text pointer and deliberately uses no auto-import
directive. Qwen Code's memory-import processor resolves import-prefixed tokens
recursively, and the instruction files reference tokens that are not files. To
stay compatible with Qwen Code this file contains no such tokens — read
CLAUDE.md directly.

## INHERITED FROM constitution/CLAUDE.md

This module's CLAUDE.md inherits, unconditionally, every rule in
constitution/CLAUDE.md and the constitution/Constitution.md it references — the
HelixConstitution submodule mounted at the parent project's constitution/
directory (resolve the path with constitution/find_constitution.sh from the
parent project root). Qwen Code MUST NOT weaken any inherited rule.

## Anti-Bluff — read first

Tests and Challenges exist for exactly one purpose: to confirm a feature
genuinely works for a real end user, end-to-end. A test that passes while the
feature is broken is a bluff test and is forbidden. CI green is necessary,
never sufficient. See this module's CLAUDE.md, AGENTS.md, and CONSTITUTION.md
for the full Sixth/Seventh Law and section 6.J / 6.L mandate.

## §11.4.78 — CodeGraph code-intelligence mandate

Inherited by §11.4.78 ID reference from `constitution/Constitution.md` §11.4.78 (this module's `CLAUDE.md` and `CONSTITUTION.md` carry the full anchor with the package name and install commands). In brief: every project worked on by AI coding agents MUST install, initialize, and use CodeGraph — a local semantic code-knowledge-graph exposed to agents over MCP — wired into every CLI agent the developers use, covered by an anti-bluff verification suite. See `CLAUDE.md` and `CONSTITUTION.md` in this module, and the constitution submodule `Constitution.md` §11.4.78, for the full mandate.

## §107 — End-user-usability covenant (verbatim operator mandate, 2026-05-22)

> **Verbatim operator mandate** (2026-05-22, restated for QWEN.md parity per the §11.4.83 cascade requirement):
>
> "all existing tests and Challenges do work in anti-bluff manner - they MUST confirm that all tested codebase really works as expected! We had been in position that all tests do execute with success and all Challenges as well, but in reality the most of the features does not work and can't be used! This MUST NOT be the case and execution of tests and Challenges MUST guarantee the quality, the completition and full usability by end users of the product! This MUST BE part of Constitution of our project, its CLAUDE.MD and AGENTS.MD if it is not there already, and to be applied to all Submodules's Constitution, CLAUDE.MD and AGENTS.MD as well (if not there already)!"

**Inheritance.** This submodule's consumers (Herald and any other downstream project) inherit the §107 covenant unchanged. The bar for shipping any consumer-visible feature is NOT "tests pass" — it is **"the end user of the binary or library that consumes this submodule can actually use the feature."** Every PASS (unit, integration, gate, Challenge, smoke, e2e) MUST carry positive runtime evidence that the user-visible behaviour works. Metadata-only / configuration-only / "absence-of-error" / grep-only PASS are §11.4 PASS-bluffs and constitute critical defects regardless of how green the summary line looks.

**Evidence responsibility.** §107 evidence inside this submodule is the responsibility of the submodule's own unit + integration tests + Challenges. §107 evidence for a consumer-visible feature that traverses this submodule is the responsibility of the consumer's end-to-end proofs (Herald's `scripts/e2e_bluff_hunt.sh` + the per-feature `docs/qa/<run-id>/` artefact mandated by §11.4.83).

**Canonical authority.** Helix Universal Constitution §11.4 + §11.4.1..§11.4.16 (anti-bluff substrate) and the existing §107 anchor already carried by this submodule's `CONSTITUTION.md`, `CLAUDE.md`, and `AGENTS.md`. This QWEN.md section restates the anchor for Qwen Code session parity per the operator's 2026-05-22 mandate that the §107 covenant MUST appear in every QWEN.md across the Helix-stack inheritance chain.

**Non-compliance is a release blocker.** No `--metadata-only-suffices`, `--green-summary-suffices`, `--coverage-suffices` flag exists.
