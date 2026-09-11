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
	"syscall"
	"testing"
	"time"
)

func TestActiveCompactRecordOmitsResultDispositionsAndStablyDigestsAdmittedRoles(t *testing.T) {
	repo := initSnapshotRepo(t)
	record := CompactRecord{Schema: compactRecordSchema, Revision: hash("active-compact-record"), State: newCompactTestState(t, repo, "active-compact-record")}

	// The reflection guard makes a future reintroduction of the retired persisted
	// field observable without coupling this post-deletion test to its Go type.
	if field := reflect.ValueOf(&record.State).Elem().FieldByName("ResultDispositions"); field.IsValid() {
		field.Set(reflect.MakeSlice(field.Type(), 1, 1))
	}
	payload, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	var serialized struct {
		State map[string]json.RawMessage `json:"state"`
	}
	if err := json.Unmarshal(payload, &serialized); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{
		"lens_results", "findings", "classifications", "outcomes", "follow_ups", "result_dispositions",
	} {
		if _, found := serialized.State[key]; found {
			t.Fatalf("new compact record serialized retired %s state", key)
		}
	}

	const wantDigest = "sha256:ca3d163bab055381827226140568f3bef7eaac187cebd76878e0b63e9e442356"
	if got := compactPreservedPayloadDigest([]byte("{}\n")); got != wantDigest {
		t.Fatalf("admitted-role payload digest = %q, want %q", got, wantDigest)
	}
}

func TestCompactHistoricalFailedValidatorRequiresLocalAttempt(t *testing.T) {
	state := CompactState{
		State: StateCorrectionRequired,
		Recovery: &CompactRecoveryProvenance{
			ConsumedCorrectionAttempts: MaxCompactCorrectionAttempts,
		},
	}
	if compactHistoricalFailedValidator(state) {
		t.Fatal("recovery accounting without a local correction attempt must not be treated as a historical failed validator")
	}
}

func TestNewCompactStateDerivesNonCircularCapturePhaseBeforeRecordCreation(t *testing.T) {
	repo := initSnapshotRepo(t)
	state := newCompactTestState(t, repo, "capture-phase-before-record")
	if !validSHA256(state.CapturePhaseRevision) {
		t.Fatalf("capture phase revision = %q, want canonical SHA-256", state.CapturePhaseRevision)
	}

	preimage := state
	preimage.CapturePhaseRevision = ""
	derived, err := deriveCompactCapturePhaseRevision(preimage)
	if err != nil {
		t.Fatal(err)
	}
	if derived != state.CapturePhaseRevision {
		t.Fatalf("derived capture phase = %q, want %q", derived, state.CapturePhaseRevision)
	}

	record, _, err := makeCompactRecord(state)
	if err != nil {
		t.Fatal(err)
	}
	if record.Revision == state.CapturePhaseRevision {
		t.Fatal("live record revision must remain distinct from the stable capture phase")
	}
}

func TestCompactCapturePhasePreimageIncludesAtomicWorktreeIdentity(t *testing.T) {
	repo := initSnapshotRepo(t)
	state := newCompactTestState(t, repo, "capture-phase-worktree")
	binding := CompactAtomicStartBinding{
		LineageID: state.LineageID, WorktreeIdentity: hash("worktree-one"),
		TargetIdentity: state.InitialSnapshot.Identity, Selector: Target{Kind: state.InitialSnapshot.Kind},
		PolicyHash: state.PolicyHash, Tier: state.RiskLevel, SelectedLenses: append([]string(nil), state.SelectedLenses...),
		OriginalChangedLines: state.OriginalChangedLines, CorrectionBudget: state.CorrectionBudget,
		CorrectionBudgetPolicy: state.CorrectionBudgetPolicy,
	}
	state.InitialAtomicStart = &binding
	first, err := deriveCompactCapturePhaseRevision(state)
	if err != nil {
		t.Fatal(err)
	}
	state.InitialAtomicStart.WorktreeIdentity = hash("worktree-two")
	second, err := deriveCompactCapturePhaseRevision(state)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("capture phase did not bind the frozen atomic START worktree identity")
	}
}

func TestCompactRecordBoundsRefuseMaxPlusOneBeforePersistence(t *testing.T) {
	entry := CompactAdmittedRoleResult{Value: json.RawMessage(`{}`)}
	tooMany := make([]CompactAdmittedRoleResult, compactMaxAdmittedRoleResults+1)
	for index := range tooMany {
		tooMany[index] = entry
	}
	if err := validateCompactRoleResultBounds(tooMany); err == nil {
		t.Fatal("accepted more than six admitted role values")
	}

	tooLargeRole := entry
	tooLargeRole.Value = make(json.RawMessage, compactReviewerResultSizeLimit+1)
	if err := validateCompactRoleResultBounds([]CompactAdmittedRoleResult{tooLargeRole}); err == nil {
		t.Fatal("accepted a role value above the current four MiB limit")
	}

	repo := initSnapshotRepo(t)
	state := newCompactTestState(t, repo, "compact-non-role-bound")
	state.InvalidationReason = strings.Repeat("x", compactNonRoleStateSizeLimit)
	if err := validateCompactNonRoleStateBounds(state); err == nil {
		t.Fatal("accepted non-role compact state at the seven MiB max-plus-one boundary")
	}
	if err := validateCompactRecordWritePayload(make([]byte, compactRecordSizeLimit+1)); err == nil {
		t.Fatal("accepted compact record bytes above the 32 MiB write limit")
	}
}

func TestCompactTargetedValidatorAttemptLedgerReplaysAndRefusesFourthWithoutMutation(t *testing.T) {
	repo, _, _, store := targetedValidationRequestFixture(t, "targeted-validator-attempt-ledger", true)
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	fix, err := (SnapshotBuilder{Repo: repo}).Build(t.Context(), Target{
		Kind: TargetFixDiff, Projection: record.State.InitialSnapshot.Projection,
		BaseRef: record.State.CurrentSnapshot.CandidateTree, IntendedUntracked: record.State.InitialSnapshot.IntendedUntracked,
		LedgerIDs: record.State.FixFindingIDs,
	})
	if err != nil {
		t.Fatal(err)
	}
	request, err := BuildTargetedValidationRequestFromSnapshot(t.Context(), repo, record.State, record.State.CapturePhaseRevision, fix)
	if err != nil {
		t.Fatal(err)
	}
	for _, digest := range []string{hash("a"), hash("b"), hash("c")} {
		replayed, err := store.RecordInconclusiveTargetedValidatorAttempt(t.Context(), request, digest)
		if err != nil || replayed {
			t.Fatalf("record distinct validator attempt %q: replayed=%t err=%v", digest, replayed, err)
		}
	}
	restarted, err := CompactAuthoritativeStore(t.Context(), repo, record.State.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := restarted.RecordInconclusiveTargetedValidatorAttempt(t.Context(), request, hash("c"))
	if err != nil || !replayed {
		t.Fatalf("exact validator replay after restart: replayed=%t err=%v", replayed, err)
	}
	after, err := restarted.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(after.State.TargetedValidatorAttempts) != 3 {
		t.Fatalf("validator attempt count = %d, want 3", len(after.State.TargetedValidatorAttempts))
	}
	beforeFourth := after.Revision
	if _, err := store.RecordInconclusiveTargetedValidatorAttempt(t.Context(), request, hash("d")); !errors.Is(err, ErrCompactTargetedValidatorAttemptsExhausted) {
		t.Fatalf("fourth validator attempt error = %v, want exhaustion", err)
	}
	afterFourth, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if afterFourth.Revision != beforeFourth || len(afterFourth.State.TargetedValidatorAttempts) != 3 {
		t.Fatal("fourth validator attempt mutated the authority")
	}
}

func TestCompactStateValidatesCanonicalAdmittedLensBindings(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	state := newCompactTestState(t, repo, "canonical-admitted-lens-bindings")
	state, store := startReviewingCompactAuthority(t, repo, state)
	captureCompactLens(t, store, state, 0)
	record := requireCompactRoleCount(t, store, 1)
	state = record.State
	entry := state.AdmittedRoleResults[0]
	if err := state.Validate(); err != nil {
		t.Fatalf("valid admitted lens binding: %v", err)
	}

	state.AdmittedRoleResults = append(state.AdmittedRoleResults, entry)
	if err := state.Validate(); err == nil {
		t.Fatal("accepted duplicate admitted lens tuple")
	}
}

func newCompactTestState(t *testing.T, repo, lineage string) CompactState {
	return newCompactTestStateWithIntended(t, repo, lineage, []string{})
}

func newCompactTestStateWithIntended(t *testing.T, repo, lineage string, intended []string) CompactState {
	t.Helper()
	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetCurrentChanges, IntendedUntracked: intended})
	if err != nil {
		t.Fatal(err)
	}
	risk, lines, err := (SnapshotBuilder{Repo: repo}).ClassifySnapshotRisk(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	lenses := []string{}
	if risk == RiskMedium {
		lenses = []string{LensReliability}
	} else if risk == RiskHigh {
		lenses = append([]string(nil), supportedLenses...)
	}
	state, err := NewCompactState(Start{
		LineageID: lineage, Mode: ModeOrdinaryBounded, Generation: 1, Snapshot: snapshot,
		PolicyHash: hash("1"), RiskLevel: risk, SelectedLenses: lenses, OriginalChangedLines: &lines,
	})
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func pendingCompactCorrection(t *testing.T, repo, lineage string) (CompactState, Snapshot) {
	t.Helper()
	writeSnapshotFile(t, repo, "tracked.txt", "base\none\ntwo\nthree\nfour\n")
	state := newCompactTestState(t, repo, lineage)
	store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace("", "review/start", state); err != nil {
		t.Fatal(err)
	}
	finding := Finding{
		ID: "R3-001", Lens: strings.TrimPrefix(state.SelectedLenses[0], "review-"), Location: "tracked.txt:5", Severity: "CRITICAL",
		Claim: "wrong value", ProofRefs: []string{"candidate-only failure"},
		EvidenceClass: EvidenceDeterministic, CausalDisposition: CausalIntroduced,
	}
	state, _ = captureAndCompleteCompactReview(t, store, state, CompactReviewInput{
		LensResults:     []LensResult{{Lens: state.SelectedLenses[0], Findings: []Finding{finding}, Evidence: []string{"reviewed once"}}},
		Classifications: []FindingEvidence{{FindingID: finding.ID, Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "changed hunk"}}, RefuterOutcomes: []EvidenceResult{},
	})
	if err := state.BeginCorrection(1); err != nil {
		t.Fatal(err)
	}
	writeSnapshotFile(t, repo, "tracked.txt", "base\none\ntwo\nthree\nfixed\n")
	fix, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetFixDiff, BaseRef: state.CurrentSnapshot.CandidateTree, IntendedUntracked: state.InitialSnapshot.IntendedUntracked, LedgerIDs: state.FixFindingIDs})
	if err != nil {
		t.Fatal(err)
	}
	return state, fix
}

func correctedCompactTestState(t *testing.T, repo, lineage string) CompactState {
	return correctedCompactTestStateWithIntended(t, repo, lineage, []string{})
}

func correctedCompactTestStateWithIntended(t *testing.T, repo, lineage string, intended []string) CompactState {
	t.Helper()
	writeSnapshotFile(t, repo, "tracked.txt", "base\none\ntwo\nthree\nfour\n")
	for _, path := range intended {
		writeSnapshotFile(t, repo, path, "initial intended content\n")
	}
	state := newCompactTestStateWithIntended(t, repo, lineage, intended)
	store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace("", "review/start", state); err != nil {
		t.Fatal(err)
	}
	finding := Finding{
		ID: "R3-001", Lens: "reliability", Location: "tracked.txt:5", Severity: "CRITICAL",
		Claim: "candidate returns the wrong terminal value", ProofRefs: []string{"differential test fails only on candidate"},
		EvidenceClass: EvidenceDeterministic, CausalDisposition: CausalIntroduced,
	}
	result := LensResult{Lens: LensReliability, Findings: []Finding{finding}, Evidence: []string{"focused differential test failed"}}
	state, _ = captureAndCompleteCompactReview(t, store, state, CompactReviewInput{
		LensResults:     []LensResult{result},
		Classifications: []FindingEvidence{{FindingID: finding.ID, Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "changed hunk causes the failure"}},
		RefuterOutcomes: []EvidenceResult{},
	})
	if err := state.BeginCorrection(2); err != nil {
		t.Fatal(err)
	}
	writeSnapshotFile(t, repo, "tracked.txt", "base\none\ntwo\nthree\nfixed\n")
	fix, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
		Kind: TargetFixDiff, BaseRef: state.InitialSnapshot.CandidateTree,
		IntendedUntracked: state.InitialSnapshot.IntendedUntracked, LedgerIDs: state.FixFindingIDs,
	})
	if err != nil {
		t.Fatal(err)
	}
	fixHash := FixDeltaHashForSnapshot(fix)
	validation := ScopedValidationResult{
		LedgerIDs: state.FixFindingIDs, FixCausedFindings: []Finding{}, FollowUps: []FollowUp{},
		OriginalCriteria:     ValidationCheck{EvidenceHash: hash("2"), FixDeltaHash: fixHash, Passed: true},
		CorrectionRegression: ValidationCheck{EvidenceHash: hash("3"), FixDeltaHash: fixHash, Passed: true},
	}
	if err := state.CompleteCorrectionVerification(fix, 2, bindTargetedValidationForTest(validation, fix)); err != nil {
		t.Fatal(err)
	}
	return state
}

func bindTargetedValidationForTest(validation ScopedValidationResult, fix Snapshot) ScopedValidationResult {
	validation.TargetedValidationRequestHash = hash("9")
	validation.CorrectionTargetIdentity = fix.Identity
	return validation
}

// --- restored from dev's pre-merge compact_store_test.go ---
//
// The upstream v2.5.0 merge replaced this package wholesale. The production
// behavior below is still live; only its test coverage was lost. Several
// restorations are adapted for two confirmed architecture changes since dev:
//
//  1. Reviewer results (and now the correction/refuter/targeted-validator role
//     values too) are no longer written as separate receipt/artifact files.
//     CompactReceipt, WriteCompactReceiptAtomic, ReceiptPath, state.Receipt(),
//     and CompactTransport.Receipt are all retired -- confirmed absent from
//     production by this package's own
//     TestCompactReceiptAndGateAPIsAreAbsentFromProductionPackages guard.
//  2. The package-level StartCompactAuthority(ctx, repo, CompactStartRequest)
//     entry point -- which searched every existing lineage in the repo for a
//     matching candidate to auto-resume/reuse/block -- is retired along with
//     CompactStartRequest/CompactStartResult/CompactStartAction. Its
//     single-lineage replacement, CompactStore.CreateOrReplayAtomicStart, is
//     already fully covered by compact_atomic_start_test.go; the cross-lineage
//     discovery half moved to AssessTargetStatus (target_status.go), which has
//     its own dedicated test file (target_status_test.go, outside this batch).
//     Old tests that exercised ONLY that discovery mechanism have no current
//     counterpart to restore into this file; see the batch report for the
//     full list.

func TestApprovedRecoveryTreatsBaseTreeMismatchAsScopeChange(t *testing.T) {
	snapshot := Snapshot{BaseTree: strings.Repeat("a", 40), CandidateTree: strings.Repeat("c", 40), PathsDigest: hash("1")}
	predecessor, successor := CompactState{State: StateApproved, CurrentSnapshot: snapshot}, CompactState{InitialSnapshot: snapshot}
	successor.InitialSnapshot.BaseTree = strings.Repeat("b", 40)
	predecessor.CurrentSnapshot.Kind, successor.InitialSnapshot.Kind = TargetCurrentChanges, TargetCurrentChanges
	if !compactRecoveryScopeChanged(predecessor.CurrentSnapshot, successor.InitialSnapshot) {
		t.Fatal("approved base-only mismatch was not recovery-eligible")
	}
	successor.InitialSnapshot.Kind = TargetFixDiff
	if compactRecoveryScopeChanged(predecessor.CurrentSnapshot, successor.InitialSnapshot) {
		t.Fatal("incompatible snapshot kinds created false base-only recovery")
	}
}

func TestCompactReleaseScopeRecoveryRequiresStrictSameCandidateExpansion(t *testing.T) {
	candidate := strings.Repeat("c", 40)
	previous := Snapshot{
		Kind: TargetCurrentChanges, Projection: ProjectionWorkspace, CandidateTree: candidate,
		Paths: []string{"reviewed.go"},
	}
	valid := Snapshot{
		Kind: TargetBaseDiff, Projection: ProjectionWorkspace, CandidateTree: candidate,
		Paths: []string{"release.go", "reviewed.go"},
	}
	predecessor := CompactState{InitialSnapshot: previous, CurrentSnapshot: previous, GenesisPaths: previous.Paths}
	predecessor.CurrentSnapshot.Kind = TargetFixDiff
	tests := []struct {
		name   string
		mutate func(*Snapshot)
	}{
		{name: "candidate changed", mutate: func(snapshot *Snapshot) { snapshot.CandidateTree = strings.Repeat("d", 40) }},
		{name: "predecessor path omitted", mutate: func(snapshot *Snapshot) { snapshot.Paths = []string{"release.go", "unrelated.go"} }},
		{name: "scope did not expand", mutate: func(snapshot *Snapshot) { snapshot.Paths = []string{"reviewed.go"} }},
		{name: "projection changed", mutate: func(snapshot *Snapshot) { snapshot.Projection = ProjectionStaged }},
		{name: "target kind is arbitrary", mutate: func(snapshot *Snapshot) { snapshot.Kind = TargetBaseWorkspaceOverlay }},
	}
	if !compactReleaseScopeRecovery(predecessor, valid) {
		t.Fatal("corrected current-changes release scope expansion was rejected")
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := valid
			candidate.Paths = append([]string(nil), valid.Paths...)
			tt.mutate(&candidate)
			if compactReleaseScopeRecovery(predecessor, candidate) {
				t.Fatalf("invalid release scope expansion accepted: %#v", candidate)
			}
		})
	}
}

// TestRecoverCompactAuthorityRejectsProjectionChange is adapted from dev: the
// predecessor fixture no longer produces or writes a CompactReceipt (retired),
// so it is built through the current receipt-free approvedCompactFixture
// instead of the dev-era approvedCompactCurrentChangesFixture.
func TestRecoverCompactAuthorityRejectsProjectionChange(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	gitSnapshot(t, repo, "add", "--", "tracked.txt")
	predecessor, store := approvedCompactFixture(t, repo, "recovery-staged-projection")
	state := predecessor.State
	state.InitialSnapshot.Projection = ProjectionStaged
	state.CurrentSnapshot.Projection = ProjectionStaged
	state.InitialSnapshot.Identity = snapshotIdentityForProjection(state.InitialSnapshot.Kind, state.InitialSnapshot.Projection, state.InitialSnapshot.BaseTree, state.InitialSnapshot.CandidateTree, state.InitialSnapshot.PathsDigest, state.InitialSnapshot.IntendedUntrackedProof, state.InitialSnapshot.IntendedUntracked, state.InitialSnapshot.LedgerIDs)
	state.CurrentSnapshot.Identity = snapshotIdentityForProjection(state.CurrentSnapshot.Kind, state.CurrentSnapshot.Projection, state.CurrentSnapshot.BaseTree, state.CurrentSnapshot.CandidateTree, state.CurrentSnapshot.PathsDigest, state.CurrentSnapshot.IntendedUntrackedProof, state.CurrentSnapshot.IntendedUntracked, state.CurrentSnapshot.LedgerIDs)
	record, payload, err := makeCompactRecord(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.StatePath(), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	writeSnapshotFile(t, repo, "tracked.txt", "new workspace scope\n")
	successor := newCompactTestState(t, repo, "recovery-workspace-projection")
	successor.Generation = state.Generation + 1
	if _, err := RecoverCompactAuthority(context.Background(), repo, CompactRecoveryRequest{PredecessorLineageID: state.LineageID, ExpectedPredecessorRevision: record.Revision, Successor: successor, Disposition: RecoveryScopeChanged, Reason: "scope changed", Actor: "maintainer"}); err == nil || !strings.Contains(err.Error(), "projection") {
		t.Fatalf("cross-projection recovery error = %v", err)
	}
}

// TestCorrectionRecoveryRejectsAuthorizedProjectionChange is adapted from dev
// only by swapping the retired newCompactStartStateForTarget helper for its
// current equivalent, newCompactFixtureStateForTarget.
func TestCorrectionRecoveryRejectsAuthorizedProjectionChange(t *testing.T) {
	repo, predecessor, _, record := correctionScopeRecoveryFixture(t, "correction-projection-predecessor")
	writeSnapshotFile(t, repo, "new-helper.go", "package helper\n")
	gitSnapshot(t, repo, "add", "new-helper.go")
	successor := newCompactFixtureStateForTarget(t, repo, "correction-projection-successor", Target{
		Kind: TargetCurrentChanges, Projection: ProjectionStaged, IntendedUntracked: []string{},
	})
	successor.Generation = predecessor.Generation + 1
	request := CompactRecoveryRequest{
		PredecessorLineageID: predecessor.LineageID, ExpectedPredecessorRevision: record.Revision, Successor: successor,
		Disposition: RecoveryScopeChanged, Reason: "expand correction scope", Actor: "maintainer",
	}
	request.MaintainerAuthorization = compactRecoveryAuthorizationBinding(predecessor.LineageID, record.Revision, successor.InitialSnapshot.Identity, request.Actor, request.Reason)
	if _, err := RecoverCompactAuthority(context.Background(), repo, request); err == nil || !strings.Contains(err.Error(), "retain the predecessor projection") {
		t.Fatalf("correction cross-projection error = %v", err)
	}
	successorStore, _ := CompactAuthoritativeStore(context.Background(), repo, successor.LineageID)
	if _, err := os.Stat(successorStore.StatePath()); !os.IsNotExist(err) {
		t.Fatalf("correction cross-projection recovery mutated successor: %v", err)
	}
}

// TestRecoverCompactAuthorityAllowsAuthorizedEscalatedProjectionChange is
// adapted from dev only by swapping the retired newCompactStartStateForTarget
// helper for its current equivalent, newCompactFixtureStateForTarget.
func TestRecoverCompactAuthorityAllowsAuthorizedEscalatedProjectionChange(t *testing.T) {
	repo := initSnapshotRepo(t)
	state := correctedCompactTestState(t, repo, "recovery-workspace-escalated")
	state.State = StateEscalated
	store, _ := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	record, payload, err := makeCompactRecord(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(store.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.StatePath(), payload, 0o644); err != nil {
		t.Fatal(err)
	}

	writeSnapshotFile(t, repo, "tracked.txt", "staged successor\n")
	gitSnapshot(t, repo, "add", "tracked.txt")
	writeSnapshotFile(t, repo, "tracked.txt", "unstaged divergence\n")
	successor := newCompactFixtureStateForTarget(t, repo, "recovery-staged-successor", Target{Kind: TargetCurrentChanges, Projection: ProjectionStaged, IntendedUntracked: []string{}})
	successor.Generation = state.Generation + 1
	if state.InitialSnapshot.Projection != "" && state.InitialSnapshot.Projection != ProjectionWorkspace || successor.InitialSnapshot.Projection != ProjectionStaged {
		t.Fatalf("fixture projections = %q -> %q", state.InitialSnapshot.Projection, successor.InitialSnapshot.Projection)
	}
	request := CompactRecoveryRequest{
		PredecessorLineageID: state.LineageID, ExpectedPredecessorRevision: record.Revision, Successor: successor,
		Disposition: RecoveryEscalated, Actor: "maintainer", Reason: "select exact staged target",
	}
	request.MaintainerAuthorization = compactRecoveryAuthorizationBinding(state.LineageID, record.Revision, successor.InitialSnapshot.Identity, request.Actor, request.Reason)

	recovered, err := RecoverCompactAuthority(context.Background(), repo, request)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.State.InitialSnapshot.Projection != ProjectionStaged || recovered.State.InitialSnapshot.CandidateTree != successor.InitialSnapshot.CandidateTree {
		t.Fatalf("cross-projection successor = %#v", recovered.State.InitialSnapshot)
	}
	if leaves, err := CompactAuthorityLeaves(context.Background(), repo); err != nil || len(leaves) != 1 || leaves[0].lineageID != successor.LineageID {
		t.Fatalf("reloaded recovery graph = %#v, %v", leaves, err)
	}
}

// TestCompactStoreRecoverRejectsIneligibleOrUnprovenPredecessor is adapted
// from dev only in its "approved without scope change" sub-case, which now
// builds its predecessor through the current receipt-free approvedCompactFixture
// instead of the dev-era approvedCompactCurrentChangesFixture.
func TestCompactStoreRecoverRejectsIneligibleOrUnprovenPredecessor(t *testing.T) {
	tests := []struct {
		name        string
		disposition RecoveryDisposition
		authorizer  string
		want        string
	}{
		{name: "approved without scope change", disposition: RecoveryScopeChanged, want: "scope has not changed"},
		{name: "reviewing", disposition: RecoveryInvalidated, want: "requires an invalidated predecessor"},
		{name: "escalated without authorization", disposition: RecoveryEscalated, authorizer: "", want: "maintainer authorization"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := initSnapshotRepo(t)
			writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
			var state CompactState
			var revision string
			var err error
			switch tt.name {
			case "approved without scope change":
				record, _ := approvedCompactFixture(t, repo, "recovery-predecessor")
				state, revision = record.State, record.Revision
			case "escalated without authorization":
				// escalatedCompactAuthorityFixture reaches StateEscalated through a
				// real admitted capture with an unresolved (insufficient-evidence)
				// finding, replacing dev's raw CompleteReview + CompleteVerification
				// dance -- the latter method is retired, and CompleteReview now
				// requires its LensResults to match state.AdmittedRoleResults.
				var record CompactRecord
				state, _, record = escalatedCompactAuthorityFixture(t, repo, "recovery-predecessor")
				revision = record.Revision
			default:
				state = newCompactTestState(t, repo, "recovery-predecessor")
				store, _ := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
				revision, err = store.Replace("", "review/start", state)
				if err != nil {
					t.Fatal(err)
				}
			}
			successor := newCompactTestState(t, repo, "recovery-successor")
			successor.Generation = state.Generation + 1
			_, err = RecoverCompactAuthority(context.Background(), repo, CompactRecoveryRequest{
				PredecessorLineageID: state.LineageID, ExpectedPredecessorRevision: revision, Successor: successor,
				Disposition: tt.disposition, Reason: "recover authority", Actor: "operator", MaintainerAuthorization: tt.authorizer,
			})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("recovery error = %v, want %q", err, tt.want)
			}
		})
	}
}

// TestCompactStagedCorrectionAcceptsIndexFixDespiteWorkspaceDivergence is
// adapted from dev only by swapping the retired newCompactStartStateForTarget
// helper for its current equivalent, newCompactFixtureStateForTarget. It
// exercises CompactState.CompleteCorrection directly and has no dependency on
// the retired StartCompactAuthority discovery path.
func TestCompactStagedCorrectionAcceptsIndexFixDespiteWorkspaceDivergence(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "base\none\ntwo\nthree\nwrong\n")
	gitSnapshot(t, repo, "add", "--", "tracked.txt")
	state := newCompactFixtureStateForTarget(t, repo, "compact-staged-correction-accept", Target{Kind: TargetCurrentChanges, Projection: ProjectionStaged, IntendedUntracked: []string{}})
	store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace("", "review/start", state); err != nil {
		t.Fatal(err)
	}
	finding := Finding{ID: "R3-001", Lens: strings.TrimPrefix(state.SelectedLenses[0], "review-"), Location: "tracked.txt:5", Severity: "CRITICAL", Claim: "wrong staged value", ProofRefs: []string{"candidate-only failure"}}
	results := make([]LensResult, len(state.SelectedLenses))
	for index, lens := range state.SelectedLenses {
		results[index] = LensResult{Lens: lens, Findings: []Finding{}, Evidence: []string{"reviewed"}}
	}
	results[0].Findings = []Finding{finding}
	// Admitted capture replaces dev's raw CompleteReview call: CompleteReview
	// now requires its LensResults to match state.AdmittedRoleResults.
	state, _ = captureAndCompleteCompactReview(t, store, state, CompactReviewInput{
		LensResults:     results,
		Classifications: []FindingEvidence{{FindingID: finding.ID, Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "changed hunk"}},
		RefuterOutcomes: []EvidenceResult{},
	})
	if err := state.BeginCorrection(2); err != nil {
		t.Fatal(err)
	}
	writeSnapshotFile(t, repo, "tracked.txt", "base\none\ntwo\nthree\nfixed\n")
	gitSnapshot(t, repo, "add", "--", "tracked.txt")
	writeSnapshotFile(t, repo, "tracked.txt", "unstaged workspace divergence\n")

	fix, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetFixDiff, Projection: ProjectionStaged, BaseRef: state.CurrentSnapshot.CandidateTree, IntendedUntracked: []string{}, LedgerIDs: state.FixFindingIDs})
	if err != nil {
		t.Fatal(err)
	}
	if got := gitSnapshot(t, repo, "show", fix.CandidateTree+":tracked.txt"); got != "base\none\ntwo\nthree\nfixed\n" {
		t.Fatalf("staged correction candidate = %q", got)
	}
	fixHash := FixDeltaHashForSnapshot(fix)
	validation := ScopedValidationResult{LedgerIDs: state.FixFindingIDs, FixCausedFindings: []Finding{}, FollowUps: []FollowUp{}, OriginalCriteria: ValidationCheck{Passed: true, EvidenceHash: hash("2"), FixDeltaHash: fixHash}, CorrectionRegression: ValidationCheck{Passed: true, EvidenceHash: hash("3"), FixDeltaHash: fixHash}}
	if err := state.CompleteCorrection(fix, 2, bindTargetedValidationForTest(validation, fix)); err != nil {
		t.Fatalf("CompleteCorrection(staged fix) error = %v", err)
	}
	if state.State != StateValidating {
		t.Fatalf("staged correction state = %#v", state)
	}
}

// TestCompactStagedCorrectionRejectsWorkspaceSnapshotWithoutMutatingState is
// adapted from dev only by swapping the retired newCompactStartStateForTarget
// helper for its current equivalent, newCompactFixtureStateForTarget.
func TestCompactStagedCorrectionRejectsWorkspaceSnapshotWithoutMutatingState(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "base\nwrong\n")
	gitSnapshot(t, repo, "add", "--", "tracked.txt")
	state := newCompactFixtureStateForTarget(t, repo, "compact-staged-correction", Target{Kind: TargetCurrentChanges, Projection: ProjectionStaged, IntendedUntracked: []string{}})
	store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace("", "review/start", state); err != nil {
		t.Fatal(err)
	}
	finding := Finding{ID: "R3-001", Lens: strings.TrimPrefix(state.SelectedLenses[0], "review-"), Location: "tracked.txt:2", Severity: "CRITICAL", Claim: "wrong staged value", ProofRefs: []string{"candidate-only failure"}}
	results := make([]LensResult, len(state.SelectedLenses))
	for index, lens := range state.SelectedLenses {
		results[index] = LensResult{Lens: lens, Findings: []Finding{}, Evidence: []string{"reviewed"}}
	}
	results[0].Findings = []Finding{finding}
	// Admitted capture replaces dev's raw CompleteReview call: CompleteReview
	// now requires its LensResults to match state.AdmittedRoleResults.
	state, _ = captureAndCompleteCompactReview(t, store, state, CompactReviewInput{
		LensResults:     results,
		Classifications: []FindingEvidence{{FindingID: finding.ID, Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "changed hunk"}},
		RefuterOutcomes: []EvidenceResult{},
	})
	if err := state.BeginCorrection(1); err != nil {
		t.Fatal(err)
	}
	before := state
	writeSnapshotFile(t, repo, "tracked.txt", "base\nfixed\n")
	workspaceFix, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetFixDiff, BaseRef: state.CurrentSnapshot.CandidateTree, IntendedUntracked: []string{}, LedgerIDs: state.FixFindingIDs})
	if err != nil {
		t.Fatal(err)
	}
	fixHash := FixDeltaHashForSnapshot(workspaceFix)
	validation := ScopedValidationResult{LedgerIDs: state.FixFindingIDs, FixCausedFindings: []Finding{}, FollowUps: []FollowUp{}, OriginalCriteria: ValidationCheck{Passed: true, EvidenceHash: hash("2"), FixDeltaHash: fixHash}, CorrectionRegression: ValidationCheck{Passed: true, EvidenceHash: hash("3"), FixDeltaHash: fixHash}}
	if err := state.CompleteCorrection(workspaceFix, 1, bindTargetedValidationForTest(validation, workspaceFix)); err == nil || !strings.Contains(err.Error(), "projection") {
		t.Fatalf("workspace correction error = %v", err)
	}
	if !reflect.DeepEqual(state, before) {
		t.Fatalf("rejected workspace correction mutated staged state:\nbefore=%#v\nafter=%#v", before, state)
	}

	stagedFix, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetFixDiff, Projection: ProjectionStaged, BaseRef: state.CurrentSnapshot.CandidateTree, IntendedUntracked: []string{}, LedgerIDs: state.FixFindingIDs})
	if err != nil {
		t.Fatal(err)
	}
	fixHash = FixDeltaHashForSnapshot(stagedFix)
	validation.OriginalCriteria.FixDeltaHash, validation.CorrectionRegression.FixDeltaHash = fixHash, fixHash
	if err := state.CompleteCorrection(stagedFix, 0, bindTargetedValidationForTest(validation, stagedFix)); err == nil || !strings.Contains(err.Error(), "unchanged candidate") {
		t.Fatalf("unchanged staged correction error = %v", err)
	}
	if !reflect.DeepEqual(state, before) {
		t.Fatalf("rejected unchanged staged correction mutated state:\nbefore=%#v\nafter=%#v", before, state)
	}
}

func TestCompactStoreReplacesCurrentStateWithCASAndExactRetry(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	state := newCompactTestState(t, repo, "compact-cas")
	store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace("", "review/start", state); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(store.Dir, "events")); !os.IsNotExist(err) {
		t.Fatalf("compact store created event history: %v", err)
	}
	results := make([]LensResult, len(state.SelectedLenses))
	for index, lens := range state.SelectedLenses {
		results[index] = LensResult{Lens: lens, Findings: []Finding{}, Evidence: []string{"review completed"}}
	}
	// Admitted capture replaces dev's raw CompleteReview call: CompleteReview
	// now requires its LensResults to match state.AdmittedRoleResults. captured
	// is the store revision produced by the capture(s), which now stands in for
	// dev's pre-capture "first" revision as the CAS anchor below.
	state, captured := captureAndCompleteCompactReview(t, store, state, CompactReviewInput{LensResults: results, Classifications: []FindingEvidence{}, RefuterOutcomes: []EvidenceResult{}})
	first := captured.Revision
	second, err := store.Replace(first, "review/complete-review", state)
	if err != nil || second == first {
		t.Fatalf("compact replacement = %q, %v", second, err)
	}
	if retry, err := store.Replace(first, "review/complete-review", state); err != nil || retry != second {
		t.Fatalf("exact compact retry = %q, %v", retry, err)
	}
	forged := state
	forged.PolicyHash = hash("f")
	if _, err := store.Replace(first, "review/complete-review", forged); !errors.Is(err, ErrConcurrentUpdate) {
		t.Fatalf("stale expected revision error = %v", err)
	}
	if _, err := store.Replace(second, "review/complete-verification", forged); !errors.Is(err, ErrInvalidSuccessor) {
		t.Fatalf("illegal compact successor error = %v", err)
	}
	loaded, err := store.Load()
	if err != nil || loaded.Revision != second || !compactStateEqual(loaded.State, state) {
		t.Fatalf("loaded compact authority = %#v, %v", loaded, err)
	}
}

func persistedCompactCorrectionRequired(t *testing.T, repo, lineage string) (CompactState, CompactStore, string) {
	t.Helper()
	writeSnapshotFile(t, repo, "tracked.txt", "base\none\ntwo\nthree\nwrong\n")
	state := newCompactTestState(t, repo, lineage)
	store, _ := CompactAuthoritativeStore(context.Background(), repo, lineage)
	revision, err := store.Replace("", "review/start", state)
	if err != nil {
		t.Fatal(err)
	}
	finding := Finding{ID: "R3-001", Lens: strings.TrimPrefix(state.SelectedLenses[0], "review-"), Location: "tracked.txt:5", Severity: "CRITICAL", Claim: "wrong value", ProofRefs: []string{"candidate-only failure"}}
	if err := state.CompleteReview(CompactReviewInput{
		LensResults:     []LensResult{{Lens: state.SelectedLenses[0], Findings: []Finding{finding}, Evidence: []string{"reviewed once"}}},
		Classifications: []FindingEvidence{{FindingID: finding.ID, Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "changed hunk"}}, RefuterOutcomes: []EvidenceResult{},
	}); err != nil {
		t.Fatal(err)
	}
	revision, err = store.Replace(revision, "review/complete-review", state)
	if err != nil {
		t.Fatal(err)
	}
	return state, store, revision
}

// TestCompactCorrectionForecastCASIsIdempotentAndRejectsCompetingForecast was
// flagged UNCLEAR by the prior audit pass: it asked whether the current
// TestCompactRevisionConflictIsTypedAndProvesNonMutation (compact_revision_conflict_test.go)
// already covers the "identical forecast replay is idempotent" half. It does
// not -- that test only proves a LOSING competing write is refused without
// mutation; it never replays the WINNING write's own exact revision/state pair
// and checks the replay returns the same committed revision without error.
// Restored as-is; it is a pure state-machine + store.Replace test with no
// dependency on the retired discovery or receipt mechanisms.
func TestCompactCorrectionForecastCASIsIdempotentAndRejectsCompetingForecast(t *testing.T) {
	repo := initSnapshotRepo(t)
	state, store, revision := persistedCompactCorrectionRequired(t, repo, "compact-correction-forecast-cas")

	first := state
	if err := first.BeginCorrection(1); err != nil {
		t.Fatal(err)
	}
	committed, err := store.Replace(revision, "review/begin-fix", first)
	if err != nil {
		t.Fatal(err)
	}
	if replayed, err := store.Replace(revision, "review/begin-fix", first); err != nil || replayed != committed {
		t.Fatalf("identical forecast replay = %q, %v; want %q", replayed, err, committed)
	}
	competing := state
	if err := competing.BeginCorrection(2); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace(revision, "review/begin-fix", competing); !errors.Is(err, ErrConcurrentUpdate) {
		t.Fatalf("competing forecast error = %v, want ErrConcurrentUpdate", err)
	}
	loaded, err := store.Load()
	if err != nil || loaded.Revision != committed || loaded.State.ProposedCorrectionLines == nil || *loaded.State.ProposedCorrectionLines != 1 || len(loaded.State.CorrectionAttempts) != 0 {
		t.Fatalf("forecast authority = %#v, %v", loaded, err)
	}
}

func TestCompactStoreRejectsSecondOrdinaryCorrectionWithoutMutation(t *testing.T) {
	const consumed = "ordinary compact correction already consumed"

	t.Run("begin fix", func(t *testing.T) {
		repo, state, record, _ := historicalFailedValidatorFixture(t, "compact-second-begin")
		store, _ := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
		before, err := os.ReadFile(store.StatePath())
		if err != nil {
			t.Fatal(err)
		}
		next := state
		forecast := 1
		next.ProposedCorrectionLines = &forecast
		if err := next.Validate(); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Replace(record.Revision, "review/begin-fix", next); !errors.Is(err, ErrInvalidSuccessor) || !strings.Contains(err.Error(), consumed) {
			t.Fatalf("second begin error = %v", err)
		}
		assertCompactAuthorityBytes(t, store, record.Revision, before)
		methodState := state
		if err := methodState.BeginCorrection(1); err == nil || !strings.Contains(err.Error(), consumed) || !reflect.DeepEqual(methodState, state) {
			t.Fatalf("BeginCorrection() = %v, state changed=%t", err, !reflect.DeepEqual(methodState, state))
		}
	})

	t.Run("complete fix", func(t *testing.T) {
		repo, state, _, _ := historicalFailedValidatorFixture(t, "compact-second-complete")
		forecast := 1
		state.ProposedCorrectionLines = &forecast
		record, payload, err := makeCompactRecord(state)
		if err != nil {
			t.Fatal(err)
		}
		store, _ := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
		if err := os.WriteFile(store.StatePath(), payload, 0o644); err != nil {
			t.Fatal(err)
		}
		methodState := state
		if err := methodState.CompleteCorrection(Snapshot{}, 0, ScopedValidationResult{}); err == nil || !strings.Contains(err.Error(), consumed) || !reflect.DeepEqual(methodState, state) {
			t.Fatalf("CompleteCorrection() = %v, state changed=%t", err, !reflect.DeepEqual(methodState, state))
		}

		writeSnapshotFile(t, repo, "tracked.txt", "base\none\ntwo\nthree\nfixed\nextra\n")
		fix, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
			Kind: TargetFixDiff, BaseRef: state.CurrentSnapshot.CandidateTree,
			IntendedUntracked: state.InitialSnapshot.IntendedUntracked, LedgerIDs: state.FixFindingIDs,
		})
		if err != nil {
			t.Fatal(err)
		}
		actual, err := (SnapshotBuilder{Repo: repo}).ChangedLines(context.Background(), fix)
		if err != nil {
			t.Fatal(err)
		}
		fixHash := FixDeltaHashForSnapshot(fix)
		validation := ScopedValidationResult{
			LedgerIDs: state.FixFindingIDs, FixCausedFindings: []Finding{}, FollowUps: []FollowUp{},
			OriginalCriteria:              ValidationCheck{Passed: true, EvidenceHash: hash("8"), FixDeltaHash: fixHash},
			CorrectionRegression:          ValidationCheck{Passed: true, EvidenceHash: hash("9"), FixDeltaHash: fixHash},
			TargetedValidationRequestHash: hash("a"), CorrectionTargetIdentity: fix.Identity,
		}
		next := state
		next.CorrectionAttempts = append(append([]CompactCorrectionAttempt{}, state.CorrectionAttempts...), CompactCorrectionAttempt{
			Snapshot: fix, ProposedLines: forecast, ActualLines: actual, FixDeltaHash: fixHash,
			OriginalCriteria: validation.OriginalCriteria, CorrectionRegression: validation.CorrectionRegression,
			TargetedValidationRequestHash: validation.TargetedValidationRequestHash, CorrectionTargetIdentity: validation.CorrectionTargetIdentity,
		})
		next.CumulativeCorrectionLines += actual
		next.CurrentSnapshot, next.FixDeltaHash = fix, fixHash
		next.ActualCorrectionLines = &actual
		original, regression := validation.OriginalCriteria, validation.CorrectionRegression
		next.OriginalCriteria, next.CorrectionRegression = &original, &regression
		next.State = StateValidating
		if err := next.Validate(); err != nil {
			t.Fatal(err)
		}
		before, err := os.ReadFile(store.StatePath())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.Replace(record.Revision, "review/complete-fix", next); !errors.Is(err, ErrInvalidSuccessor) || !strings.Contains(err.Error(), consumed) {
			t.Fatalf("second complete error = %v", err)
		}
		assertCompactAuthorityBytes(t, store, record.Revision, before)
	})
}

func assertCompactAuthorityBytes(t *testing.T, store CompactStore, revision string, before []byte) {
	t.Helper()
	after, err := os.ReadFile(store.StatePath())
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("rejected successor changed authority bytes: %v", err)
	}
	loaded, err := store.Load()
	if err != nil || loaded.Revision != revision {
		t.Fatalf("rejected successor changed authority revision: got %#v, %v; want %s", loaded, err, revision)
	}
}

func TestCompactStoreReplaceContextRejectsCancelledMutation(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	state := newCompactTestState(t, repo, "compact-cancelled-replace")
	store, _ := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.ReplaceContext(ctx, "", "review/start", state); !errors.Is(err, context.Canceled) {
		t.Fatalf("ReplaceContext() error = %v, want context.Canceled", err)
	}
	if _, err := os.Stat(store.StatePath()); !os.IsNotExist(err) {
		t.Fatalf("cancelled replacement published authority: %v", err)
	}
}

func TestSyncReviewDirectoryHandlesUnsupportedWindowsDirectorySync(t *testing.T) {
	originalSync, originalGOOS := syncReviewDirectory, reviewRuntimeGOOS
	t.Cleanup(func() {
		syncReviewDirectory, reviewRuntimeGOOS = originalSync, originalGOOS
	})
	for _, tt := range []struct {
		name string
		goos string
		err  error
		want bool
	}{
		{name: "windows permission is unsupported", goos: "windows", err: os.ErrPermission, want: false},
		{name: "all platforms reject invalid directory handle", goos: "linux", err: syscall.EINVAL, want: false},
		{name: "all platforms reject declared unsupported", goos: "linux", err: errors.ErrUnsupported, want: false},
		{name: "other sync failures fail closed", goos: "windows", err: errors.New("disk failure"), want: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			syncReviewDirectory = func(string) error { return tt.err }
			reviewRuntimeGOOS = func() string { return tt.goos }
			if got := SyncReviewDirectory(t.TempDir()) != nil; got != tt.want {
				t.Fatalf("SyncReviewDirectory() error presence = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestCompactMalformedValidatorDoesNotConsumeAuthority(t *testing.T) {
	repo := initSnapshotRepo(t)
	state, fix := pendingCompactCorrection(t, repo, "validator-malformed")
	before := state
	validation := ScopedValidationResult{LedgerIDs: state.FixFindingIDs, FixCausedFindings: []Finding{}, FollowUps: []FollowUp{},
		OriginalCriteria:     ValidationCheck{EvidenceHash: "not-a-hash", FixDeltaHash: FixDeltaHashForSnapshot(fix)},
		CorrectionRegression: ValidationCheck{EvidenceHash: hash("regression"), FixDeltaHash: FixDeltaHashForSnapshot(fix)}}
	if err := state.CompleteCorrection(fix, 1, bindTargetedValidationForTest(validation, fix)); err == nil || !reflect.DeepEqual(state, before) {
		t.Fatalf("malformed validator consumed authority: %#v, %v", state, err)
	}
}

func TestCompactClearedEscalationRequiresHistoricalExhaustion(t *testing.T) {
	forgedRepo := initSnapshotRepo(t)
	forged, fix := pendingCompactCorrection(t, forgedRepo, "forged-cleared-escalation")
	fixHash := FixDeltaHashForSnapshot(fix)
	validation := ScopedValidationResult{LedgerIDs: forged.FixFindingIDs, FixCausedFindings: []Finding{}, FollowUps: []FollowUp{},
		OriginalCriteria: ValidationCheck{EvidenceHash: hash("2"), FixDeltaHash: fixHash, Passed: true}, CorrectionRegression: ValidationCheck{EvidenceHash: hash("3"), FixDeltaHash: fixHash, Passed: true}}
	if err := forged.CompleteCorrection(fix, 1, bindTargetedValidationForTest(validation, fix)); err != nil {
		t.Fatal(err)
	}
	forged.State, forged.ActualCorrectionLines, forged.OriginalCriteria, forged.CorrectionRegression = StateEscalated, nil, nil, nil
	forged.FixDeltaHash = EmptyFixDeltaHash
	const want = "completed compact correction requires in-budget forecast and actual size"
	if err := forged.Validate(); err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("one-attempt cleared escalation validation = %v, want %q", err, want)
	}
	forgedRecord, _, _ := makeCompactRecord(forged)
	forgedTransport := CompactTransport{Schema: CompactTransportSchema, Record: forgedRecord}
	forgedTransport.BundleDigest = compactTransportDigest(forgedTransport)
	gitSnapshot(t, forgedRepo, "commit", "-am", "forged correction delivery")
	if _, err := ImportCompactTransport(context.Background(), forgedRepo, forgedTransport); err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("one-attempt cleared escalation import = %v, want %q", err, want)
	}

	historicalRepo := initSnapshotRepo(t)
	historical, next := pendingCompactCorrection(t, historicalRepo, "historical-cleared-escalation")
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			writeSnapshotFile(t, historicalRepo, "tracked.txt", strings.Repeat("historical correction\n", attempt+1))
			next, _ = (SnapshotBuilder{Repo: historicalRepo}).Build(context.Background(), Target{Kind: TargetFixDiff, BaseRef: historical.CurrentSnapshot.CandidateTree, IntendedUntracked: historical.InitialSnapshot.IntendedUntracked, LedgerIDs: historical.FixFindingIDs})
		}
		fixDelta := FixDeltaHashForSnapshot(next)
		historical.CorrectionAttempts = append(historical.CorrectionAttempts, CompactCorrectionAttempt{Snapshot: next, ProposedLines: 1, FixDeltaHash: fixDelta,
			OriginalCriteria: ValidationCheck{EvidenceHash: hash(string(rune('a' + attempt))), FixDeltaHash: fixDelta, Passed: true}, CorrectionRegression: ValidationCheck{EvidenceHash: hash(string(rune('d' + attempt))), FixDeltaHash: fixDelta}})
		historical.CurrentSnapshot = next
	}
	historical.State, historical.ProposedCorrectionLines = StateEscalated, nil
	if err := historical.Validate(); err != nil {
		t.Fatalf("historical three-attempt cleared state: %v", err)
	}
	historicalRecord, _, _ := makeCompactRecord(historical)
	historicalTransport := CompactTransport{Schema: CompactTransportSchema, Record: historicalRecord}
	historicalTransport.BundleDigest = compactTransportDigest(historicalTransport)
	gitSnapshot(t, historicalRepo, "commit", "-am", "historical correction delivery")
	if _, err := ImportCompactTransport(context.Background(), historicalRepo, historicalTransport); err != nil {
		t.Fatalf("import historical three-attempt cleared state: %v", err)
	}
}

// TestEscalatedRecoveryRequiresChangedTarget is adapted from dev: the parallel
// assertions against the retired package-level StartCompactAuthority are
// dropped (that discovery entry point no longer exists -- see the file-level
// note above), keeping only the AssessTargetStatus and RecoverCompactAuthority
// assertions, which are both still current and were exercised side-by-side
// with StartCompactAuthority in the original.
func TestEscalatedRecoveryRequiresChangedTarget(t *testing.T) {
	repo := initSnapshotRepo(t)
	state := correctedCompactTestState(t, repo, "escalated-target")
	state.State = StateEscalated
	store, _ := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	record, payload, err := makeCompactRecord(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(store.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.StatePath(), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	successor := newCompactTestState(t, repo, "escalated-target-g2")
	successor.Generation = state.Generation + 1
	request := CompactRecoveryRequest{PredecessorLineageID: state.LineageID, ExpectedPredecessorRevision: record.Revision, Successor: successor,
		Disposition: RecoveryEscalated, Actor: "maintainer", Reason: "retry terminal validator"}
	request.MaintainerAuthorization = compactRecoveryAuthorizationBinding(state.LineageID, record.Revision, successor.InitialSnapshot.Identity, request.Actor, request.Reason)
	status, statusErr := AssessTargetStatus(context.Background(), repo, TargetStatusRequest{Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}, LineageID: state.LineageID})
	if statusErr != nil || status.Action != TargetStatusActionStop || status.Replayability != ReplayabilityManualActionRequired {
		t.Fatalf("same-target terminal status = %#v, %v", status, statusErr)
	}
	if _, err := RecoverCompactAuthority(context.Background(), repo, request); err == nil || !strings.Contains(err.Error(), "target has not changed") {
		t.Fatalf("same-target escalated recovery error = %v", err)
	}
	writeSnapshotFile(t, repo, "tracked.txt", "changed escalated target\n")
	request.Successor = newCompactTestState(t, repo, successor.LineageID)
	request.Successor.Generation = state.Generation + 1
	request.MaintainerAuthorization = compactRecoveryAuthorizationBinding(state.LineageID, record.Revision, request.Successor.InitialSnapshot.Identity, request.Actor, request.Reason)
	status, statusErr = AssessTargetStatus(context.Background(), repo, TargetStatusRequest{Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}, LineageID: state.LineageID})
	recovered, recoverErr := RecoverCompactAuthority(context.Background(), repo, request)
	replayed, replayErr := RecoverCompactAuthority(context.Background(), repo, request)
	after, _ := os.ReadFile(store.StatePath())
	if statusErr != nil || recoverErr != nil || replayErr != nil || status.Action != TargetStatusActionRecover ||
		replayed.Revision != recovered.Revision || !bytes.Equal(payload, after) {
		t.Fatalf("changed-target recovery: status=%#v recovery=%v replay=%v", status, recoverErr, replayErr)
	}
}

// historicalFailedValidatorFixture is defined in target_status_test.go
// (restored there verbatim by a sibling batch, since none of its
// dependencies were touched by the merge).

// TestCompactHistoricalFailedValidatorTransportRequiresExactBinding is
// restored as-is: it builds a plain CompactTransport{Schema, Record} (no
// receipt field was ever referenced here), so it needed no adaptation for the
// receipt retirement.
func TestCompactHistoricalFailedValidatorTransportRequiresExactBinding(t *testing.T) {
	repo, state, predecessor, _ := historicalFailedValidatorFixture(t, "historical-transport")
	gitSnapshot(t, repo, "add", "tracked.txt")
	gitSnapshot(t, repo, "commit", "-m", "historical corrected candidate")
	predecessorTransport := CompactTransport{Schema: CompactTransportSchema, Record: predecessor}
	predecessorTransport.BundleDigest = compactTransportDigest(predecessorTransport)

	for _, tt := range []struct {
		name, want                 string
		changed, exact, projection bool
	}{{"same target", "target has not changed", false, true, false}, {"changed target", "", true, true, false},
		{"authorized projection change", "", true, true, true}, {"wrong projection binding", "exact maintainer authorization", true, false, true}} {
		t.Run(tt.name, func(t *testing.T) {
			destination := filepath.Join(t.TempDir(), "clone")
			gitSnapshot(t, repo, "clone", "--no-local", repo, destination)
			if _, err := ImportCompactTransport(context.Background(), destination, predecessorTransport); err != nil {
				t.Fatal(err)
			}
			if tt.changed {
				writeSnapshotFile(t, destination, "tracked.txt", "changed imported target\n")
				gitSnapshot(t, destination, "add", "tracked.txt")
				gitSnapshot(t, destination, "config", "user.email", "test@example.com")
				gitSnapshot(t, destination, "config", "user.name", "Test User")
				gitSnapshot(t, destination, "commit", "-m", "changed recovery target")
			}
			successor := newCompactRevisionState(t, destination, "historical-transport-g2-"+strings.ReplaceAll(tt.name, " ", "-"))
			successor.Generation = state.Generation + 1
			if tt.projection {
				successor.InitialSnapshot.Kind = TargetBaseDiff
				successor.CurrentSnapshot.Kind = TargetBaseDiff
				successor.InitialSnapshot.Projection = ProjectionStaged
				successor.CurrentSnapshot.Projection = ProjectionStaged
				successor.InitialSnapshot.Identity = snapshotIdentityForProjection(successor.InitialSnapshot.Kind, ProjectionStaged, successor.InitialSnapshot.BaseTree, successor.InitialSnapshot.CandidateTree, successor.InitialSnapshot.PathsDigest, successor.InitialSnapshot.IntendedUntrackedProof, successor.InitialSnapshot.IntendedUntracked, successor.InitialSnapshot.LedgerIDs)
				successor.CurrentSnapshot.Identity = snapshotIdentityForProjection(successor.CurrentSnapshot.Kind, ProjectionStaged, successor.CurrentSnapshot.BaseTree, successor.CurrentSnapshot.CandidateTree, successor.CurrentSnapshot.PathsDigest, successor.CurrentSnapshot.IntendedUntrackedProof, successor.CurrentSnapshot.IntendedUntracked, successor.CurrentSnapshot.LedgerIDs)
			}
			successor.Recovery = &CompactRecoveryProvenance{PredecessorLineageID: state.LineageID, PredecessorRevision: predecessor.Revision,
				Disposition: RecoveryEscalated, Actor: "maintainer", Reason: "recover failed validator", RecoveredAt: time.Now().UTC()}
			successor.Recovery.MaintainerAuthorization = "wrong"
			if tt.exact {
				successor.Recovery.MaintainerAuthorization = compactRecoveryAuthorizationBinding(state.LineageID, predecessor.Revision, successor.InitialSnapshot.Identity, successor.Recovery.Actor, successor.Recovery.Reason)
			}
			record, _, err := makeCompactRecord(successor)
			if err != nil {
				t.Fatal(err)
			}
			transport := CompactTransport{Schema: CompactTransportSchema, Record: record}
			transport.BundleDigest = compactTransportDigest(transport)
			_, err = ImportCompactTransport(context.Background(), destination, transport)
			store, _ := CompactAuthoritativeStore(context.Background(), destination, successor.LineageID)
			if tt.want != "" {
				if err == nil || !strings.Contains(err.Error(), tt.want) {
					t.Fatalf("import error = %v, want %q", err, tt.want)
				}
				if _, statErr := os.Stat(store.StatePath()); !os.IsNotExist(statErr) {
					t.Fatalf("wrong binding installed successor: %v", statErr)
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCompactStoreFailsClosedForCorruptionAndIgnoresInvalidTempState(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	state := newCompactTestState(t, repo, "compact-corruption")
	store, _ := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	revision, err := store.Replace("", "review/start", state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.Dir, ".atomic-interrupted"), []byte("not authority"), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load()
	if err != nil || loaded.Revision != revision {
		t.Fatalf("invalid temp displaced authority: %#v, %v", loaded, err)
	}
	payload, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	var record map[string]any
	if err := json.Unmarshal(payload, &record); err != nil {
		t.Fatal(err)
	}
	record["revision"] = hash("a")
	corrupt, _ := json.Marshal(record)
	if err := os.WriteFile(store.StatePath(), corrupt, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("corrupt compact state error = %v", err)
	}
}

func TestCompactDiscoveryIgnoresOnlyUnpublishedCrashResidue(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	state := newCompactTestState(t, repo, "compact-published")
	store, _ := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if _, err := store.Replace("", "review/start", state); err != nil {
		t.Fatal(err)
	}
	residue, _ := CompactAuthoritativeStore(context.Background(), repo, "compact-crash-residue")
	if err := os.MkdirAll(residue.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".atomic-interrupted", ".publish-interrupted"} {
		if err := os.WriteFile(filepath.Join(residue.Dir, name), []byte("partial"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	leaves, err := CompactAuthorityLeaves(context.Background(), repo)
	if err != nil || len(leaves) != 1 || leaves[0].lineageID != state.LineageID {
		t.Fatalf("leaves with crash residue = %#v, %v", leaves, err)
	}
	unexpected := filepath.Join(residue.Dir, "unexpected-residue")
	if err := os.WriteFile(unexpected, []byte("not a temporary publication"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := CompactAuthorityLeaves(context.Background(), repo); err == nil {
		t.Fatal("unexpected state-less lineage entry was hidden as crash residue")
	}
	if err := os.Remove(unexpected); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(residue.StatePath(), []byte("corrupt published authority"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := CompactAuthorityLeaves(context.Background(), repo); err == nil {
		t.Fatal("corrupt published authority was hidden as residue")
	}
}

func TestCompactStoreRejectsForgedServiceTokenRiskDowngrade(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "neutral/service-token.ts", "export const token = 'candidate'\n")
	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{"neutral/service-token.ts"}})
	if err != nil {
		t.Fatal(err)
	}
	lines, err := (SnapshotBuilder{Repo: repo}).ChangedLines(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	state, err := NewCompactState(Start{
		LineageID: "compact-service-token-forgery", Mode: ModeOrdinaryBounded, Generation: 1,
		Snapshot: snapshot, PolicyHash: hash("1"), RiskLevel: RiskMedium,
		SelectedLenses: []string{LensReliability}, OriginalChangedLines: &lines,
	})
	if err != nil {
		t.Fatal(err)
	}
	store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace("", "review/start", state); err == nil || !errors.Is(err, ErrInvalidSuccessor) {
		t.Fatalf("forged medium service-token state error = %v, want invalid successor", err)
	}
	for _, lenses := range [][]string{{LensRisk}, {LensReliability, LensReadability, LensResilience, LensRisk}} {
		if _, err := NewCompactState(Start{
			LineageID: "compact-service-token-invalid-high", Mode: ModeOrdinaryBounded, Generation: 1,
			Snapshot: snapshot, PolicyHash: hash("1"), RiskLevel: RiskHigh,
			SelectedLenses: lenses, OriginalChangedLines: &lines,
		}); err == nil {
			t.Fatalf("invalid high-risk lenses %v were accepted", lenses)
		}
	}
}

// TestCompactStateRejectsChecksumValidImpossibleSemantics is adapted from
// dev: sub-cases that mutated CompactState.LensResults/Findings/
// Classifications/Outcomes directly are dropped. Those fields are retired --
// CompactReviewView now derives all review semantics from AdmittedRoleResults
// (see the Historical* decode-only stubs on CompactState) -- so that
// consistency surface no longer has a state-field counterpart to tamper with
// here; it is exercised instead through CompactReviewView's own extensive
// validation and this package's causality/escalation-accounting tests. The
// sub-cases below still mutate plain CompactState fields untouched by that
// migration (FixFindingIDs, FixDeltaHash, CurrentSnapshot, OriginalCriteria,
// CorrectionRegression) and are restored as-is.
func TestCompactStateRejectsChecksumValidImpossibleSemantics(t *testing.T) {
	repo := initSnapshotRepo(t)
	valid := correctedCompactTestState(t, repo, "compact-semantic-invalid")

	tests := []struct {
		name   string
		mutate func(*CompactState)
	}{
		{name: "corroborated causal finding omitted from fix IDs", mutate: func(state *CompactState) { state.FixFindingIDs = []string{} }},
		{name: "arbitrary fix delta hash", mutate: func(state *CompactState) { state.FixDeltaHash = hash("f") }},
		{name: "approved correction has no completed correction", mutate: func(state *CompactState) {
			state.CurrentSnapshot = state.InitialSnapshot
			state.FixDeltaHash = EmptyFixDeltaHash
			state.ProposedCorrectionLines = nil
			state.ActualCorrectionLines = nil
			state.OriginalCriteria = nil
			state.CorrectionRegression = nil
		}},
		{name: "corrected state uses wrong fix base", mutate: func(state *CompactState) { state.CurrentSnapshot.BaseTree = state.InitialSnapshot.BaseTree }},
		{name: "corrected state uses wrong ledger IDs", mutate: func(state *CompactState) { state.CurrentSnapshot.LedgerIDs = []string{"OTHER"} }},
		{name: "approved correction has failed targeted check", mutate: func(state *CompactState) { state.OriginalCriteria.Passed = false }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := valid
			state.FixFindingIDs = append([]string(nil), valid.FixFindingIDs...)
			if valid.OriginalCriteria != nil {
				original, regression := *valid.OriginalCriteria, *valid.CorrectionRegression
				state.OriginalCriteria, state.CorrectionRegression = &original, &regression
			}
			tt.mutate(&state)
			state.InitialSnapshot.Identity = snapshotIdentity(state.InitialSnapshot.Kind, state.InitialSnapshot.BaseTree, state.InitialSnapshot.CandidateTree, state.InitialSnapshot.PathsDigest, state.InitialSnapshot.IntendedUntrackedProof, state.InitialSnapshot.IntendedUntracked, state.InitialSnapshot.LedgerIDs)
			state.CurrentSnapshot.Identity = snapshotIdentity(state.CurrentSnapshot.Kind, state.CurrentSnapshot.BaseTree, state.CurrentSnapshot.CandidateTree, state.CurrentSnapshot.PathsDigest, state.CurrentSnapshot.IntendedUntrackedProof, state.CurrentSnapshot.IntendedUntracked, state.CurrentSnapshot.LedgerIDs)
			record, payload, err := makeCompactRecord(state)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := parseCompactRecord(payload, state.LineageID); err == nil || strings.Contains(err.Error(), "checksum mismatch") {
				t.Fatalf("checksum-valid impossible state parse error = %v", err)
			}
			store, _ := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
			if err := os.MkdirAll(store.Dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(store.StatePath(), payload, 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := store.Load(); err == nil || strings.Contains(err.Error(), "checksum mismatch") {
				t.Fatalf("checksum-valid impossible current load error = %v", err)
			}
			_ = os.RemoveAll(store.Dir)
			transport := CompactTransport{Schema: CompactTransportSchema, Record: record}
			transport.BundleDigest = compactTransportDigest(transport)
			transportPayload, _ := json.Marshal(transport)
			if _, err := ParseCompactTransport(transportPayload); err == nil || strings.Contains(err.Error(), "checksum mismatch") {
				t.Fatalf("checksum-valid impossible transport parse error = %v", err)
			}
			if _, err := ImportCompactTransport(context.Background(), repo, transport); err == nil || strings.Contains(err.Error(), "checksum mismatch") {
				t.Fatalf("checksum-valid impossible import error = %v", err)
			}
		})
	}
}

func TestCompactStoreRejectsConcurrentLockedWriter(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	state := newCompactTestState(t, repo, "compact-locked")
	store, _ := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	lock, err := acquireStoreLock(store.lockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.release()
	if _, err := store.Replace("", "review/start", state); !errors.Is(err, ErrConcurrentUpdate) {
		t.Fatalf("concurrent compact writer error = %v", err)
	}
}

func newCompactRevisionState(t *testing.T, repo, lineage string) CompactState {
	t.Helper()
	commit := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetExactRevision, Revision: commit})
	if err != nil {
		t.Fatal(err)
	}
	risk, lines, err := (SnapshotBuilder{Repo: repo}).ClassifySnapshotRisk(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	lenses := []string{}
	if risk == RiskMedium {
		lenses = []string{LensReliability}
	} else if risk == RiskHigh {
		lenses = append([]string(nil), supportedLenses...)
	}
	state, err := NewCompactState(Start{LineageID: lineage, Mode: ModeOrdinaryBounded, Generation: 1, Snapshot: snapshot, PolicyHash: hash("1"), RiskLevel: risk, SelectedLenses: lenses, OriginalChangedLines: &lines})
	if err != nil {
		t.Fatal(err)
	}
	return state
}

// TestCompactTransportRoundTripRecoversEquivalentCurrentAuthority is adapted
// from dev: CompactTransport no longer carries a Receipt (retired), so the
// receipt build/write/compare lines are dropped and the final "conflicting
// artifact at the destination is protected" case is retargeted from a
// tampered receipt FILE onto a genuinely different STATE at the destination
// lineage -- the same underlying protection
// (installTransportRecordLocked refuses to silently overwrite an existing,
// non-identical destination authority), just exercised through the current
// single persistence surface instead of the retired receipt file.
func TestCompactTransportRoundTripRecoversEquivalentCurrentAuthority(t *testing.T) {
	source := initSnapshotRepo(t)
	writeSnapshotFile(t, source, "tracked.txt", "candidate\n")
	gitSnapshot(t, source, "add", "tracked.txt")
	gitSnapshot(t, source, "commit", "-m", "candidate")
	state := newCompactRevisionState(t, source, "compact-transport")
	store, _ := CompactAuthoritativeStore(context.Background(), source, state.LineageID)
	if _, err := store.Replace("", "review/start", state); err != nil {
		t.Fatal(err)
	}
	results := make([]LensResult, len(state.SelectedLenses))
	for index, lens := range state.SelectedLenses {
		results[index] = LensResult{Lens: lens, Findings: []Finding{}, Evidence: []string{"review completed"}}
	}
	// Admitted capture replaces dev's raw CompleteReview call, and
	// CloseCleanReviewOnLastEvent replaces the retired CompleteVerification:
	// both transitions are applied in-memory, then published in one CAS write.
	state, captured := captureAndCompleteCompactReview(t, store, state, CompactReviewInput{LensResults: results, Classifications: []FindingEvidence{}, RefuterOutcomes: []EvidenceResult{}})
	if err := state.CloseCleanReviewOnLastEvent(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace(captured.Revision, "review/complete-review", state); err != nil {
		t.Fatal(err)
	}
	transport, err := store.ExportTransport()
	if err != nil {
		t.Fatal(err)
	}

	destination := filepath.Join(t.TempDir(), "clone")
	gitSnapshot(t, source, "clone", "--no-local", source, destination)
	imported, err := ImportCompactTransport(context.Background(), destination, transport)
	if err != nil {
		t.Fatal(err)
	}
	destinationStore, _ := CompactAuthoritativeStore(context.Background(), destination, state.LineageID)
	destinationTransport, err := destinationStore.ExportTransport()
	if err != nil {
		t.Fatal(err)
	}
	if imported.Revision != transport.Record.Revision || !reflect.DeepEqual(destinationTransport.Record, transport.Record) {
		t.Fatalf("compact transport round trip changed authority")
	}
	if _, err := ImportCompactTransport(context.Background(), destination, transport); err != nil {
		t.Fatalf("exact compact transport retry: %v", err)
	}
	beforeConflict, err := os.ReadFile(destinationStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	conflicting := transport
	conflictingState := conflicting.Record.State
	conflictingState.PolicyHash = hash("conflicting-transport-policy")
	conflictingRecord, _, err := makeCompactRecord(conflictingState)
	if err != nil {
		t.Fatal(err)
	}
	conflicting.Record = conflictingRecord
	conflicting.BundleDigest = compactTransportDigest(conflicting)
	if _, err := ImportCompactTransport(context.Background(), destination, conflicting); !errors.Is(err, ErrConcurrentUpdate) {
		t.Fatalf("conflicting compact transport import error = %v, want ErrConcurrentUpdate", err)
	}
	afterConflict, err := os.ReadFile(destinationStore.StatePath())
	if err != nil || !bytes.Equal(afterConflict, beforeConflict) {
		t.Fatalf("conflicting compact transport import mutated existing destination authority: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destinationStore.Dir, "events")); !os.IsNotExist(err) {
		t.Fatalf("compact import reconstructed event history: %v", err)
	}
}

// TestCompactTransportImportRejectsWrongDeliveredTreeAndScope is adapted from
// dev only by dropping the retired receipt build and the Receipt field
// (CompactTransport no longer carries one); the two invariants under test
// (wrong delivered tree, wrong delivered path scope) are both still enforced
// by validateCompactTransportDelivery independent of any receipt.
func TestCompactTransportImportRejectsWrongDeliveredTreeAndScope(t *testing.T) {
	source := initSnapshotRepo(t)
	state := correctedCompactTestState(t, source, "compact-transport-binding")
	gitSnapshot(t, source, "add", "tracked.txt")
	gitSnapshot(t, source, "commit", "-m", "corrected candidate")
	tests := []struct {
		name   string
		mutate func(*CompactState)
		want   string
	}{
		{name: "wrong delivered tree", want: "delivered tree", mutate: func(candidate *CompactState) {
			candidate.CurrentSnapshot.CandidateTree = candidate.InitialSnapshot.BaseTree
			candidate.FixDeltaHash = FixDeltaHashForSnapshot(candidate.CurrentSnapshot)
		}},
		{name: "wrong delivered path scope", want: "path scope", mutate: func(candidate *CompactState) {
			candidate.InitialSnapshot.Paths = []string{"other.txt"}
			candidate.InitialSnapshot.PathsDigest = digestPaths(candidate.InitialSnapshot.Paths)
			candidate.GenesisPaths = append([]string(nil), candidate.InitialSnapshot.Paths...)
			candidate.CurrentSnapshot.Paths = []string{"other.txt"}
			candidate.CurrentSnapshot.PathsDigest = digestPaths(candidate.CurrentSnapshot.Paths)
			candidate.FixDeltaHash = FixDeltaHashForSnapshot(candidate.CurrentSnapshot)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := state
			candidate.InitialSnapshot.Paths = append([]string(nil), state.InitialSnapshot.Paths...)
			candidate.CurrentSnapshot.Paths = append([]string(nil), state.CurrentSnapshot.Paths...)
			candidate.GenesisPaths = append([]string(nil), state.GenesisPaths...)
			tt.mutate(&candidate)
			candidate.InitialSnapshot.Identity = snapshotIdentity(candidate.InitialSnapshot.Kind, candidate.InitialSnapshot.BaseTree, candidate.InitialSnapshot.CandidateTree, candidate.InitialSnapshot.PathsDigest, candidate.InitialSnapshot.IntendedUntrackedProof, candidate.InitialSnapshot.IntendedUntracked, candidate.InitialSnapshot.LedgerIDs)
			candidate.CurrentSnapshot.Identity = snapshotIdentity(candidate.CurrentSnapshot.Kind, candidate.CurrentSnapshot.BaseTree, candidate.CurrentSnapshot.CandidateTree, candidate.CurrentSnapshot.PathsDigest, candidate.CurrentSnapshot.IntendedUntrackedProof, candidate.CurrentSnapshot.IntendedUntracked, candidate.CurrentSnapshot.LedgerIDs)
			candidate.OriginalCriteria.FixDeltaHash = candidate.FixDeltaHash
			candidate.CorrectionRegression.FixDeltaHash = candidate.FixDeltaHash
			if err := candidate.Validate(); err != nil {
				t.Fatalf("test candidate must remain checksum-valid and semantically self-consistent: %v", err)
			}
			record, _, err := makeCompactRecord(candidate)
			if err != nil {
				t.Fatal(err)
			}
			transport := CompactTransport{Schema: CompactTransportSchema, Record: record}
			transport.BundleDigest = compactTransportDigest(transport)
			clone := filepath.Join(t.TempDir(), "clone")
			gitSnapshot(t, source, "clone", "--no-local", source, clone)
			if _, err := ImportCompactTransport(context.Background(), clone, transport); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("wrong compact delivery import error = %v", err)
			}
		})
	}
}

func TestCompactDiagnosticTraceContainsMetadataOnly(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	state := newCompactTestState(t, repo, "compact-trace")
	store, _ := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	store.TracePath = filepath.Join(t.TempDir(), "trace.jsonl")
	if _, err := store.Replace("", "review/start", state); err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(store.TracePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "initial_snapshot") || strings.Contains(string(payload), "findings") || !strings.Contains(string(payload), `"operation":"review/start"`) {
		t.Fatalf("diagnostic trace contains authority snapshot or lacks metadata: %s", payload)
	}
}

// TestReplaceContextGuardedReportsTraceWriteFailureInsteadOfSwallowingIt
// covers issue #1854: a caller supplying TracePath asked for observability,
// so a failed trace write must be reported even though the mutation itself
// has already committed and must not be rolled back or fail.
func TestReplaceContextGuardedReportsTraceWriteFailureInsteadOfSwallowingIt(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	state := newCompactTestState(t, repo, "compact-trace-replace-fail")
	store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// TracePath's parent directory is a regular file, so appendCompactTrace's
	// os.MkdirAll must fail deterministically.
	store.TracePath = filepath.Join(blocker, "trace.jsonl")

	var gotOperation, gotPath string
	var gotErr error
	original := compactTraceWarn
	t.Cleanup(func() { compactTraceWarn = original })
	compactTraceWarn = func(operation, path string, err error) {
		gotOperation, gotPath, gotErr = operation, path, err
	}

	revision, err := store.Replace("", "review/start", state)
	if err != nil {
		t.Fatalf("a lost review trace must not fail the mutation: %v", err)
	}
	if revision == "" {
		t.Fatal("expected a committed revision despite the trace write failure")
	}
	if gotErr == nil {
		t.Fatal("expected the trace write failure to be reported instead of swallowed")
	}
	if gotOperation != "review/start" {
		t.Fatalf("wrong operation reported for the lost trace: %q", gotOperation)
	}
	if gotPath != store.TracePath {
		t.Fatalf("wrong path reported for the lost trace: %q", gotPath)
	}
}

func TestSnapshotCandidateLocationSupportsStructuredCausality(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "same\nold\nkeep\nremoved\nstable\n")
	gitSnapshot(t, repo, "add", "tracked.txt")
	gitSnapshot(t, repo, "commit", "-m", "line evidence base")
	base := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	writeSnapshotFile(t, repo, "tracked.txt", "same\nnew\nkeep\nstable\nadded\n")
	if err := os.Remove(filepath.Join(repo, "deleted.txt")); err != nil {
		t.Fatal(err)
	}
	gitSnapshot(t, repo, "add", "-A")
	gitSnapshot(t, repo, "commit", "-m", "line evidence candidate")
	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetBaseDiff, BaseRef: base, IntendedUntracked: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name      string
		location  string
		causality CausalDisposition
		want      bool
		wantError FindingLocationErrorReason
	}{{"introduced replacement", "tracked.txt:2", CausalIntroduced, true, ""}, {"introduced addition", "tracked.txt:5", CausalIntroduced, true, ""}, {"introduced deletion", "deleted.txt:1", CausalIntroduced, false, ""}, {"old-side deletion collision", "tracked.txt:4", CausalIntroduced, false, ""}, {"introduced unchanged", "tracked.txt:1", CausalIntroduced, false, ""}, {"worsened changed", "tracked.txt:2", CausalWorsened, true, ""}, {"worsened unchanged", "tracked.txt:1", CausalWorsened, false, ""}, {"activated unchanged", "tracked.txt:1", CausalBehaviorActivated, true, ""}, {"activated deletion", "deleted.txt:1", CausalBehaviorActivated, false, ""}, {"activated out of range", "tracked.txt:99", CausalBehaviorActivated, false, ""}, {"outside genesis", "other.txt:1", CausalBehaviorActivated, false, ""}, {"range", "tracked.txt:1-2", CausalIntroduced, false, "line_suffix_not_integer"}, {"non-numeric", "tracked.txt:one", CausalIntroduced, false, "line_suffix_not_integer"}, {"overflow", "tracked.txt:" + strings.Repeat("9", 64), CausalIntroduced, false, "line_suffix_not_integer"}, {"leading plus", "tracked.txt:+1", CausalIntroduced, false, "line_suffix_not_integer"}, {"zero", "tracked.txt:0", CausalIntroduced, false, "line_must_be_positive"}, {"negative", "tracked.txt:-1", CausalIntroduced, false, "line_must_be_positive"}, {"colon traversal", "internal:../tracked.txt:1", CausalIntroduced, false, "path_must_be_canonical"}, {"missing suffix", "tracked.txt:", CausalWorsened, false, "expected_path_and_line"}, {"malformed", "tracked.txt", CausalWorsened, false, "expected_path_and_line"}} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := (SnapshotBuilder{Repo: repo}).CandidateLocationSupportsCausality(context.Background(), snapshot, tt.location, tt.causality)
			if tt.wantError == "" && (err != nil || got != tt.want) {
				t.Fatalf("CandidateLocationSupportsCausality(%q, %q) = %t, %v", tt.location, tt.causality, got, err)
			}
			if tt.wantError != "" {
				var locationErr *FindingLocationError
				if got || !errors.Is(err, ErrInvalidFindingLocation) || !errors.As(err, &locationErr) || locationErr.Reason != tt.wantError {
					t.Fatalf("CandidateLocationSupportsCausality(%q) = %t, %v; want typed %q", tt.location, got, err, tt.wantError)
				}
			}
		})
	}
}
