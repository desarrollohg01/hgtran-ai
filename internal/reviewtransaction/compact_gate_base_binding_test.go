package reviewtransaction

import (
	"strings"
	"testing"
)

// TestGateContextRoundTripsBaseBindingDiagnostics keeps the additive fields
// inside the same guarded validation every other optional block gets. Persisted
// invalidation evidence is re-parsed with DisallowUnknownFields and compared by
// value, so an unvalidated field would be a hole in that round trip.
func TestGateContextRoundTripsBaseBindingDiagnostics(t *testing.T) {
	base := strings.Repeat("a", 40)
	actual := strings.Repeat("b", 40)
	candidate := strings.Repeat("c", 40)
	digest := "sha256:" + strings.Repeat("d", 64)
	valid := `{"gate":"pre-pr","lineage_id":"lineage-one","generation":1,"store_revision":"` + digest +
		`","genesis_revision":"` + digest + `","chain_identity":"` + digest + `","bundle_digest":"` + digest +
		`","base_tree":"` + actual + `","candidate_tree":"` + candidate + `","paths_digest":"` + digest +
		`","fix_delta_hash":"` + digest + `","policy_hash":"` + digest + `","ledger_hash":"` + digest +
		`","evidence_hash":"` + digest + `","base_relationship_valid":false,"receipt_base_tree":"` + base +
		`","denial":{"stage":"receipt-binding","code":"base-mismatch"},"base_mismatch":{"expected":"` + base +
		`","actual":"` + actual + `"}}`
	parsed, err := ParseGateContext([]byte(valid))
	if err != nil {
		t.Fatalf("valid base-binding context: %v", err)
	}
	if parsed.ReceiptBaseTree != base || parsed.BaseMismatch == nil ||
		parsed.BaseMismatch.Expected != base || parsed.BaseMismatch.Actual != actual {
		t.Fatalf("parsed base-binding context = %#v", parsed)
	}

	for name, payload := range map[string]string{
		"base mismatch without the matching denial code": strings.Replace(valid, `"code":"base-mismatch"`, `"code":"candidate-or-paths-mismatch"`, 1),
		"base mismatch actual disagrees with base_tree":  strings.Replace(valid, `"actual":"`+actual+`"}}`, `"actual":"`+candidate+`"}}`, 1),
		"base mismatch expected is not a tree hash":      strings.Replace(valid, `"expected":"`+base+`"`, `"expected":"not-a-tree"`, 1),
		"receipt base tree repeats base_tree":            strings.Replace(valid, `"receipt_base_tree":"`+base+`"`, `"receipt_base_tree":"`+actual+`"`, 1),
		"receipt base tree is not a tree hash":           strings.Replace(valid, `"receipt_base_tree":"`+base+`"`, `"receipt_base_tree":"not-a-tree"`, 1),
	} {
		if _, err := ParseGateContext([]byte(payload)); err == nil {
			t.Fatalf("%s parsed as a valid gate context", name)
		}
	}
}
