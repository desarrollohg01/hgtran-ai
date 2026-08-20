package update

import "testing"

// The fork depends on three tools that originated with Gentleman-Programming
// and that HG has since forked. Until this test existed, the registry still
// pointed all three at the upstream, and TestRegistryContents asserted that
// arrangement as a required invariant — so the green suite guaranteed that a
// build of this fork would check upstream releases and, on a newer tag,
// replace itself with the upstream binary. The rename undone at runtime.
//
// This pins the opposite: anything HG forked resolves to HG.

// forkedTools maps a registry tool name to the HG repository that now owns it.
// Tools genuinely authored by third parties are absent on purpose — they are
// not ours to repoint.
var forkedTools = map[string]string{
	"gentle-ai": "desarrollohg01/hgtran-ai",
	"engram":    "desarrollohg01/engram",
	"hga":       "desarrollohg01/hgtran-guardian-angel",
}

func TestForkedToolsResolveToHG(t *testing.T) {
	for _, tool := range Tools {
		want, isForked := forkedTools[tool.Name]
		if !isForked {
			continue
		}
		got := tool.Owner + "/" + tool.Repo
		if got != want {
			t.Errorf("tool %q resolves to %q, want %q", tool.Name, got, want)
		}
	}
}

// TestNoForkedToolPointsAtTheUpstream is the guard that would have caught the
// original defect. It is deliberately separate from the table above: the table
// says where each tool should be, this says where none of them may be.
func TestNoForkedToolPointsAtTheUpstream(t *testing.T) {
	for _, tool := range Tools {
		if _, isForked := forkedTools[tool.Name]; !isForked {
			continue
		}
		if tool.Owner == "Gentleman-Programming" {
			t.Errorf("tool %q still points at the upstream owner; a release there would replace this fork's binary", tool.Name)
		}
	}
}

// The Guardian Angel fork renamed its command from gga to hga. Detection keys
// on the command name, so a registry that still says "gga" finds nothing on
// PATH and reinstalls the tool on every run.
func TestGuardianAngelIsDetectedByItsCurrentCommandName(t *testing.T) {
	var found *ToolInfo
	for i := range Tools {
		if Tools[i].Name == "hga" {
			found = &Tools[i]
			break
		}
	}
	if found == nil {
		t.Fatal("no tool named \"hga\" in the registry; the Guardian Angel fork installs a command by that name")
	}
	if len(found.DetectCmd) == 0 || found.DetectCmd[0] != "hga" {
		t.Errorf("hga DetectCmd = %v, want it to invoke \"hga\"", found.DetectCmd)
	}
	for _, p := range found.FallbackPaths("/home/u", "") {
		if contains(p, "gga") {
			t.Errorf("hga FallbackPaths still probes a gga path: %q", p)
		}
	}
}
