# Proposal: Restore the regression gate on Windows

## Intent

`go test ./...` on an ordinary Windows checkout reports **7 failing packages**, and none of
them fails because of a defect in the product. Two unrelated environmental causes are mixed
together, and each one hides work behind it.

A gate that is red for reasons nobody can act on stops being read. Once it stops being read,
the day it goes red for a real reason, nobody notices. That is the actual cost here — not the
seven lines of output.

Measured on 2026-08-19, branch `feat/estandares-crud-hg`, Windows 11 without
`SeCreateSymbolicLinkPrivilege`:

| Cause | Packages | Status |
|---|---|---|
| `os.Symlink` fails and the test calls `t.Fatal` | `cli`, `communitytool`, `sddstatus` | **Fixed** in `8e4b7e1b` |
| `os.Symlink` fails, still unguarded | `reviewtransaction` and 39 other files | Open — see Task 2 |
| Package exceeds the 10-minute default timeout | `reviewtransaction`, `e2e/organicruntime` | Open — see Task 1 |

## The two causes, and why they must be fixed in this order

**The timeout hides the symlinks.** `internal/reviewtransaction` holds 20+ test files that call
`os.Symlink` without the guard, but the package never reaches them: it aborts on time first.
Fixing the symlinks there cannot even be *verified* until the package finishes. So the slow
package comes first.

## Scope

### In Scope
- Make `internal/reviewtransaction` complete within a bounded, declared time budget
- Make `e2e/organicruntime` complete, or move it out of the default gate deliberately and say so
- Propagate `internal/symlinktest` to the remaining unguarded `os.Symlink` sites
- Correct `CONTRIBUTING.md`, which currently documents behaviour the code does not have

### Out of Scope
- Rewriting what the tests assert. This change makes the existing suite runnable; it does not
  revisit coverage
- Enabling Developer Mode on any particular machine. That is a workaround for one host, not a
  fix for the repository

## Evidence

### `internal/reviewtransaction` does not finish

Not a deadlock, which was the first hypothesis and was wrong. Run with `-timeout 40m`, it
aborted at 2400s, and the test in flight at that moment had been running for **1m7s**. Nothing
was stuck; the package simply costs more than 40 minutes of wall clock.

- **903 tests** in the package
- A 7-test sample of the `TestCompactPrePRChain` family took **454s** — roughly 65s each
- The slowest single test measured, `TestCompactPrePRChainRejectsInvalidMembersAndBindings`,
  takes **201s** on its own
- The tests shell out: **27 `exec.Command`** and 73 mentions of git across the package's
  `_test.go` files. On Windows every process creation is expensive, and the antivirus scans each one
- **58 `t.Parallel()`** calls spread across 903 tests

### The symlink guard existed and nobody used it

`internal/symlinktest` landed in `eac327fc` with `MustSymlink` and `SkipIfPrivilegeError`. When
this change began it had **zero** callers — the one grep match was a comment in `filemerge`
mentioning errno 1314, not a call.

`8e4b7e1b` converted the three packages that were failing at that moment. The measured effect:
5 tests moved from FAIL to SKIP, and **1 326 tests returned to the gate** (1 035 in `cli`, 200
in `sddstatus`, 91 in `communitytool`) — tests that were being silently dropped because one
failing test takes its whole package down with it.

**43 test files call `os.Symlink`. Four sites are now guarded. The rest are not.**

### `CONTRIBUTING.md` documents behaviour that does not exist

Line 173 states that symlink tests "will be **skipped automatically** on Windows builds where
the process lacks `SeCreateSymbolicLinkPrivilege`".

That was not true when it was written and is only partly true now. Unguarded sites do not skip:
they fail, and they take their package with them. A contributor who reads that line and then
sees a red suite concludes the repository is broken, which is the opposite of what the document
is for.

## Capabilities

### Modified
- `regression-gate` — every package either completes within its declared budget or is excluded
  from the default gate on purpose, with the exclusion written down
