package appidentity_test

import (
	"testing"

	"bitbucket.org/hgt_development/hgtran-ai/v2/internal/appidentity"
)

func TestNameIsTheForkBinary(t *testing.T) {
	if got, want := appidentity.Name, "hgtran-ai"; got != want {
		t.Fatalf("Name = %q, want %q", got, want)
	}
}

func TestLegacyNameIsKeptForMigration(t *testing.T) {
	if got, want := appidentity.LegacyName, "gentle-ai"; got != want {
		t.Fatalf("LegacyName = %q, want %q", got, want)
	}
}

// TestNameAndLegacyNameDiffer guards the invariant the migration paths depend
// on. If the two ever collapse to the same string, code that distinguishes a
// pre-rename install from a current one silently stops distinguishing anything
// while still compiling and still passing every other test.
func TestNameAndLegacyNameDiffer(t *testing.T) {
	if appidentity.Name == appidentity.LegacyName {
		t.Fatal("Name and LegacyName are identical; migration code can no longer tell the two installs apart")
	}
}

// TestCandidatesPrefersTheCurrentNameFirst pins the order. It matters: a user
// mid-migration can have both the renamed binary and the pre-fork one on PATH
// at once, and resolving to the old one would silently keep using the install
// the rename was meant to replace.
func TestCandidatesPrefersTheCurrentNameFirst(t *testing.T) {
	got := appidentity.Candidates()
	if len(got) != 2 || got[0] != appidentity.Name || got[1] != appidentity.LegacyName {
		t.Fatalf("Candidates() = %v, want [%q %q]", got, appidentity.Name, appidentity.LegacyName)
	}
}

func TestResolveExistingPrefersCurrentWhenBothExist(t *testing.T) {
	name, ok := appidentity.ResolveExisting(func(string) bool { return true })
	if !ok || name != appidentity.Name {
		t.Fatalf("ResolveExisting(all exist) = %q,%v; want %q,true", name, ok, appidentity.Name)
	}
}

// TestResolveExistingFallsBackToLegacy is the whole point of the type: an
// install that predates the rename has the old name on disk and nothing else.
func TestResolveExistingFallsBackToLegacy(t *testing.T) {
	name, ok := appidentity.ResolveExisting(func(n string) bool { return n == appidentity.LegacyName })
	if !ok || name != appidentity.LegacyName {
		t.Fatalf("ResolveExisting(legacy only) = %q,%v; want %q,true", name, ok, appidentity.LegacyName)
	}
}

func TestResolveExistingReportsWhenNeitherExists(t *testing.T) {
	if name, ok := appidentity.ResolveExisting(func(string) bool { return false }); ok || name != "" {
		t.Fatalf("ResolveExisting(none) = %q,%v; want \"\",false", name, ok)
	}
}
