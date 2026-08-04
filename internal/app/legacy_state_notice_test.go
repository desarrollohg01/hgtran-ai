package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bitbucket.org/hgt_development/hgtran-ai/v2/internal/statepath"
)

// The identity rename moved the state root. An install predating it keeps its
// data under the old root, where nothing reads it any more. Starting fresh and
// silently ignoring it is the one outcome worth ruling out: the user would see
// a first-install screen and conclude their settings and backups were lost.
//
// The notice never aborts. Aborting would brick `doctor`, which is the command
// someone would reach for to understand the problem.

func writeState(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %q: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte(`{"default":"claude"}`), 0o644); err != nil {
		t.Fatalf("write state in %q: %v", dir, err)
	}
}

func TestLegacyStateNoticeNamesBothRootsAndTheFix(t *testing.T) {
	home := t.TempDir()
	writeState(t, statepath.LegacyRoot(home))

	var out bytes.Buffer
	writeLegacyStateNotice(&out, home)

	got := out.String()
	if got == "" {
		t.Fatal("a pre-rename install was present; want a notice, got nothing")
	}
	for _, want := range []string{statepath.LegacyRoot(home), statepath.Root(home)} {
		if !strings.Contains(got, want) {
			t.Errorf("notice must name %q so the user can act on it; got:\n%s", want, got)
		}
	}
}

func TestLegacyStateNoticeSilentWhenCurrentRootInhabited(t *testing.T) {
	home := t.TempDir()
	writeState(t, statepath.LegacyRoot(home))
	writeState(t, statepath.Root(home))

	var out bytes.Buffer
	writeLegacyStateNotice(&out, home)

	if out.Len() != 0 {
		t.Fatalf("current root already holds state; want silence, got:\n%s", out.String())
	}
}

func TestLegacyStateNoticeSilentOnFreshMachine(t *testing.T) {
	home := t.TempDir()

	var out bytes.Buffer
	writeLegacyStateNotice(&out, home)

	if out.Len() != 0 {
		t.Fatalf("no install of either era; want silence, got:\n%s", out.String())
	}
}

// A directory alone is not an install: three MkdirAll calls in this binary
// create roots as a side effect, so existence would fire on a clean machine.
func TestLegacyStateNoticeIgnoresEmptyLegacyDirectory(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(statepath.LegacyRoot(home), "cache"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	var out bytes.Buffer
	writeLegacyStateNotice(&out, home)

	if out.Len() != 0 {
		t.Fatalf("legacy directory exists but holds no state; want silence, got:\n%s", out.String())
	}
}
