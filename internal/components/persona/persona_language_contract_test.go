package persona

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bitbucket.org/hgt_development/hgtran-ai/v2/internal/model"
)

func TestInjectGentlemanNeutralArtifactsRoutesToNeutralContent(t *testing.T) {
	home := t.TempDir()

	result, err := Inject(home, opencodeAdapter(), model.PersonaGentlemanNeutralArtifacts)
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("Inject() changed = false")
	}

	content, err := os.ReadFile(filepath.Join(home, ".config", "opencode", "AGENTS.md"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	text := string(content)

	// The alias routes to neutral content, not hgtran
	if strings.Contains(text, "Rioplatense") {
		t.Fatalf("alias should route to neutral — found hgtran tone marker 'Rioplatense'")
	}

	// Verify neutral content is present
	for _, want := range []string{
		"Generated technical artifacts default to English",
		"Public/contextual comments follow the target context language",
		"If the selected reply language is English, every part of the direct reply must be English",
		"Prompts starting with or dominated by hi, hello, hey, or similar English greetings are English prompts",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("installed persona missing %q; content:\n%s", want, text)
		}
	}
}
