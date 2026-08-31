#!/usr/bin/env bash
# Verification harness for the hgtran-ai identity rename.
#
# The rename touches ~7598 occurrences across ~1086 files. At that scale the
# question is never "did I change enough" — it is "did I change anything I
# should not have". This harness answers both, mechanically.
#
#   bash scripts/rename-harness.sh baseline   # capture the pre-change state
#   bash scripts/rename-harness.sh check      # compare current state to it
#
# `check` fails if the build breaks, the suite regresses, or a package that
# used to have passing tests stops having them. It reports remaining
# occurrences per class but does NOT fail on them — progress is expected to be
# partial while slices land one at a time.

set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
BASELINE="${RENAME_HARNESS_BASELINE:-$ROOT/.rename-baseline.txt}"
MODE="${1:-check}"

green() { printf '\033[32m%s\033[0m\n' "$1"; }
red()   { printf '\033[31m%s\033[0m\n' "$1"; }
dim()   { printf '\033[2m%s\033[0m\n' "$1"; }

# --- occurrence census, by risk class (see exploration.md §1.1) -------------
#
# The census uses `git grep`, not `rg`, on purpose. What is being renamed is
# versioned repository content, and git is the authoritative view of that.
# ripgrep applies its own text/binary heuristics and reported 10 fewer files
# than git on identical content — a constant offset, but one that invites
# doubt about whether a number moved because of a change or because of a
# filter. git grep removes the ambiguity: if git does not track it, it is not
# part of the rename.
#
# The harness itself and the baseline file are excluded — they legitimately
# contain the old strings as search patterns and as recorded history.
SELF=':!scripts/rename-harness.sh :!.rename-baseline.txt'

count() { # <basic-regex> [pathspec...]
  # shellcheck disable=SC2086
  git grep -I -c -e "$1" -- "${@:2}" $SELF 2>/dev/null | awk -F: '{s+=$NF} END {print s+0}'
}
countfiles() {
  # shellcheck disable=SC2086
  git grep -I -l -e "$1" -- "${@:2}" $SELF 2>/dev/null | wc -l | tr -d ' '
}

census() {
  echo "I1_binary=$(count '\bgentle-ai\b')"
  echo "I2_module=$(count 'gentleman-programming/hgtran-ai' '*.go')"
  echo "I3_gga=$(count '\bgga\b')"
  echo "I4_urls=$(count 'github.com/Gentleman-Programming')"
  echo "I5_brand=$(count 'Hgtran AI')"
  echo "I6_golden=$(countfiles '\(gentleman\|hgtran-ai\)' '*.golden')"
  # The user state root, tracked apart from the binary name because it is the
  # one class where getting it wrong orphans data instead of breaking a build.
  # The quoted form excludes the three filename prefixes that merely start with
  # the old name (.hgtran-ai-*.tmp, .hgtran-ai-default-agent.json, and the
  # OpenCode uninstall marker) — those are brand, not the state root.
  echo "I7_stateroot=$(count '"\.hgtran-ai"')"
  echo "TOTAL=$(count '\(gentleman\|hgtran-ai\|Gentleman\|Hgtran AI\)')"
  echo "FILES=$(countfiles '\(gentleman\|hgtran-ai\|Gentleman\|Hgtran AI\)')"
}

# --- functional state -------------------------------------------------------

# Packages whose tests pass right now. Comparing this set before/after is what
# catches "the rename silently broke a package" — a raw pass/fail count would
# hide a package that stopped being compiled at all.
#
# e2e/organicruntime is excluded by default: it exceeds the per-package timeout
# and is killed at 11m, so including it costs eleven minutes per run to learn
# nothing. It already fails on the baseline, so it can never be a regression
# signal. Set RENAME_HARNESS_FULL=1 to include it before the final slice.
# Packages excluded from the gate, and why each one earns its exclusion.
#
# The rule: a package that does not pass today can never produce a regression
# signal, because a regression is "used to pass, now does not". Running one
# costs its full time budget to learn nothing. Excluding it costs no signal.
#
# What this exclusion MUST NOT become is a silent cap. The count is printed on
# every run, and every entry is listed here with its measured reason:
#
#   e2e/organicruntime                exceeds the budget; killed at ~11m
#   internal/reviewtransaction        exceeds the budget (documented debt)
#   internal/sddstatus                exceeds 300s
#   internal/cli                      exceeds the budget
#   internal/components/sdd           fails
#   internal/components/communitytool fails
#
# Set RENAME_HARNESS_FULL=1 to run everything before the final slice.
EXCLUDE_PKG="${RENAME_HARNESS_EXCLUDE:-e2e/organicruntime|internal/reviewtransaction|internal/sddstatus|internal/cli|internal/components/sdd|internal/components/communitytool}"

# Per-package time budget. Measured, not guessed: internal/update needs 165s on
# this hardware, so a 60s bound reports it as failing and blinds the gate to
# every real regression in it. 300s clears the slowest package that passes,
# with margin.
PKG_TIMEOUT="${RENAME_HARNESS_PKG_TIMEOUT:-300s}"

packages() {
  if [[ -n "$EXCLUDE_PKG" && "${RENAME_HARNESS_FULL:-0}" != "1" ]]; then
    go list ./... 2>/dev/null | rg -v "$EXCLUDE_PKG"
  else
    go list ./... 2>/dev/null
  fi
}

excluded_count() {
  local all kept
  all="$(go list ./... 2>/dev/null | rg -c '' || echo 0)"
  kept="$(packages | rg -c '' || echo 0)"
  echo $((all - kept))
}

# Package identity is recorded WITHOUT the module prefix.
#
# This matters: renaming the module changes the import path of every package in
# it. Comparing full paths across that rename reports the entire repository as
# "lost" — which is what the first run of this harness did. The part that must
# stay stable is the path *within* the module, so that is what gets compared.
strip_module() {
  local mod
  mod="$(go list -m 2>/dev/null)"
  if [[ -n "$mod" ]]; then sed "s|^${mod}/||; s|^${mod}$|.|"; else cat; fi
}

# Packages whose tests pass, as a sorted, deduplicated list of REAL packages.
#
# The naive version harvested every line matching /^ok / from the combined test
# output. Test bodies print to stdout too, so an arbitrary line beginning with
# "ok " was collected as if it were a package result. A stray "e" landed in the
# baseline that way, out of order and followed by a second sorted run of
# duplicates — and `comm` on unsorted input silently reports garbage, so the
# gate claimed three regressions while comparing the baseline to itself.
#
# The fix is to intersect against `go list`: whatever is not a package Go knows
# about is noise by definition. `sort -u` then guarantees the total order and
# uniqueness `comm` requires.
passing_packages() {
  local known reported
  known="$(mktemp)"; reported="$(mktemp)"
  packages | strip_module | sort -u > "$known"
  # shellcheck disable=SC2046
  go test -timeout "$PKG_TIMEOUT" $(packages) 2>/dev/null \
    | awk '/^ok /{print $2}' | strip_module | sort -u > "$reported"
  comm -12 "$reported" "$known"
  rm -f "$known" "$reported"
}

build_state() {
  if go build ./... >/dev/null 2>&1; then echo "BUILD=ok"; else echo "BUILD=fail"; fi
  if go vet ./... >/dev/null 2>&1; then echo "VET=ok"; else echo "VET=fail"; fi
  # The format gate belongs here and was missing from the first version of this
  # harness. The module rename reordered import blocks — bitbucket.org sorts
  # before github.com — and gofmt requires imports sorted within their group.
  # Build and vet were both happy; ci.yml:34 would not have been. A verification
  # harness that omits a gate CI enforces is a harness that lies by omission.
  if go run ./internal/gofmtcheck >/dev/null 2>&1; then echo "FMT=ok"; else echo "FMT=fail"; fi
}

# --- modes ------------------------------------------------------------------

if [[ "$MODE" == "baseline" ]]; then
  echo "Capturing baseline — this runs the full suite, expect a few minutes."
  dim "Excluded from the gate: $(excluded_count) package(s) that do not pass today — see the list in this script."
  {
    echo "# rename harness baseline"
    build_state
    census
    echo "--- passing packages ---"
    passing_packages
  } > "$BASELINE"
  green "Baseline written to $BASELINE"
  dim "$(rg -c '' "$BASELINE" | tr -d ' ') lines captured"
  exit 0
fi

if [[ ! -f "$BASELINE" ]]; then
  red "No baseline at $BASELINE — run: bash scripts/rename-harness.sh baseline"
  exit 1
fi

fails=0
echo "== Build, vet and format =="
now_build="$(build_state)"
base_build="$(rg '^(BUILD|VET|FMT)=' "$BASELINE")"
if [[ "$now_build" == "$base_build" ]]; then
  green "  build/vet/fmt unchanged from baseline"
else
  red "  build/vet/fmt regressed"
  printf '    baseline: %s\n    now:      %s\n' "$(echo "$base_build" | tr '\n' ' ')" "$(echo "$now_build" | tr '\n' ' ')"
  fails=$((fails + 1))
fi

echo "== Test packages =="
tmp="$(mktemp)"
passing_packages > "$tmp"
base_pkgs="$(mktemp)"
awk '/^--- passing packages ---$/{f=1;next} f' "$BASELINE" > "$base_pkgs"

# A package that passed before and does not now is a regression. A package that
# passes now and did not before is fine (and expected: renamed import paths).
lost="$(comm -23 "$base_pkgs" "$tmp")"
if [[ -z "$lost" ]]; then
  green "  no package lost its passing tests ($(rg -c '' "$tmp" | tr -d ' ') passing)"
else
  red "  packages that regressed:"
  printf '    %s\n' $lost
  fails=$((fails + 1))
fi
rm -f "$tmp" "$base_pkgs"

echo "== Rename census =="
printf '  %-14s %10s %10s\n' "class" "baseline" "now"
while IFS='=' read -r k v; do
  [[ "$k" =~ ^(I[1-6]_|TOTAL|FILES) ]] || continue
  nv="$(census | rg "^$k=" | cut -d= -f2)"
  if [[ "$nv" -lt "$v" ]]; then mark=$'\033[32m↓\033[0m'
  elif [[ "$nv" -gt "$v" ]]; then mark=$'\033[31m↑\033[0m'
  else mark=" "; fi
  printf '  %-14s %10s %10s %b\n' "$k" "$v" "$nv" "$mark"
done < <(rg '^(I[1-6]_|TOTAL|FILES)' "$BASELINE")

echo
if [[ "$fails" -eq 0 ]]; then
  green "Harness passed — no functional regression."
  exit 0
fi
red "$fails harness check(s) failed."
exit 1
