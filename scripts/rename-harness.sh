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

count() { rg -c "$1" --glob '!.git' --glob '!.rename-baseline.txt' --glob "!scripts/rename-harness.sh" "${@:2}" 2>/dev/null | awk -F: '{s+=$NF} END {print s+0}'; }

census() {
  echo "I1_binary=$(count '\bgentle-ai\b')"
  echo "I2_module=$(count 'gentleman-programming/gentle-ai' --glob '*.go')"
  echo "I3_gga=$(count '\bgga\b')"
  echo "I4_urls=$(count 'github.com/Gentleman-Programming')"
  echo "I5_brand=$(count 'Gentle AI')"
  echo "I6_golden=$(rg -il 'gentleman|gentle-ai' --glob '*.golden' 2>/dev/null | wc -l | tr -d ' ')"
  echo "TOTAL=$(count -i 'gentleman|gentle-ai')"
  echo "FILES=$(rg -il 'gentleman|gentle-ai' --glob '!.git' 2>/dev/null | wc -l | tr -d ' ')"
}

# --- functional state -------------------------------------------------------

# Packages whose tests pass right now. Comparing this set before/after is what
# catches "the rename silently broke a package" — a raw pass/fail count would
# hide a package that stopped being compiled at all.
passing_packages() {
  go test ./... 2>/dev/null | awk '/^ok /{print $2}' | sort
}

build_state() {
  if go build ./... >/dev/null 2>&1; then echo "BUILD=ok"; else echo "BUILD=fail"; fi
  if go vet ./... >/dev/null 2>&1; then echo "VET=ok"; else echo "VET=fail"; fi
}

# --- modes ------------------------------------------------------------------

if [[ "$MODE" == "baseline" ]]; then
  echo "Capturing baseline — this runs the full suite, expect a few minutes."
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
echo "== Build and vet =="
now_build="$(build_state)"
base_build="$(rg '^(BUILD|VET)=' "$BASELINE")"
if [[ "$now_build" == "$base_build" ]]; then
  green "  build/vet unchanged from baseline"
else
  red "  build/vet regressed"
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
