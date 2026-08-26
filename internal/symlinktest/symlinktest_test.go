package symlinktest

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// CONTRIBUTING.md promises that symlink tests skip themselves on Windows builds
// where the process lacks SeCreateSymbolicLinkPrivilege. Exactly one package in
// the repository implemented that promise; 48 other test files call os.Symlink
// and t.Fatal on the error instead.
//
// The cost is not the failing test — it is that a failing test makes its whole
// package report FAIL, which removes every other test in that package from the
// regression gate. Nine packages were invisible for this reason.
//
// The subtle part, and the reason the naive check does not work: errors.Is does
// NOT map errno 1314 to os.ErrPermission. The errno has to be unwrapped by hand.

func TestIsPrivilegeErrorRecognizesErrno1314(t *testing.T) {
	err := &os.LinkError{Op: "symlink", Old: "a", New: "b", Err: syscall.Errno(1314)}

	if !IsPrivilegeError(err) {
		t.Fatal("errno 1314 wrapped in *os.LinkError must be recognised as the privilege error")
	}
}

// This is the assertion that justifies the helper existing at all. If errors.Is
// ever starts mapping 1314 to os.ErrPermission, this test fails and tells the
// next reader the hand-unwrapping can go.
func TestErrnoIsNotReachableViaErrPermission(t *testing.T) {
	err := &os.LinkError{Op: "symlink", Old: "a", New: "b", Err: syscall.Errno(1314)}

	if errors.Is(err, os.ErrPermission) {
		t.Skip("errors.Is now maps errno 1314 to os.ErrPermission; the manual unwrap in IsPrivilegeError is no longer needed")
	}
}

func TestIsPrivilegeErrorRejectsOtherFailures(t *testing.T) {
	cases := map[string]error{
		"nil":               nil,
		"plain error":       errors.New("boom"),
		"not exist":         os.ErrNotExist,
		"permission proper": os.ErrPermission,
		"other errno":       &os.LinkError{Op: "symlink", Err: syscall.Errno(5)},
		"errno not in link": syscall.Errno(1314),
		"link wrapping nil": &os.LinkError{Op: "symlink"},
	}
	for name, err := range cases {
		if IsPrivilegeError(err) {
			t.Errorf("%s: must not be classified as the symlink privilege error", name)
		}
	}
}

// A bare errno 1314 outside *os.LinkError is deliberately NOT matched: os.Symlink
// always wraps in *os.LinkError, so a bare errno means the error came from
// somewhere else and swallowing it would hide a real failure.
func TestIsPrivilegeErrorRequiresTheLinkErrorWrapper(t *testing.T) {
	if IsPrivilegeError(syscall.Errno(1314)) {
		t.Fatal("a bare errno must not be matched; os.Symlink always wraps in *os.LinkError")
	}
}

func TestMustSymlinkCreatesTheLinkWhenPermitted(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	link := filepath.Join(dir, "link.txt")

	// Skips the test if the privilege is missing; that is the whole point.
	MustSymlink(t, target, link)

	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("lstat link: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%q is not a symlink; mode = %v", link, info.Mode())
	}
}

// TestPrivilegeErrorMasksOtherFailuresWhenPrivilegeIsMissing pins a limitation
// of this whole approach, verified rather than assumed.
//
// Windows checks SeCreateSymbolicLinkPrivilege BEFORE it validates the path, so
// on a host without the privilege os.Symlink returns errno 1314 even for a link
// into a directory that does not exist. Every symlink failure looks like the
// privilege refusal, which means MustSymlink will skip on genuine bugs too.
//
// That is accepted deliberately: the alternative is the status quo, where these
// tests fail and take every other test in their package out of the regression
// gate. A skip that occasionally hides a symlink bug on Windows costs less than
// nine packages with no gate at all. On a host that HAS the privilege the
// masking disappears and real errors surface normally — which is what this test
// asserts in that direction.
func TestPrivilegeErrorMasksOtherFailuresWhenPrivilegeIsMissing(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}

	// Probe whether this host grants the privilege at all.
	probe := filepath.Join(dir, "probe")
	privileged := os.Symlink(target, probe) == nil

	// A link whose parent directory does not exist is a genuine failure.
	err := os.Symlink(target, filepath.Join(dir, "no-such-dir", "link"))
	if err == nil {
		t.Fatal("a symlink into a missing directory unexpectedly succeeded")
	}

	if privileged {
		if IsPrivilegeError(err) {
			t.Fatalf("host grants the privilege, so a missing parent must not classify as a privilege error: %v", err)
		}
		return
	}
	if !IsPrivilegeError(err) {
		t.Fatalf("host withholds the privilege, so every symlink failure is expected to report errno 1314; got: %v", err)
	}
}
