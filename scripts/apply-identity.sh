#!/usr/bin/env bash
# Re-apply the HG identity rename over the working tree.
#
# Why this exists: the rename was performed once, by hand, across ~1000 files.
# That made every future upstream sync intractable — merging a month of upstream
# work would mean redoing the substitution manually. A mechanical rename is not
# something you merge; it is something you RE-APPLY. This script is the applier,
# and scripts/rename-harness.sh is its verifier.
#
#   bash scripts/apply-identity.sh --check   # report what would change, change nothing
#   bash scripts/apply-identity.sh           # apply
#
# Scope, stated plainly so nobody trusts it further than it goes. This handles
# the MECHANICAL classes: module path, binary/command name, repository URLs and
# the state-root directory. It deliberately does NOT touch the bare words
# "gentleman"/"Gentleman", because three live things wear them and each is a
# product decision rather than a substitution:
#
#   - the Homebrew tap `gentleman-programming/tap/...`, which is a real formula
#     published under that name; rewriting it points at a formula that does not
#     exist (see internal/update/upgrade/strategy.go)
#   - the persona component, which installs a persona named after the upstream
#     author into every configured agent
#   - authorship attribution in LICENSE and CONTRIBUTORS.md
#
# Run the harness afterwards. The applier changes; only the harness tells you
# whether the change was right.

set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

MODE="${1:-apply}"

green() { printf '\033[32m%s\033[0m\n' "$1"; }
red()   { printf '\033[31m%s\033[0m\n' "$1"; }
dim()   { printf '\033[2m%s\033[0m\n' "$1"; }

# --- what MUST NOT be rewritten ---------------------------------------------
#
# Every entry here is a place where the pre-fork name is the correct, intended
# content. Rewriting any of them is a silent defect, and one of them has already
# happened: commit 4ab09a59 let a bulk substitution rewrite the harness's own
# search patterns, so the census spent weeks measuring the new identity while
# reporting it as the old one. That is why this list exists and why the guard
# below refuses to run without it.
EXCLUDE_PATHS=(
  ':!scripts/apply-identity.sh'                    # itself
  ':!scripts/rename-harness.sh'                    # the verifier — see 4ab09a59
  ':!.rename-baseline.txt'                         # recorded history
  ':!internal/appidentity/'                        # LegacyName is the pre-fork name on purpose
  ':!internal/statepath/'                          # LegacyDirName, same reason
  ':!internal/reviewtransaction/testdata/'         # frozen capture fixtures, byte-exact by design
  ':!LICENSE'                                      # authorship
  ':!CONTRIBUTORS.md'                              # authorship
  ':!docs/audits/'                                 # dated records of what was true then
  ':!openspec/changes/archive/'                    # same
)

# --- historical links are not ours to repoint --------------------------------
#
# A URL like github.com/Gentleman-Programming/gentle-ai/pull/1801 is a citation
# of the upstream's own pull request. Repointing it produces a link into HG's
# repository where that number means something else, or nothing.
#
# This is not hypothetical. The bulk rename of 4ab09a59 rewrote 434 such links
# across 74 files into `desarrollohg01/hgtran-ai`, a repository that
# returns 404 — the upstream owner paired with our repository name. The docs now
# cite dead references to work that does exist, under a name it never had.
#
# A line matching this is skipped whole. Erring toward leaving a link alone is
# cheap; erring the other way breaks a citation nobody will re-derive.
HISTORICAL_LINK='github\.com/[Gg]entleman-[Pp]rogramming/[a-z-]*/(pull|issues|commit|compare|releases/tag)/'

# --- the substitutions, longest first ---------------------------------------
#
# Order is load-bearing: the URL forms must run before the bare binary name, or
# `gentle-ai` inside a URL gets rewritten first and the longer patterns stop
# matching.
declare -a SUBS=(
  's|github\.com/gentleman-programming/gentle-ai|github.com/desarrollohg01/hgtran-ai|g'
  's|github\.com/Gentleman-Programming/gentle-ai|github.com/desarrollohg01/hgtran-ai|g'
  's|Gentleman-Programming/gentle-ai|desarrollohg01/hgtran-ai|g'
  's|gentleman-programming/gentle-ai|desarrollohg01/hgtran-ai|g'
  's|GENTLE_AI_CHANNEL|HGTRAN_AI_CHANNEL|g'
  's|\.gentle-ai|.hgtran-ai|g'
  's|gentle-ai|hgtran-ai|g'
  's|Gentle-AI|HGTran-AI|g'
  's|Gentle AI|HGTran AI|g'
)

# --- deliberate non-renames -------------------------------------------------
#
# These look like gaps in the table above and are not. Each was checked; adding
# a rule for it would break something that currently works. Listed here so the
# next reader — human or agent — does not "complete" the table and regress:
#
#   GGA_SKIP_FILE_CHECK, GGA_TEST_CAPTURE, GGA_TEST_EXIT
#       Environment variables this repository sets to drive the external `hga`
#       binary. They are a contract with that program, which deliberately kept
#       its GGA_* names because they are also written into user git hooks and
#       matched by exact string. Renaming them here breaks the integration.
#
#   gentle-engram, gentle-pi
#       Published package and harness names — `gentle-engram@latest` is what npm
#       resolves, and `pi install npm:gentle-pi` is documented as such. Renaming
#       either produces an install command for a package that does not exist.
#
#   bare `gga` in prose
#       The binary is now `hga`, so documentation SHOULD say hga — but a blanket
#       substitution cannot tell prose from the GGA_* variables above. Prose is
#       corrected by hand when the file is touched, never by this script.

# Guard: refuse to run if the substitution table has itself been rewritten.
#
# A left-hand side that no longer names the pre-fork identity is the same class
# of corruption that hit the harness. Fail loudly rather than perform a no-op
# rename and report success.
for s in "${SUBS[@]}"; do
  lhs="${s#s|}"; lhs="${lhs%%|*}"
  case "$lhs" in
    *hgtran*|*HGTran*|*hgt_development*|*desarrollohg01*)
      red "FATAL: a substitution searches for the CURRENT identity, not the pre-fork one."
      red "       A rename rewrote this file. Restore the table before running."
      red "       Offending pattern: $lhs"
      exit 2 ;;
  esac
done

# Files git tracks that still carry the pre-fork identity, minus the exclusions.
targets() {
  git grep -I -l -e 'gentle-ai' -e 'Gentle-AI' -e 'Gentle AI' \
    -- . "${EXCLUDE_PATHS[@]}" 2>/dev/null | sort -u
}

mapfile -t FILES < <(targets)

if [[ ${#FILES[@]} -eq 0 ]]; then
  green "Nothing to apply — no tracked file outside the exclusions carries the pre-fork identity."
  exit 0
fi

if [[ "$MODE" == "--check" ]]; then
  dim "Would rewrite ${#FILES[@]} file(s):"
  for f in "${FILES[@]}"; do
    n=$(grep -c -e 'gentle-ai' -e 'Gentle-AI' -e 'Gentle AI' "$f" 2>/dev/null || echo 0)
    printf '  %-72s %s\n' "$f" "$n"
  done
  dim ""
  dim "Excluded from the rename by policy: ${#EXCLUDE_PATHS[@]} path rule(s) — see the list in this script."
  exit 0
fi

changed=0
skipped_lines=0
for f in "${FILES[@]}"; do
  before="$(cat "$f")"
  # Lines carrying a historical upstream citation are passed through untouched;
  # everything else goes through the substitution table.
  while IFS= read -r line; do
    if [[ "$line" =~ $HISTORICAL_LINK ]]; then
      printf '%s\n' "$line"
      skipped_lines=$((skipped_lines + 1))
      continue
    fi
    for s in "${SUBS[@]}"; do
      line="$(printf '%s' "$line" | sed "$s")"
    done
    printf '%s\n' "$line"
  done < "$f" > "$f.tmp" && mv "$f.tmp" "$f"
  [[ "$(cat "$f")" != "$before" ]] && changed=$((changed + 1))
done

green "Applied the identity rename to $changed file(s)."
[[ $skipped_lines -gt 0 ]] && dim "Left $skipped_lines line(s) untouched: historical upstream citations."
dim "Now run: bash scripts/rename-harness.sh check"
dim "The applier changes; only the harness says whether the change was right."
