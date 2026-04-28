#!/usr/bin/env bash
#
# scripts/ci.sh — single local entry point for digital.vasic.http3 CI.
#
# Per CONSTITUTION.md, this module does NOT use hosted CI services. This
# script IS the CI. Pre-push hooks and tag scripts must invoke this.
#
# Usage:
#   scripts/ci.sh                # full gate (default)
#   scripts/ci.sh --quick        # skip fuzz / vulncheck (dev iteration)
#   scripts/ci.sh --fuzz-time=2m # override default 30s fuzz duration

set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

QUICK=false
FUZZ_TIME="30s"
for arg in "$@"; do
  case "$arg" in
    --quick) QUICK=true ;;
    --fuzz-time=*) FUZZ_TIME="${arg#--fuzz-time=}" ;;
    -h|--help)
      sed -n '/^# Usage/,/^# *$/p' "$0" | sed 's/^# \?//'
      exit 0
      ;;
    *) echo "unknown flag: $arg" >&2; exit 2 ;;
  esac
done

log() { printf '\033[1;36m[ci]\033[0m %s\n' "$*"; }
fail() { printf '\033[1;31m[ci:fail]\033[0m %s\n' "$*" >&2; exit 1; }

# 1. Tidy + diff check
#
# Snapshot go.mod/go.sum, run tidy, diff. We do NOT compare against git index
# because the script must work in a freshly cloned repo before the first
# commit. The contract is: tidy must be a no-op against the current files.
log "step 1/7  go mod tidy"
_pre_modsum="$(sha256sum go.mod go.sum 2>/dev/null | sort)"
go mod tidy
_post_modsum="$(sha256sum go.mod go.sum 2>/dev/null | sort)"
if [[ "$_pre_modsum" != "$_post_modsum" ]]; then
  git diff --no-index --no-color /dev/null go.mod 2>/dev/null | head -50 || true
  fail "go mod tidy changed go.mod / go.sum; commit the tidied result"
fi

# 2. Vet
log "step 2/7  go vet ./..."
go vet ./...

# 3. Build
log "step 3/7  go build ./..."
go build ./...

# 4. Test (race detector + count=1 to defeat caching)
log "step 4/7  go test -race -count=1 ./..."
go test -race -count=1 ./...

if $QUICK; then
  log "skipping fuzz / gosec / govulncheck (--quick)"
  log "ci OK (quick)"
  exit 0
fi

# 5. Fuzz
log "step 5/7  go test -fuzz=FuzzConfigValidate -fuzztime=$FUZZ_TIME ./pkg/server"
go test -run='^$' -fuzz=FuzzConfigValidate -fuzztime="$FUZZ_TIME" ./pkg/server

# 6. gosec (optional — warn but don't fail if not installed)
if command -v gosec >/dev/null 2>&1; then
  log "step 6/7  gosec ./..."
  gosec -quiet ./...
else
  log "step 6/7  gosec not installed; skipping (recommended: go install github.com/securego/gosec/v2/cmd/gosec@latest)"
fi

# 7. govulncheck
if command -v govulncheck >/dev/null 2>&1; then
  log "step 7/7  govulncheck ./..."
  govulncheck ./...
else
  log "step 7/7  govulncheck not installed; skipping (recommended: go install golang.org/x/vuln/cmd/govulncheck@latest)"
fi

log "ci OK"
