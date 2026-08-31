package cli

import (
	"strings"
	"testing"

	"bitbucket.org/hgt_development/hgtran-ai/v2/internal/verify"
)

// TestWithFailedSyncVerificationNoteNamesSyncCommand closes finding 1 of the
// install/sync surface audit for the sync path: the generic
// "Run repair on failed checks" text named a command that does not exist.
// The honest retry command for sync is simply `hgtran-ai sync`.
func TestWithFailedSyncVerificationNoteNamesSyncCommand(t *testing.T) {
	report := verify.Report{Ready: false, FinalNote: verify.VerificationIssuesMessage}

	updated := withFailedSyncVerificationNote(report)

	if strings.Contains(updated.FinalNote, "repair") {
		t.Fatalf("FinalNote still names the nonexistent repair command: %q", updated.FinalNote)
	}
	if !strings.Contains(updated.FinalNote, "hgtran-ai sync") {
		t.Fatalf("FinalNote = %q, want it to name `hgtran-ai sync`", updated.FinalNote)
	}
}

// TestWithFailedSyncVerificationNoteDoesNotOverrideACustomizedNote proves the
// override is scoped to exactly the generic production failure text.
func TestWithFailedSyncVerificationNoteDoesNotOverrideACustomizedNote(t *testing.T) {
	report := verify.Report{Ready: false, FinalNote: "custom failure text"}

	updated := withFailedSyncVerificationNote(report)
	if updated.FinalNote != "custom failure text" {
		t.Fatalf("FinalNote changed unexpectedly: %q", updated.FinalNote)
	}
}

// TestWithFailedSyncVerificationNoteLeavesReadyReportsAlone proves a Ready
// report is never touched.
func TestWithFailedSyncVerificationNoteLeavesReadyReportsAlone(t *testing.T) {
	report := verify.Report{Ready: true, FinalNote: verify.ReadyMessage}

	updated := withFailedSyncVerificationNote(report)
	if updated.FinalNote != verify.ReadyMessage {
		t.Fatalf("FinalNote changed unexpectedly: %q", updated.FinalNote)
	}
}
