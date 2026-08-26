package cli

import (
	"testing"

	"bitbucket.org/hgt_development/hgtran-ai/v2/internal/model"
)

func TestNormalizePersonaAcceptsGentlemanNeutralArtifacts(t *testing.T) {
	got, err := normalizePersona("gentleman-neutral-artifacts")
	if err != nil {
		t.Fatalf("normalizePersona() error = %v", err)
	}
	if got != model.PersonaGentlemanNeutralArtifacts {
		t.Fatalf("normalizePersona() = %q, want %q", got, model.PersonaGentlemanNeutralArtifacts)
	}
}
