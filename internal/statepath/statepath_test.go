package statepath

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The user state root used to be a private const in internal/state, which meant
// eighteen production sites hardcoded the literal instead. These tests pin the
// root down to one place so the next rename is a one-line change.

func TestRootUsesCurrentDirName(t *testing.T) {
	home := t.TempDir()
	got := Root(home)
	want := filepath.Join(home, ".hgtran-ai")
	if got != want {
		t.Fatalf("Root(%q) = %q, want %q", home, got, want)
	}
}

func TestDerivedPathsHangOffRoot(t *testing.T) {
	home := t.TempDir()
	root := Root(home)

	cases := map[string]string{
		"StateFile":           StateFile(home),
		"Backups":             Backups(home),
		"Cache":               Cache(home),
		"ModelVariantsCache":  ModelVariantsCache(home),
		"ReviewContexts":      ReviewContexts(home),
		"PiCodeGraphManifest": PiCodeGraphManifest(home),
	}
	for name, got := range cases {
		if !strings.HasPrefix(got, root+string(filepath.Separator)) {
			t.Errorf("%s(%q) = %q, which does not hang off root %q", name, home, got, root)
		}
		if got != filepath.Clean(got) {
			t.Errorf("%s(%q) = %q, which is not a cleaned path", name, home, got)
		}
	}
}

// TestNoDerivedPathMentionsLegacyName is the regression guard for the rename.
// internal/opencode/models_test.go carries the same shape for the earlier
// .cache/gentle-ai -> .gentle-ai move; this is its successor.
func TestNoDerivedPathMentionsLegacyName(t *testing.T) {
	home := t.TempDir()
	for name, got := range map[string]string{
		"Root":                Root(home),
		"StateFile":           StateFile(home),
		"Backups":             Backups(home),
		"Cache":               Cache(home),
		"ModelVariantsCache":  ModelVariantsCache(home),
		"ReviewContexts":      ReviewContexts(home),
		"PiCodeGraphManifest": PiCodeGraphManifest(home),
	} {
		if strings.Contains(got, LegacyDirName) {
			t.Errorf("%s(%q) = %q, which still contains the legacy root %q", name, home, got, LegacyDirName)
		}
	}
}

// The orphan check exists so an install from before the rename is reported
// rather than silently ignored. It keys on state.json, never on directory
// existence: three eager MkdirAll calls in this binary create the root as a
// side effect, so existence alone would fire on a fresh machine.

func writeStateFile(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %q: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, stateFileName), []byte(`{"default":"claude"}`), 0o644); err != nil {
		t.Fatalf("write state file in %q: %v", dir, err)
	}
}

func TestOrphanedLegacyStateDetectsPreRenameInstall(t *testing.T) {
	home := t.TempDir()
	writeStateFile(t, LegacyRoot(home))

	if !OrphanedLegacyState(home) {
		t.Fatal("legacy root holds a state file and the current root does not; want orphan reported")
	}
}

func TestOrphanedLegacyStateSilentWhenCurrentRootInhabited(t *testing.T) {
	home := t.TempDir()
	writeStateFile(t, LegacyRoot(home))
	writeStateFile(t, Root(home))

	if OrphanedLegacyState(home) {
		t.Fatal("current root already holds state; want no orphan warning")
	}
}

func TestOrphanedLegacyStateSilentOnFreshMachine(t *testing.T) {
	home := t.TempDir()

	if OrphanedLegacyState(home) {
		t.Fatal("neither root exists; want no orphan warning")
	}
}

// An empty legacy directory is not an install. uninstall.NewService and
// filemerge both MkdirAll their way into existence even on dry runs, so
// existence is a falsifiable signal and state.json is not.
func TestOrphanedLegacyStateIgnoresEmptyLegacyDirectory(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(LegacyRoot(home), "backups"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if OrphanedLegacyState(home) {
		t.Fatal("legacy directory exists but holds no state file; want no orphan warning")
	}
}

func TestOrphanedLegacyStateIsPure(t *testing.T) {
	home := t.TempDir()
	writeStateFile(t, LegacyRoot(home))

	for i := 0; i < 3; i++ {
		if !OrphanedLegacyState(home) {
			t.Fatalf("call %d disagreed with the previous ones", i+1)
		}
	}
	if _, err := os.Stat(Root(home)); !os.IsNotExist(err) {
		t.Fatalf("OrphanedLegacyState created %q; it must only read", Root(home))
	}
}

func TestLegacyAccessorsPointAtTheOldRoot(t *testing.T) {
	home := t.TempDir()
	wantRoot := filepath.Join(home, ".gentle-ai")
	if got := LegacyRoot(home); got != wantRoot {
		t.Fatalf("LegacyRoot(%q) = %q, want %q", home, got, wantRoot)
	}
	if got, want := LegacyStateFile(home), filepath.Join(wantRoot, "state.json"); got != want {
		t.Fatalf("LegacyStateFile(%q) = %q, want %q", home, got, want)
	}
}
