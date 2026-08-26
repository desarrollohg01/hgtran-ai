# Tasks: Restore the regression gate on Windows

Order matters: Task 1 before Task 2. The slow package hides its own symlink failures, so the
symlink work there cannot be verified until the package finishes.

## Phase 1: Make `internal/reviewtransaction` finish

- [ ] T-01 Measure where the time goes before changing anything: `go test -count=1 -v -timeout 90m
      ./internal/reviewtransaction/ > rt-verbose.log 2>&1` in the background, then rank tests by
      duration. Do NOT optimise from the hypothesis below — confirm it first. The 27 `exec.Command`
      sites are the suspect, not the verdict
- [ ] T-02 Decide per slow test whether the git subprocess is what is under test or merely
      scaffolding. Where it is scaffolding, replace it with a fixture. Where it IS the subject,
      leave it: a test that proves git integration must run git
- [ ] T-03 Widen `t.Parallel()` where the tests do not share a working directory. 58 of 903 is low,
      but a shared temp root makes parallelism unsafe — check before adding it
- [ ] T-04 Declare an explicit `-timeout` for the package in whatever runs the gate, and write down
      the budget. A package with no declared budget silently inherits 10 minutes and fails when it
      grows past it, which is what happened here
- [ ] T-05 If after T-02/T-03 it still does not fit a sane budget, move it to a nightly job ON
      PURPOSE and record the decision. An honest exclusion beats a gate everyone has learned to
      ignore

## Phase 2: Finish propagating `symlinktest`

- [ ] T-06 With `reviewtransaction` finishing, run it and collect the symlink failures that the
      timeout was hiding. They have never been observed — do not assume the count
- [ ] T-07 Convert the remaining unguarded sites to `symlinktest.MustSymlink`. Inventory:
      `rg -l "os\.Symlink" --glob "*_test.go" | while read f; do rg -q symlinktest "$f" || echo "$f"; done`
- [ ] T-08 Prefer `MustSymlink` over `SkipIfPrivilegeError`, per the package doc. Watch for sites
      that already call `t.Skipf` on ANY error — those look correct and are not: they hide real
      symlink bugs too. `pi_codegraph_test.go` had one
- [ ] T-09 Verify the way this change was verified: run each converted package on a host WITHOUT
      the privilege and confirm FAIL becomes SKIP and the package reports `ok`. Report the number
      of tests returned to the gate, not the number of lines changed

## Phase 3: `e2e/organicruntime`

- [ ] T-10 Establish why it exceeds 10 minutes. It is not the same cause as `reviewtransaction`:
      its log shows a `git clone` of gentleman-guardian-angel failing on `open /dev/tty`, which
      means it wants an interactive terminal it will never get in a headless run
- [ ] T-11 Either give it a non-interactive path or take it out of the default gate explicitly

## Phase 4: Make the documentation true

- [ ] T-12 Correct `CONTRIBUTING.md` line 173. It promises automatic skipping that only happens at
      guarded sites. State the rule that actually holds: sites using `symlinktest` skip, unguarded
      sites fail — and point contributors at the helper
- [ ] T-13 State the requirement so it is checkable: a new test that creates a symlink goes through
      `internal/symlinktest`. Consider a gate that greps for bare `os.Symlink` in `_test.go`, since
      a rule nobody can verify is a rule that decays — this one already did

## Verification

- [ ] `go test -count=1 ./...` completes with no FAIL on Windows without the symlink privilege
- [ ] The same command completes on a host WITH the privilege, and the tests run instead of skipping
- [ ] Every package excluded from the default gate is listed somewhere a person will find it
