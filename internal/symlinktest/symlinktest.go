// Package symlinktest lets a test create a symlink without caring whether the
// host grants the privilege to do so.
//
// CONTRIBUTING.md states that symlink tests skip themselves on Windows builds
// where the process lacks SeCreateSymbolicLinkPrivilege. That was true of one
// package. Forty-eight other test files called os.Symlink and t.Fatal'd on the
// error, so on an ordinary Windows checkout they failed — and a failing test
// makes its whole package report FAIL, which silently removed every other test
// in that package from the regression gate. Nine packages were invisible for
// this reason, and three of them are the packages the identity rename touches
// most.
//
// This package is imported only from _test.go files, so it never reaches the
// product binary and never enters the deadcode graph rooted at ./cmd.
package symlinktest

import (
	"errors"
	"os"
	"syscall"
	"testing"
)

// errPrivilegeNotHeld is the Windows ERROR_PRIVILEGE_NOT_HELD returned by
// os.Symlink when the process lacks SeCreateSymbolicLinkPrivilege.
const errPrivilegeNotHeld = syscall.Errno(1314)

// IsPrivilegeError reports whether err is os.Symlink refusing for lack of the
// Windows symlink privilege, as opposed to any other failure.
//
// The errno must be unwrapped by hand: errors.Is does NOT map 1314 to
// os.ErrPermission, so the obvious check silently never matches. A bare errno
// outside *os.LinkError is rejected on purpose — os.Symlink always wraps in
// *os.LinkError, so a bare 1314 came from somewhere else and swallowing it
// would hide a real failure.
func IsPrivilegeError(err error) bool {
	var le *os.LinkError
	if !errors.As(err, &le) {
		return false
	}
	var errno syscall.Errno
	if !errors.As(le.Err, &errno) {
		return false
	}
	return errno == errPrivilegeNotHeld
}

// MustSymlink creates a symlink, skipping the test if the host withholds the
// privilege and failing it on any other error.
//
// It is deliberately one call rather than a predicate the caller combines with
// its own error handling: the two-step form is what every unguarded site in
// this repository got wrong, each in the same way.
//
// Known limitation, verified rather than assumed: Windows checks the privilege
// BEFORE it validates the path, so on a host without the privilege os.Symlink
// reports errno 1314 even for a link into a directory that does not exist. Every
// symlink failure therefore looks like the privilege refusal, and this function
// will skip on a genuine bug as readily as on a missing privilege.
//
// That trade is accepted on purpose. The alternative is the previous state, in
// which these tests failed and took every other test in their package out of the
// regression gate. A skip that occasionally hides a Windows symlink bug costs
// less than nine packages with no gate at all. On a host that grants the
// privilege — CI on Linux and macOS, or Windows with Developer Mode — the
// masking does not occur and real errors surface normally.
func MustSymlink(t testing.TB, target, link string) {
	t.Helper()
	err := os.Symlink(target, link)
	if err == nil {
		return
	}
	if IsPrivilegeError(err) {
		t.Skipf("skipping: SeCreateSymbolicLinkPrivilege not held on this host, so %q cannot be created: %v", link, err)
		return
	}
	t.Fatalf("symlink %q -> %q: %v", link, target, err)
}

// SkipIfPrivilegeError skips the test when err is the privilege refusal and
// otherwise does nothing, leaving the caller to handle a real error.
//
// Prefer MustSymlink. This exists for the sites that need the error value —
// a test asserting that symlink creation is rejected, for instance.
func SkipIfPrivilegeError(t testing.TB, err error) {
	t.Helper()
	if IsPrivilegeError(err) {
		t.Skipf("skipping: SeCreateSymbolicLinkPrivilege not held on this host: %v", err)
	}
}
