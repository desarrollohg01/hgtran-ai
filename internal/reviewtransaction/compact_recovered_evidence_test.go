package reviewtransaction

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// TestAccountingOnlyRecoveryImportsExactPredecessorEvidenceIntoValidatingSuccessor
// proves RecoverCompactAuthority's accounting-only evidence-import path: an
// escalated predecessor whose only failure was correction-line accounting
// (both original-criteria and correction-regression checks passed) recovers
// into a validating successor that carries the predecessor's exact admitted
// review evidence and repository-derived correction accounting, without
// requiring (or mutating) anything else.
func TestAccountingOnlyRecoveryImportsExactPredecessorEvidenceIntoValidatingSuccessor(t *testing.T) {
	repo := initSnapshotRepo(t)
	predecessor := accountingOnlyRecoverableEscalatedState(t, repo, "accounting-only-predecessor")
	predecessorStore, predecessorRecord := persistEscalatedRecoveryFixture(t, repo, predecessor)

	successor := recoveredEvidenceSuccessor(t, repo, predecessor, "accounting-only-successor")
	const actor = "maintainer@example.com"
	const reason = "recover repository-derived correction after historical line-accounting escalation"
	authorization := compactRecoveryAuthorizationBinding(
		predecessor.LineageID, predecessorRecord.Revision, successor.InitialSnapshot.Identity, actor, reason,
	)
	recovered, err := RecoverCompactAuthority(context.Background(), repo, CompactRecoveryRequest{
		PredecessorLineageID: predecessor.LineageID, ExpectedPredecessorRevision: predecessorRecord.Revision,
		Successor: successor, Disposition: RecoveryEscalated, Reason: reason, Actor: actor,
		MaintainerAuthorization: authorization,
	})
	if err != nil {
		t.Fatal(err)
	}
	if recovered.State.State != StateValidating || recovered.State.Recovery == nil || recovered.State.Recovery.Evidence == nil {
		t.Fatalf("recovered state = %#v", recovered.State)
	}
	evidence := recovered.State.Recovery.Evidence
	attempt := predecessorRecord.State.CorrectionAttempts[len(predecessorRecord.State.CorrectionAttempts)-1]
	request, err := rebuildCompactRecoveredTargetedValidationRequest(predecessorRecord.State, *evidence)
	if err != nil {
		t.Fatalf("rebuild targeted validation request: %v", err)
	}
	// The fixture's correction rewrites tracked.txt's final line only ("four"
	// -> "fixed"): one repository-derived changed line, counted here as one
	// removal plus one addition (2), even though the predecessor's recorded
	// (accounted) total was budget+1 (3). That gap between the accounted total
	// and the true repository-derived diff is exactly what makes this
	// escalation accounting-only and recoverable.
	if evidence.NativeCorrectionLines != 2 || evidence.NativeCorrectionLines > predecessorRecord.State.CorrectionBudget ||
		evidence.NativeCorrectionLines >= attempt.ActualLines ||
		request.ExpectedRevision != predecessorRecord.State.CapturePhaseRevision ||
		request.TargetIdentity != predecessor.InitialSnapshot.Identity ||
		request.CorrectionTargetIdentity != predecessorRecord.State.CurrentSnapshot.Identity ||
		!reflect.DeepEqual(recovered.State.AdmittedRoleResults, predecessorRecord.State.AdmittedRoleResults) ||
		recovered.State.FixDeltaHash != predecessor.FixDeltaHash ||
		recovered.State.ActualCorrectionLines == nil || *recovered.State.ActualCorrectionLines != evidence.NativeCorrectionLines ||
		len(recovered.State.CorrectionAttempts) != 0 || !snapshotsEqual(recovered.State.InitialSnapshot, recovered.State.CurrentSnapshot) {
		t.Fatalf("imported recovery evidence = %#v, state=%#v", evidence, recovered.State)
	}
	if err := validateCompactRecoveryEdge(predecessorRecord, recovered.State); err != nil {
		t.Fatalf("recovered edge: %v", err)
	}

	unchangedPredecessor, err := predecessorStore.Load()
	if err != nil || unchangedPredecessor.Revision != predecessorRecord.Revision ||
		!compactStateEqual(unchangedPredecessor.State, predecessorRecord.State) {
		t.Fatalf("recovery mutated predecessor: err=%v before=%#v after=%#v", err, predecessorRecord, unchangedPredecessor)
	}
}

// accountingOnlyRecoverableEscalatedState builds an escalated authority whose
// only failure is correction-line accounting: the recorded correction actual
// exceeds the frozen budget by exactly one line, but the repository-derived
// native diff for that same correction attempt is a strictly smaller,
// single-line replacement that stays within both the proposed lines and the
// frozen budget. That gap between the (wrong) recorded accounting and the
// (true) repository evidence is exactly what RecoverCompactAuthority's
// accounting-only import path exists to repair. It intentionally builds its
// own fixture (rather than reusing the shared accountingOnlyEscalatedState in
// compact_escalation_accounting_test.go) because that shared fixture pins its
// budget/proposed/actual at 1/1/2 for EscalationAccounting() reporting tests,
// which makes the native and accounted lines equal — ineligible for recovery.
func accountingOnlyRecoverableEscalatedState(t *testing.T, repo, lineage string) CompactState {
	t.Helper()
	writeSnapshotFile(t, repo, "tracked.txt", "base\none\ntwo\nthree\nfour\nfive\nsix\nseven\n")
	state := newCompactTestState(t, repo, lineage)
	t.Logf("DEBUG risk=%q originalChangedLines=%d budget=%d lenses=%v", state.RiskLevel, state.OriginalChangedLines, state.CorrectionBudget, state.SelectedLenses)
	if state.CorrectionBudget < 2 || len(state.SelectedLenses) != 1 {
		t.Fatalf("fixture risk/budget = %q/%d", state.RiskLevel, state.CorrectionBudget)
	}
	store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace("", "review/start", state); err != nil {
		t.Fatal(err)
	}
	finding := Finding{
		ID: "R3-001", Lens: strings.TrimPrefix(state.SelectedLenses[0], "review-"), Location: "tracked.txt:5", Severity: "CRITICAL",
		Claim: "candidate retains the wrong value", ProofRefs: []string{"candidate-only differential failure"},
		EvidenceClass: EvidenceDeterministic, CausalDisposition: CausalIntroduced,
	}
	state, _ = captureAndCompleteCompactReview(t, store, state, CompactReviewInput{
		LensResults:     []LensResult{{Lens: state.SelectedLenses[0], Findings: []Finding{finding}, Evidence: []string{"reviewed exact candidate tree"}}},
		Classifications: []FindingEvidence{{FindingID: finding.ID, Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "changed hunk"}},
		RefuterOutcomes: []EvidenceResult{},
	})
	if err := state.BeginCorrection(2); err != nil {
		t.Fatal(err)
	}
	writeSnapshotFile(t, repo, "tracked.txt", "base\none\ntwo\nthree\nfixed\n")
	fix, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
		Kind: TargetFixDiff, BaseRef: state.CurrentSnapshot.CandidateTree,
		IntendedUntracked: state.InitialSnapshot.IntendedUntracked, LedgerIDs: state.FixFindingIDs,
	})
	if err != nil {
		t.Fatal(err)
	}
	fixHash := FixDeltaHashForSnapshot(fix)
	validation := bindTargetedValidationForTest(ScopedValidationResult{
		LedgerIDs: state.FixFindingIDs, FixCausedFindings: []Finding{}, FollowUps: []FollowUp{},
		OriginalCriteria:     ValidationCheck{EvidenceHash: hash("2"), FixDeltaHash: fixHash, Passed: true},
		CorrectionRegression: ValidationCheck{EvidenceHash: hash("3"), FixDeltaHash: fixHash, Passed: true},
	}, fix)
	if err := state.CompleteCorrection(fix, state.CorrectionBudget+1, validation); err != nil {
		t.Fatal(err)
	}
	if !compactAccountingOnlyEscalation(state) {
		t.Fatalf("fixture is not accounting-only escalation: %#v", state)
	}
	return state
}

// TestAccountingOnlyRecoveryFailsClosedOrStartsFreshWhenEvidenceDoesNotMatch
// proves the accounting-only import path only ever engages for the exact
// eligible predecessor/successor pair: a stale expected revision still fails
// closed with ErrConcurrentUpdate, a policy mismatch refuses the import and
// then finds the (unchanged) target already recovered, and a genuinely
// changed target falls through to an ordinary fresh-review successor instead
// of reusing stale evidence.
func TestAccountingOnlyRecoveryFailsClosedOrStartsFreshWhenEvidenceDoesNotMatch(t *testing.T) {
	t.Run("stale predecessor revision", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		predecessor := accountingOnlyEscalatedState(t, repo, "accounting-stale-predecessor")
		_, record := persistEscalatedRecoveryFixture(t, repo, predecessor)
		successor := recoveredEvidenceSuccessor(t, repo, predecessor, "accounting-stale-successor")
		request := CompactRecoveryRequest{
			PredecessorLineageID: predecessor.LineageID, ExpectedPredecessorRevision: hash("8"), Successor: successor,
			Disposition: RecoveryEscalated, Reason: "stale", Actor: "maintainer@example.com",
			MaintainerAuthorization: compactRecoveryAuthorizationBinding(predecessor.LineageID, record.Revision, successor.InitialSnapshot.Identity, "maintainer@example.com", "stale"),
		}
		if _, err := RecoverCompactAuthority(context.Background(), repo, request); !errors.Is(err, ErrConcurrentUpdate) {
			t.Fatalf("stale recovery error = %v", err)
		}
	})

	t.Run("policy mismatch cannot import", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		predecessor := accountingOnlyEscalatedState(t, repo, "accounting-policy-predecessor")
		_, record := persistEscalatedRecoveryFixture(t, repo, predecessor)
		successor := recoveredEvidenceSuccessor(t, repo, predecessor, "accounting-policy-successor")
		successor.PolicyHash = hash("7")
		const actor, reason = "maintainer@example.com", "policy mismatch"
		_, err := RecoverCompactAuthority(context.Background(), repo, CompactRecoveryRequest{
			PredecessorLineageID: predecessor.LineageID, ExpectedPredecessorRevision: record.Revision, Successor: successor,
			Disposition: RecoveryEscalated, Reason: reason, Actor: actor,
			MaintainerAuthorization: compactRecoveryAuthorizationBinding(predecessor.LineageID, record.Revision, successor.InitialSnapshot.Identity, actor, reason),
		})
		if err == nil || !strings.Contains(err.Error(), "target has not changed") {
			t.Fatalf("policy-mismatched evidence import error = %v", err)
		}
	})

	t.Run("changed bytes use fresh review", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		predecessor := accountingOnlyEscalatedState(t, repo, "accounting-changed-predecessor")
		_, record := persistEscalatedRecoveryFixture(t, repo, predecessor)
		writeSnapshotFile(t, repo, "tracked.txt", "base\none\ntwo\nthree\nchanged again\n")
		successor := recoveredEvidenceSuccessor(t, repo, predecessor, "accounting-changed-successor")
		const actor, reason = "maintainer@example.com", "changed candidate recovery"
		recovered, err := RecoverCompactAuthority(context.Background(), repo, CompactRecoveryRequest{
			PredecessorLineageID: predecessor.LineageID, ExpectedPredecessorRevision: record.Revision, Successor: successor,
			Disposition: RecoveryEscalated, Reason: reason, Actor: actor,
			MaintainerAuthorization: compactRecoveryAuthorizationBinding(predecessor.LineageID, record.Revision, successor.InitialSnapshot.Identity, actor, reason),
		})
		if err != nil {
			t.Fatal(err)
		}
		if recovered.State.State != StateReviewing || recovered.State.Recovery.Evidence != nil || len(recovered.State.AdmittedRoleResults) != 0 {
			t.Fatalf("changed target reused predecessor evidence: %#v", recovered.State)
		}
	})
}

func TestCompactRecoveredEvidenceOwnsOnlyAccountingReferences(t *testing.T) {
	typeOfEvidence := reflect.TypeOf(CompactRecoveredEvidence{})
	for _, field := range []string{
		"SourceCorrectionAttempt",
		"TargetedValidationRequest",
		"ReviewEvidenceHash",
		"SuccessorTargetIdentity",
	} {
		if _, found := typeOfEvidence.FieldByName(field); found {
			t.Fatalf("recovered evidence still owns forbidden copied field %q", field)
		}
	}
	for _, field := range []string{"PredecessorTargetIdentity", "AdmittedRoleReferences"} {
		if _, found := typeOfEvidence.FieldByName(field); !found {
			t.Fatalf("recovered evidence is missing accounting field %q", field)
		}
	}
	typeOfReference := reflect.TypeOf(CompactRecoveredEvidenceReference{})
	for _, field := range []string{"Role", "Lens", "SelectedOrder", "TargetIdentity", "CapturePhaseRevision", "RequestHash", "ArtifactDigest"} {
		if _, found := typeOfReference.FieldByName(field); !found {
			t.Fatalf("recovered evidence reference is missing binding field %q", field)
		}
	}
	for _, field := range []string{"Value", "Path", "Digest", "ResultHash"} {
		if _, found := typeOfReference.FieldByName(field); found {
			t.Fatalf("recovered evidence reference owns forbidden payload or duplicate digest field %q", field)
		}
	}
}

func TestCompactRecoveredEvidenceReferencesRejectNonCanonicalBindings(t *testing.T) {
	predecessor, evidence, request := accountingRecoveryReferenceFixture(t)
	if err := validateCompactRecoveredEvidenceReferences(predecessor, evidence); err != nil {
		t.Fatalf("valid accounting references: %v", err)
	}
	rebuilt, err := rebuildCompactRecoveredTargetedValidationRequest(predecessor, evidence)
	if err != nil {
		t.Fatalf("rebuild targeted validation request: %v", err)
	}
	if !reflect.DeepEqual(rebuilt, request) || rebuilt.ExpectedRevision != predecessor.CapturePhaseRevision {
		t.Fatalf("recovered request = %#v, want frozen Pn-bound request %#v", rebuilt, request)
	}
	liveRevision, err := CompactRevisionForState(predecessor)
	if err != nil || rebuilt.ExpectedRevision == liveRevision {
		t.Fatalf("recovered request bound live Rn %q instead of frozen Pn %q: %v", liveRevision, predecessor.CapturePhaseRevision, err)
	}

	cases := map[string]func(*CompactRecoveredEvidence){
		"duplicate": func(value *CompactRecoveredEvidence) {
			value.AdmittedRoleReferences = append(value.AdmittedRoleReferences, value.AdmittedRoleReferences[0])
		},
		"unordered": func(value *CompactRecoveredEvidence) {
			value.AdmittedRoleReferences[0], value.AdmittedRoleReferences[1] = value.AdmittedRoleReferences[1], value.AdmittedRoleReferences[0]
		},
		"unknown digest": func(value *CompactRecoveredEvidence) {
			value.AdmittedRoleReferences[0].ArtifactDigest = hash("a")
		},
		"stale predecessor target": func(value *CompactRecoveredEvidence) {
			value.PredecessorTargetIdentity = hash("b")
		},
		"stale target": func(value *CompactRecoveredEvidence) {
			value.AdmittedRoleReferences[0].TargetIdentity = hash("b")
		},
		"stale phase": func(value *CompactRecoveredEvidence) {
			value.AdmittedRoleReferences[0].CapturePhaseRevision = hash("c")
		},
		"stale validator phase": func(value *CompactRecoveredEvidence) {
			value.AdmittedRoleReferences[len(value.AdmittedRoleReferences)-1].CapturePhaseRevision = hash("c")
		},
		"missing admitted entry": func(value *CompactRecoveredEvidence) {
			value.AdmittedRoleReferences = value.AdmittedRoleReferences[:len(value.AdmittedRoleReferences)-1]
		},
		"stale validator request": func(value *CompactRecoveredEvidence) {
			value.AdmittedRoleReferences[len(value.AdmittedRoleReferences)-1].RequestHash = hash("d")
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			changed := cloneCompactRecoveredEvidence(evidence)
			mutate(&changed)
			if err := validateCompactRecoveredEvidenceReferences(predecessor, changed); err == nil {
				t.Fatalf("accepted %s accounting references: %#v", name, changed)
			}
			if _, err := rebuildCompactRecoveredTargetedValidationRequest(predecessor, changed); err == nil {
				t.Fatalf("rebuilt targeted request from %s accounting references", name)
			}
		})
	}
}

func TestCompactRecoveredEvidenceReferencesAreNotActiveCaptureSlots(t *testing.T) {
	predecessor, evidence, _ := accountingRecoveryReferenceFixture(t)
	successor := newCompactTestState(t, initSnapshotRepo(t), "accounting-reference-classification")
	successor.Recovery = &CompactRecoveryProvenance{Evidence: &evidence}
	for _, admitted := range predecessor.AdmittedRoleResults {
		if !compactAdmittedRoleResultIsAccountingOnly(successor, admitted) {
			t.Fatalf("accounting reference did not classify admitted role result: %#v", admitted)
		}
		if compactAdmittedRoleResultCanSatisfyActiveCapture(successor, admitted) {
			t.Fatalf("accounting reference satisfied an active capture slot: %#v", admitted)
		}
	}
	fresh := predecessor.AdmittedRoleResults[0]
	fresh.ArtifactDigest = hash("e")
	if compactAdmittedRoleResultIsAccountingOnly(successor, fresh) || !compactAdmittedRoleResultCanSatisfyActiveCapture(successor, fresh) {
		t.Fatalf("unreferenced role result classification = accounting=%v active=%v", compactAdmittedRoleResultIsAccountingOnly(successor, fresh), compactAdmittedRoleResultCanSatisfyActiveCapture(successor, fresh))
	}
}

func accountingRecoveryReferenceFixture(t *testing.T) (CompactState, CompactRecoveredEvidence, TargetedValidationRequest) {
	t.Helper()
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "base\nwrong\n")
	predecessor := newCompactTestState(t, repo, "accounting-reference-predecessor")
	policy := "frozen accounting recovery policy"
	predecessor.FrozenPolicyContent, predecessor.PolicyHash = &policy, compactPolicyContentHash(policy)
	predecessor.CorrectionBudget, predecessor.CorrectionBudgetPolicy = 1, ""
	phase, err := deriveCompactCapturePhaseRevision(predecessor)
	if err != nil {
		t.Fatal(err)
	}
	predecessor.CapturePhaseRevision = phase
	store, err := CompactAuthoritativeStore(t.Context(), repo, predecessor.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	record := writeCompactFixtureRecord(t, store, predecessor)
	finding := Finding{
		ID: "R3-001", Lens: "reliability", Location: "tracked.txt:2", Severity: "CRITICAL",
		Claim: "wrong value", ProofRefs: []string{"changed hunk causes failure"},
		EvidenceClass: EvidenceDeterministic, CausalDisposition: CausalIntroduced,
	}
	captureAdmittedCorrectionFinding(t, store, predecessor, finding)
	record, err = store.Load()
	if err != nil {
		t.Fatal(err)
	}
	predecessor = record.State
	view, err := predecessor.CompactReviewView()
	if err != nil {
		t.Fatal(err)
	}
	if err := predecessor.CompleteReview(CompactReviewInput{
		LensResults: view.LensResults, Classifications: []FindingEvidence{view.Classifications[finding.ID]},
	}); err != nil {
		t.Fatal(err)
	}
	record.Revision, err = store.Replace(record.Revision, "review/complete-review", predecessor)
	if err != nil {
		t.Fatal(err)
	}
	if err := predecessor.BeginCorrection(1); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace(record.Revision, "review/begin-fix", predecessor); err != nil {
		t.Fatal(err)
	}
	writeSnapshotFile(t, repo, "tracked.txt", "base\nfixed\n")
	fix, err := (SnapshotBuilder{Repo: repo}).Build(t.Context(), Target{
		Kind: TargetFixDiff, Projection: predecessor.InitialSnapshot.Projection,
		BaseRef: predecessor.CurrentSnapshot.CandidateTree, IntendedUntracked: predecessor.InitialSnapshot.IntendedUntracked,
		LedgerIDs: predecessor.FixFindingIDs,
	})
	if err != nil {
		t.Fatal(err)
	}
	request, err := BuildTargetedValidationRequestFromSnapshot(t.Context(), repo, predecessor, predecessor.CapturePhaseRevision, fix)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CaptureAdmittedTargetedValidatorResult(t.Context(), CompactAdmittedTargetedValidatorResultRequest{
		ExpectedRequest: request, Payload: []byte(`{"outcome":"passed"}`),
	}); err != nil {
		t.Fatal(err)
	}
	record, err = store.Load()
	if err != nil {
		t.Fatal(err)
	}
	predecessor = record.State
	fixHash := FixDeltaHashForSnapshot(fix)
	validation := ScopedValidationResult{
		LedgerIDs: predecessor.FixFindingIDs, FixCausedFindings: []Finding{}, FollowUps: []FollowUp{},
		OriginalCriteria:              ValidationCheck{EvidenceHash: hash("2"), FixDeltaHash: fixHash, Passed: true},
		CorrectionRegression:          ValidationCheck{EvidenceHash: hash("3"), FixDeltaHash: fixHash, Passed: true},
		TargetedValidationRequestHash: request.RequestHash, CorrectionTargetIdentity: fix.Identity,
	}
	if err := predecessor.CompleteCorrection(fix, 1, validation); err != nil {
		t.Fatal(err)
	}
	actual := 2
	predecessor.State = StateEscalated
	predecessor.CorrectionAttempts[0].ActualLines = actual
	predecessor.CumulativeCorrectionLines, predecessor.ActualCorrectionLines = actual, &actual
	if err := predecessor.Validate(); err != nil {
		t.Fatalf("validate accounting predecessor: %v", err)
	}
	evidence := CompactRecoveredEvidence{
		Schema:                    CompactRecoveredEvidenceSchema,
		Relation:                  string(compactTargetChangedScope),
		PathRelation:              string(compactPathsSame),
		PredecessorTargetIdentity: predecessor.InitialSnapshot.Identity,
		NativeCorrectionLines:     1,
		AdmittedRoleReferences:    compactRecoveredEvidenceReferences(predecessor),
	}
	return predecessor, evidence, request
}

func cloneCompactRecoveredEvidence(value CompactRecoveredEvidence) CompactRecoveredEvidence {
	value.AdmittedRoleReferences = append([]CompactRecoveredEvidenceReference(nil), value.AdmittedRoleReferences...)
	return value
}
