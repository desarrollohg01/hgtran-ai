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
