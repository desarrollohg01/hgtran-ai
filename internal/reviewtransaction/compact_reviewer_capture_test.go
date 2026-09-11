package reviewtransaction

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

type compactReviewerCaptureFixture struct {
	store   CompactStore
	state   CompactState
	request CompactAdmittedReviewerResultRequest
}

func captureAdmittedCorrectionFinding(t *testing.T, store CompactStore, state CompactState, finding Finding) LensResult {
	t.Helper()
	frozen, err := (SnapshotBuilder{Repo: store.repo}).FrozenCandidateContext(t.Context(), state.InitialSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	lens := state.SelectedLenses[0]
	subject, err := NewArtifactSubject(state, state.CapturePhaseRevision, frozen, lens, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	result := LensResult{Lens: lens, Findings: []Finding{finding}, Evidence: []string{"inspected the complete frozen candidate scope"}}
	inspection := ArtifactInspection{Status: ArtifactInspectionCompleted, Paths: append([]string(nil), state.InitialSnapshot.Paths...)}
	raw, err := json.Marshal(compactProviderReviewerResult{SubjectHash: subject.SubjectHash, Inspection: inspection, Lens: lens, Findings: result.Findings, Evidence: result.Evidence})
	if err != nil {
		t.Fatal(err)
	}
	capture, err := store.CaptureAdmittedReviewerResult(t.Context(), CompactAdmittedReviewerResultRequest{
		ExpectedRevision: state.CapturePhaseRevision, TargetIdentity: state.InitialSnapshot.Identity,
		FrozenContext: frozen, ArtifactSubject: subject, Inspection: inspection, Result: result,
		CandidateCausalFindingIDs: []string{finding.ID}, RawPayload: append(raw, '\n'),
	})
	if err != nil {
		t.Fatal(err)
	}
	return capture.LensResult
}

func newCompactReviewerCaptureFixture(t *testing.T, lineage string) compactReviewerCaptureFixture {
	t.Helper()
	repo := initSnapshotRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, "internal"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.go", "b.go"} {
		if err := os.WriteFile(filepath.Join(repo, "internal", name), []byte("package internal\n\nfunc Value() int { return 1 }\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitSnapshot(t, repo, "add", "--", "internal/a.go", "internal/b.go")
	gitSnapshot(t, repo, "commit", "-m", "add go fixture")
	for _, name := range []string{"a.go", "b.go"} {
		if err := os.WriteFile(filepath.Join(repo, "internal", name), []byte("package internal\n\nfunc Value() int { return 2 }\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	state := newCompactTestState(t, repo, lineage)
	if len(state.SelectedLenses) != 1 || state.SelectedLenses[0] != LensReliability {
		t.Fatalf("fixture lenses = %v", state.SelectedLenses)
	}
	store, err := CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace("", "review/start", state); err != nil {
		t.Fatal(err)
	}
	frozen, err := (SnapshotBuilder{Repo: repo}).FrozenCandidateContext(context.Background(), state.InitialSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	subject, err := NewArtifactSubject(state, state.CapturePhaseRevision, frozen, LensReliability, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	inspection := ArtifactInspection{Status: ArtifactInspectionCompleted, Paths: append([]string(nil), state.InitialSnapshot.Paths...)}
	result := LensResult{Lens: LensReliability, Findings: []Finding{}, Evidence: []string{"inspected internal/a.go:1 against the complete frozen candidate"}}
	raw, err := json.Marshal(compactProviderReviewerResult{SubjectHash: subject.SubjectHash, Inspection: inspection, Lens: subject.Lens, Findings: result.Findings, Evidence: result.Evidence})
	if err != nil {
		t.Fatal(err)
	}
	return compactReviewerCaptureFixture{
		store: store, state: state,
		request: CompactAdmittedReviewerResultRequest{
			ExpectedRevision: state.CapturePhaseRevision, TargetIdentity: state.InitialSnapshot.Identity,
			FrozenContext: frozen, ArtifactSubject: subject, Inspection: inspection, Result: result,
			CandidateCausalFindingIDs: []string{}, RawPayload: append(raw, '\n'),
		},
	}
}

// TestCompactReviewerResultSidecarOwnersAreAbsent prevents the retired result
// directory from becoming a lifecycle owner again. Lens result bytes and their
// digests live only in CompactState.AdmittedRoleResults.
func TestCompactReviewerResultSidecarOwnersAreAbsent(t *testing.T) {
	for _, source := range []string{
		"compact_store.go",
		"compact_reclaim.go",
		filepath.Join("..", "cli", "review_opencode_transport.go"),
	} {
		payload, err := os.ReadFile(source)
		if err != nil {
			t.Fatalf("read production owner %s: %v", source, err)
		}
		for _, forbidden := range []string{
			"CompactReviewerResultsDir", "reviewer-results", "reviewResultArtifactPath",
			"CompactIncidentsDir", "EnsureCompactIncidentsDir", "ResultDispositions",
		} {
			if strings.Contains(string(payload), forbidden) {
				t.Fatalf("retired compact result owner %q remains in %s", forbidden, source)
			}
		}
	}
	if _, err := os.Stat("compact_result_disposition.go"); !os.IsNotExist(err) {
		t.Fatalf("retired compact result disposition owner remains: %v", err)
	}
}

func TestApprovedAcknowledgementHasNoImmediateBurnOrSidecarRevival(t *testing.T) {
	root := filepath.Join("..", "..")
	for _, source := range []string{
		filepath.Join(root, "internal", "cli", "review_facade.go"),
		filepath.Join(root, "internal", "cli", "review_last_event_closure.go"),
		filepath.Join(root, "internal", "cli", "review_next_transition.go"),
		filepath.Join(root, "internal", "cli", "review_start_contract.go"),
		filepath.Join(root, "internal", "cli", "review_status_contract.go"),
		filepath.Join(root, "internal", "reviewtransaction", "compact_burn.go"),
		filepath.Join(root, "scripts", "crosslane", "battery.go"),
		filepath.Join(root, "bench", "journeys_wave3.go"),
		filepath.Join(root, "bench", "journeys_provider_capture.go"), filepath.Join(root, "e2e", "organicruntime", "organic_runtime_test.go"),
	} {
		payload, err := os.ReadFile(source)
		if err != nil {
			t.Fatalf("read current acknowledgement owner %s: %v", source, err)
		}
		for _, forbidden := range []string{"BurnApprovedCompactAuthority(", "IssueApprovedCompactAcknowledgement(", "burnApproved(", "requireAtomicLineageBurned", "requireBurnedApproval", "PublishReviewRepositoryContext", "CompactReviewerResultsDir", "EffectIntents", "lens-contexts", "rctx1_"} {
			if strings.Contains(string(payload), forbidden) {
				t.Fatalf("current acknowledgement owner %s revived forbidden %q", source, forbidden)
			}
		}
	}
	for _, schema := range []string{
		filepath.Join(root, "contracts", "review-integration", "v2", "schemas", "start.schema.json"),
		filepath.Join(root, "contracts", "review-integration", "v2", "schemas", "status-v5.schema.json"),
		filepath.Join(root, "contracts", "review-integration", "v2", "schemas", "last-event-closure.schema.json"),
	} {
		payload, err := os.ReadFile(schema)
		if err != nil {
			t.Fatalf("read acknowledgement schema %s: %v", schema, err)
		}
		for _, forbidden := range []string{"review-integration/v3", "consent/v4", "consent-v4"} {
			if strings.Contains(string(payload), forbidden) {
				t.Fatalf("acknowledgement schema %s expanded prohibited protocol %q", schema, forbidden)
			}
		}
	}
}

func TestCorrectionPlanRequestUsesAdmittedCaptureOverLegacyProjections(t *testing.T) {
	fixture := newCompactReviewerCaptureFixture(t, "correction-plan-admitted-capture")
	finding := Finding{
		ID: "R3-001", Lens: strings.TrimPrefix(fixture.state.SelectedLenses[0], "review-"), Location: "internal/a.go:1", Severity: "CRITICAL",
		Claim: "candidate needs correction", ProofRefs: []string{"changed hunk causes failure"}, EvidenceClass: EvidenceDeterministic, CausalDisposition: CausalIntroduced,
	}
	result := captureAdmittedCorrectionFinding(t, fixture.store, fixture.state, finding)
	record, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	state := record.State
	if err := state.CompleteReview(CompactReviewInput{
		LensResults:     []LensResult{result},
		Classifications: []FindingEvidence{{FindingID: finding.ID, Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "changed hunk causes failure"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := state.BeginCorrection(1); err != nil {
		t.Fatal(err)
	}
	want, err := BuildCorrectionPlanRequest(state, state.CapturePhaseRevision)
	if err != nil {
		t.Fatal(err)
	}

	// Retired projections cannot be reintroduced into CompactState; the read must
	// remain entirely derived from the canonical admitted capture.
	got, err := BuildCorrectionPlanRequest(state, state.CapturePhaseRevision)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("correction plan from tampered legacy projections = %#v, want admitted capture %#v", got, want)
	}
}

func TestCompactStoreCaptureAdmittedReviewerResultPublishesOneRecordExactReplay(t *testing.T) {
	fixture := newCompactReviewerCaptureFixture(t, "native-admitted-reviewer")
	first, err := fixture.store.CaptureAdmittedReviewerResult(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	record, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !first.Slot.Occupied || len(record.State.AdmittedRoleResults) != 1 || record.State.AdmittedRoleResults[0].ArtifactDigest != first.Slot.Digest {
		t.Fatalf("capture did not persist one canonical role value: %#v", record)
	}
	replayed, err := fixture.store.CaptureAdmittedReviewerResult(context.Background(), fixture.request)
	if err != nil {
		t.Fatalf("exact replay: %v", err)
	}
	after, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Slot.Digest != first.Slot.Digest || after.Revision != record.Revision {
		t.Fatalf("exact replay changed authority: before=%#v after=%#v", record, after)
	}
}

func TestCompactStoreCaptureAdmittedReviewerResultRefusesStalePhaseWithoutMutation(t *testing.T) {
	fixture := newCompactReviewerCaptureFixture(t, "native-admitted-stale")
	before, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	request := fixture.request
	request.ExpectedRevision = hash("a")
	request.ArtifactSubject.AuthorityRevision = request.ExpectedRevision
	if _, err := fixture.store.CaptureAdmittedReviewerResult(context.Background(), request); err == nil {
		t.Fatal("stale capture phase was accepted")
	}
	after, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != before.Revision || len(after.State.AdmittedRoleResults) != 0 {
		t.Fatalf("stale phase mutated authority: before=%#v after=%#v", before, after)
	}
}

func TestCompactStoreMergesRefuterTupleAndReplaysWithoutAWrite(t *testing.T) {
	fixture := newCompactReviewerCaptureFixture(t, "record-refuter-capture")
	if _, err := fixture.store.CaptureAdmittedReviewerResult(t.Context(), fixture.request); err != nil {
		t.Fatal(err)
	}
	current, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	request := CompactAdmittedRefuterResultRequest{
		ExpectedRevision: current.State.CapturePhaseRevision, TargetIdentity: current.State.InitialSnapshot.Identity,
		RequestHash: hash("b"), Payload: []byte(`{"results":[]}`),
	}
	if err := fixture.store.CaptureAdmittedRefuterResult(t.Context(), request); err != nil {
		t.Fatal(err)
	}
	merged, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(merged.State.AdmittedRoleResults) != 2 {
		t.Fatalf("record values = %d, want lens plus refuter", len(merged.State.AdmittedRoleResults))
	}
	if err := fixture.store.CaptureAdmittedRefuterResult(t.Context(), request); err != nil {
		t.Fatalf("refuter replay: %v", err)
	}
	replayed, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Revision != merged.Revision {
		t.Fatal("refuter exact replay wrote a successor")
	}
}

func TestReopenedRefuterWithRetiredPayloadRequiresCurrentPhase(t *testing.T) {
	repo, store, state := highRiskCaptureAuthority(t, "reopen-refuter-current-phase")
	for order := range state.SelectedLenses {
		captureCompactLens(t, store, state, order)
	}
	initial := requireCompactRoleCount(t, store, 4)
	request := CompactAdmittedRefuterResultRequest{ExpectedRevision: initial.State.CapturePhaseRevision, TargetIdentity: initial.State.InitialSnapshot.Identity, RequestHash: hash("c"), Payload: []byte(`{"results":[]}`)}
	if err := store.CaptureAdmittedRefuterResult(t.Context(), request); err != nil {
		t.Fatal(err)
	}
	initial = requireCompactRoleCount(t, store, 5)
	retiredDigest := ""
	for _, entry := range initial.State.AdmittedRoleResults {
		if entry.Role == CompactRoleRefuter {
			retiredDigest = entry.ArtifactDigest
		}
	}
	reopened := reopenOneCapturedLens(t, repo, store, initial, LensRisk)
	captureCompactLens(t, store, reopened.State, 0)
	current := requireCompactRoleCount(t, store, 4)
	request.ExpectedRevision = current.State.CapturePhaseRevision
	if err := store.CaptureAdmittedRefuterResult(t.Context(), request); err != nil {
		t.Fatalf("fresh refuter with retired payload: %v", err)
	}
	current = requireCompactRoleCount(t, store, 5)
	currentDigest := ""
	for _, entry := range current.State.AdmittedRoleResults {
		if entry.Role == CompactRoleRefuter && entry.CapturePhaseRevision == current.State.CapturePhaseRevision {
			currentDigest = entry.ArtifactDigest
		}
	}
	if _, found := current.State.AdmittedRoleResult(CompactRoleRefuter, current.State.CapturePhaseRevision, current.State.InitialSnapshot.Identity, request.RequestHash); !found || currentDigest != retiredDigest {
		t.Fatalf("fresh current-phase refuter was not admitted from matching bytes: found=%t digest=%q", found, currentDigest)
	}
	beforeReplay := current.Revision
	if err := store.CaptureAdmittedRefuterResult(t.Context(), request); err != nil {
		t.Fatalf("current-phase exact replay: %v", err)
	}
	if replay := requireCompactRoleCount(t, store, 5); replay.Revision != beforeReplay {
		t.Fatal("current-phase exact refuter replay wrote authority")
	}
	request.ExpectedRevision = initial.State.CapturePhaseRevision
	if err := store.CaptureAdmittedRefuterResult(t.Context(), request); err == nil {
		t.Fatal("stale prior refuter phase satisfied the current slot")
	}
	if stale := requireCompactRoleCount(t, store, 5); stale.Revision != beforeReplay {
		t.Fatal("stale prior refuter phase replay mutated authority")
	}
}

// TestCompactStoreCaptureAdmittedReviewerResultConvergesAfterExactReplayLockTimeout
// is intentionally NOT restored. In dev's filesystem-artifact architecture,
// exact replay could be satisfied by re-reading an immutable sidecar file
// without the store write lock, so a replay converged even while another
// holder had the store lock. In the current architecture, admitted reviewer
// results live inside CompactState itself (see mergeAdmittedLensResult), so
// even a no-op exact replay must acquire the same store lock as a real
// mutation to read state safely. Empirically re-running this scenario against
// current code (fixture captures once, an external holder takes
// store.lockPath, then a replay is attempted) reliably returns
// *AuthorityLockTimeoutError after the 2s maintenanceLockTimeout instead of
// converging. This is not a regression: it is the direct, intended
// consequence of moving reviewer-result durability from a lock-free
// filesystem artifact into the same CAS'd, lock-protected state file as every
// other compact mutation. Weakening the assertion to expect a timeout would
// not guard any real invariant, so this test is left dropped rather than
// restored.

// TestCompactStoreResolveAdmittedReviewerResultIsExactAndReadOnly restores dev's
// read-only coverage for ResolveAdmittedReviewerResult, retargeted from the
// retired filesystem artifact/digest sidecar pair onto the sole current
// persistence surface: the compact state file itself.
func TestCompactStoreResolveAdmittedReviewerResultIsExactAndReadOnly(t *testing.T) {
	fixture := newCompactReviewerCaptureFixture(t, "resolve-native-admitted-reviewer")
	stateBefore, err := os.ReadFile(fixture.store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	missing, found, err := fixture.store.ResolveAdmittedReviewerResult(
		context.Background(),
		fixture.request.ExpectedRevision,
		fixture.request.TargetIdentity,
		fixture.request.FrozenContext,
		fixture.request.ArtifactSubject,
	)
	if err != nil || found || !reflect.DeepEqual(missing, LensResult{}) {
		t.Fatalf("missing admitted result = %#v, %t, %v", missing, found, err)
	}
	stateAfterMiss, err := os.ReadFile(fixture.store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(stateBefore, stateAfterMiss) {
		t.Fatal("read-only miss mutated compact authority")
	}

	want, err := fixture.store.CaptureAdmittedReviewerResult(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	stateAfterCapture, err := os.ReadFile(fixture.store.StatePath())
	if err != nil {
		t.Fatal(err)
	}

	got, found, err := fixture.store.ResolveAdmittedReviewerResult(
		context.Background(),
		fixture.request.ExpectedRevision,
		fixture.request.TargetIdentity,
		fixture.request.FrozenContext,
		fixture.request.ArtifactSubject,
	)
	if err != nil || !found || !reflect.DeepEqual(got, want.LensResult) {
		t.Fatalf("resolved admitted result = %#v, %t, %v", got, found, err)
	}
	stateAfterResolve, err := os.ReadFile(fixture.store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(stateAfterCapture, stateAfterResolve) {
		t.Fatal("resolver changed compact authority")
	}
}

// TestCompactStoreResolveAdmittedReviewerResultFailsClosed restores dev's
// fail-closed coverage for ResolveAdmittedReviewerResult. The mismatched-target
// and tampered-frozen-context cases are unchanged; the third case (a tampered
// artifact) is retargeted from a hand-edited sidecar file onto a hand-edited
// admitted role value inside CompactState, since that is where the bytes now
// live.
func TestCompactStoreResolveAdmittedReviewerResultFailsClosed(t *testing.T) {
	fixture := newCompactReviewerCaptureFixture(t, "resolve-admitted-reviewer-refusal")
	if _, err := fixture.store.CaptureAdmittedReviewerResult(context.Background(), fixture.request); err != nil {
		t.Fatal(err)
	}
	if _, found, err := fixture.store.ResolveAdmittedReviewerResult(
		context.Background(),
		fixture.request.ExpectedRevision,
		verificationTestHash("different-review-target"),
		fixture.request.FrozenContext,
		fixture.request.ArtifactSubject,
	); err == nil || found {
		t.Fatalf("mismatched target = found %t, error %v", found, err)
	}
	tamperedFrozen := fixture.request.FrozenContext
	tamperedFrozen.ChangedPathManifest = append(
		[]ChangedPathManifestEntry(nil),
		tamperedFrozen.ChangedPathManifest...,
	)
	tamperedFrozen.ChangedPathManifest[0].ModeOnly = true
	if _, found, err := fixture.store.ResolveAdmittedReviewerResult(
		context.Background(),
		fixture.request.ExpectedRevision,
		fixture.request.TargetIdentity,
		tamperedFrozen,
		fixture.request.ArtifactSubject,
	); err == nil || found {
		t.Fatalf("tampered frozen context = found %t, error %v", found, err)
	}

	record, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	tampered := append([]CompactAdmittedRoleResult(nil), record.State.AdmittedRoleResults...)
	found := false
	for index, entry := range tampered {
		if entry.Role != CompactRoleLens {
			continue
		}
		mutated := append([]byte(nil), bytes.TrimSuffix(entry.Value, []byte("}"))...)
		mutated = append(mutated, []byte(`,"unexpected":true}`)...)
		entry.Value = mutated
		entry.ArtifactDigest = compactPreservedPayloadDigest(append(append([]byte(nil), mutated...), '\n'))
		tampered[index] = entry
		found = true
	}
	if !found {
		t.Fatal("fixture has no admitted lens result to tamper")
	}
	next := cloneCompactStateInitialAtomicStart(record.State)
	next.AdmittedRoleResults = tampered
	_, payload, err := makeCompactRecord(next)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeAtomic(fixture.store.StatePath(), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, found, err := fixture.store.ResolveAdmittedReviewerResult(
		context.Background(),
		fixture.request.ExpectedRevision,
		fixture.request.TargetIdentity,
		fixture.request.FrozenContext,
		fixture.request.ArtifactSubject,
	); err == nil || found {
		t.Fatalf("unknown-field admitted value = found %t, error %v", found, err)
	}
}

// TestCompactStoreCaptureAdmittedReviewerResultRejectsUnsafeOrConflictingSlots
// restores dev's conflicting-slot coverage. The original table also exercised
// symlinked, hardlinked, and group-readable reviewer-result files under a RAR
// safety directory; that whole mechanism is retired along with the standalone
// reviewer-result artifact/digest sidecar (see
// TestCompactReviewerResultSidecarOwnersAreAbsent), so those cases have no
// remaining production counterpart. What survives, and still had zero direct
// coverage, is ErrCapturedReviewerResultSlotConflict itself: a second capture
// for an already-occupied lens slot with different canonical bytes must be
// refused without mutating compact authority.
func TestCompactStoreCaptureAdmittedReviewerResultRejectsUnsafeOrConflictingSlots(t *testing.T) {
	fixture := newCompactReviewerCaptureFixture(t, "capture-refusal-conflicting-slot")
	if _, err := fixture.store.CaptureAdmittedReviewerResult(context.Background(), fixture.request); err != nil {
		t.Fatal(err)
	}
	before, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	conflicting := fixture.request
	conflicting.RawPayload = append([]byte("different transport\n"), fixture.request.RawPayload...)
	if _, err := fixture.store.CaptureAdmittedReviewerResult(context.Background(), conflicting); !errors.Is(err, ErrCapturedReviewerResultSlotConflict) {
		t.Fatalf("conflicting capture error = %v, want %v", err, ErrCapturedReviewerResultSlotConflict)
	}
	after, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != before.Revision || !reflect.DeepEqual(after.State.AdmittedRoleResults, before.State.AdmittedRoleResults) {
		t.Fatal("rejected conflicting capture mutated compact authority")
	}
}

// TestCompactStoreCaptureAdmittedReviewerResultRejectsCallerDerivedContext
// restores dev's coverage of caller-derived (rather than repository-derived)
// frozen context, dropping only the filesystem existence check for the retired
// reviewer-result directory.
func TestCompactStoreCaptureAdmittedReviewerResultRejectsCallerDerivedContext(t *testing.T) {
	fixture := newCompactReviewerCaptureFixture(t, "capture-rederive-frozen-context")
	tampered := fixture.request
	tampered.FrozenContext.ChangedPathManifest = append(
		[]ChangedPathManifestEntry(nil),
		tampered.FrozenContext.ChangedPathManifest...,
	)
	tampered.FrozenContext.ChangedPathManifest[0].ModeOnly = true
	if _, err := fixture.store.CaptureAdmittedReviewerResult(context.Background(), tampered); err == nil || !strings.Contains(
		err.Error(),
		"does not match repository authority",
	) {
		t.Fatalf("caller-derived frozen context error = %v", err)
	}
	after, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(after.State.AdmittedRoleResults) != 0 {
		t.Fatal("context refusal mutated compact authority")
	}
}

// TestCompactStoreCaptureAdmittedReviewerResultSerializesConcurrentReplayAndConflict
// restores dev's concurrency coverage for exact replay and slot conflict,
// retargeted from the retired filesystem artifact readback onto CompactState.
func TestCompactStoreCaptureAdmittedReviewerResultSerializesConcurrentReplayAndConflict(t *testing.T) {
	t.Run("exact replay", func(t *testing.T) {
		fixture := newCompactReviewerCaptureFixture(t, "capture-concurrent-replay")
		const attempts = 8
		var wait sync.WaitGroup
		errorsByAttempt := make([]error, attempts)
		hashes := make([]string, attempts)
		for index := 0; index < attempts; index++ {
			wait.Add(1)
			go func(index int) {
				defer wait.Done()
				result, err := fixture.store.CaptureAdmittedReviewerResult(
					context.Background(),
					fixture.request,
				)
				errorsByAttempt[index] = err
				hashes[index] = result.ResultHash
			}(index)
		}
		wait.Wait()
		for index := range errorsByAttempt {
			if errorsByAttempt[index] != nil ||
				hashes[index] == "" ||
				hashes[index] != hashes[0] {
				t.Fatalf(
					"concurrent replay[%d] = hash %q, error %v",
					index,
					hashes[index],
					errorsByAttempt[index],
				)
			}
		}
		record, err := fixture.store.Load()
		if err != nil {
			t.Fatal(err)
		}
		if len(record.State.AdmittedRoleResults) != 1 {
			t.Fatalf("concurrent exact replay produced %d admitted role results, want 1", len(record.State.AdmittedRoleResults))
		}
	})

	t.Run("different raw authority", func(t *testing.T) {
		fixture := newCompactReviewerCaptureFixture(t, "capture-concurrent-conflict")
		alternate := fixture.request
		alternate.RawPayload = append(
			[]byte("review transport prefix\n"),
			alternate.RawPayload...,
		)
		requests := []CompactAdmittedReviewerResultRequest{
			fixture.request,
			alternate,
		}
		var wait sync.WaitGroup
		results := make([]error, len(requests))
		for index := range requests {
			wait.Add(1)
			go func(index int) {
				defer wait.Done()
				_, results[index] = fixture.store.CaptureAdmittedReviewerResult(
					context.Background(),
					requests[index],
				)
			}(index)
		}
		wait.Wait()
		successes, conflicts := 0, 0
		for _, err := range results {
			switch {
			case err == nil:
				successes++
			case strings.Contains(
				err.Error(),
				"different canonical bytes",
			):
				conflicts++
			default:
				t.Fatalf("concurrent conflict error = %v", err)
			}
		}
		if successes != 1 || conflicts != 1 {
			t.Fatalf(
				"concurrent conflict = %d success, %d conflict",
				successes,
				conflicts,
			)
		}
	})
}
