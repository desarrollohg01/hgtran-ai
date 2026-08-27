package catalog

import (
	"testing"

	"bitbucket.org/hgt_development/hgtran-ai/v2/internal/components/skills"
	"bitbucket.org/hgt_development/hgtran-ai/v2/internal/model"
)

// TestMVPSkillsCoverAllPresetSkills ensures every skill that presets.go would
// install is also registered in the catalog's mvpSkills allowlist. This
// prevents a future addition to sddSkills or foundationSkills from being
// silently rejected by normalizeSkills in cli/validate.go.
func TestMVPSkillsCoverAllPresetSkills(t *testing.T) {
	catalogSet := make(map[model.SkillID]bool)
	for _, s := range MVPSkills() {
		catalogSet[s.ID] = true
	}

	presetSkills := skills.AllSkillIDs()
	for _, id := range presetSkills {
		if !catalogSet[id] {
			t.Errorf("skill %q is in presets but missing from catalog mvpSkills", id)
		}
	}
}

// TestMVPSkillsNoDuplicates ensures no skill is listed twice in mvpSkills.
func TestMVPSkillsNoDuplicates(t *testing.T) {
	seen := make(map[model.SkillID]bool)
	for _, s := range MVPSkills() {
		if seen[s.ID] {
			t.Errorf("duplicate skill %q in mvpSkills", s.ID)
		}
		seen[s.ID] = true
	}
}

func TestMVPSkillsIncludeRequestedBundledSkillsWithCanonicalNames(t *testing.T) {
	required := map[model.SkillID]string{
		model.SkillCreator:           "skill-creator",
		model.SkillSkillRegistry:     "skill-registry",
		model.SkillCognitiveDoc:      "cognitive-doc-design",
		model.SkillCommentWriter:     "comment-writer",
		model.SkillJudgmentDay:       "judgment-day",
		model.SkillSDDInit:           "sdd-init",
		model.SkillImprover:          "skill-improver",
		model.SkillRDDDefectWorkflow: "rdd-defect-workflow",
	}

	found := make(map[model.SkillID]string)
	for _, skill := range MVPSkills() {
		found[skill.ID] = skill.Name
		if skill.Name == "judgement-day" {
			t.Fatalf("catalog uses non-canonical spelling %q; want judgment-day", skill.Name)
		}
	}

	for id, wantName := range required {
		name, ok := found[id]
		if !ok {
			t.Fatalf("MVPSkills() missing requested bundled skill %q", id)
		}
		if name != wantName {
			t.Fatalf("MVPSkills() name for %q = %q, want %q", id, name, wantName)
		}
	}
}

// TestMVPSkillsShipEveryHGLayerStandard guards the set the team relies on to
// keep one standard across machines.
//
// A layer standard that exists as a file but is missing from this catalog never
// reaches anyone: the installer works from MVPSkills(), not from the assets
// directory, so an unregistered SKILL.md is embedded in the binary and then
// silently ignored.
func TestMVPSkillsShipEveryHGLayerStandard(t *testing.T) {
	required := map[model.SkillID]string{
		model.SkillBackendCRUD:   "backend-crud-standard",
		model.SkillFrontendCRUD:  "frontend-crud-standard",
		model.SkillDBChange:      "db-change-standard",
		model.SkillWorkerService: "worker-service-standard",
	}

	found := make(map[model.SkillID]string)
	for _, skill := range MVPSkills() {
		found[skill.ID] = skill.Name
	}

	for id, wantName := range required {
		name, ok := found[id]
		if !ok {
			t.Fatalf("MVPSkills() missing HG layer standard %q", id)
		}
		if name != wantName {
			t.Fatalf("MVPSkills() name for %q = %q, want %q", id, name, wantName)
		}
	}
}
