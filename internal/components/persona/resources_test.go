package persona_test

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/desarrollohg01/hgtran-ai/v2/internal/components/persona"
	"github.com/desarrollohg01/hgtran-ai/v2/internal/model"
)

func TestResourcePlanOutputStylePaths(t *testing.T) {
	dir := t.TempDir()
	hgtran := filepath.Join(dir, "hgtran.md")
	neutral := filepath.Join(dir, "neutral.md")

	tests := []struct {
		name    string
		persona model.PersonaID
		want    persona.OutputStylePaths
	}{
		{
			name:    "hgtran writes its selected style without removing neutral",
			persona: model.PersonaGentleman,
			want: persona.OutputStylePaths{
				Write:  hgtran,
				Backup: []string{hgtran, neutral},
			},
		},
		{
			name:    "neutral writes its selected style and removes retired hgtran",
			persona: model.PersonaNeutral,
			want: persona.OutputStylePaths{
				Write:  neutral,
				Backup: []string{hgtran, neutral},
				Remove: []string{hgtran},
			},
		},
		{
			name:    "legacy neutral alias writes neutral and removes retired hgtran",
			persona: model.PersonaGentlemanNeutralArtifacts,
			want: persona.OutputStylePaths{
				Write:  neutral,
				Backup: []string{hgtran, neutral},
				Remove: []string{hgtran},
			},
		},
		{
			name:    "custom manages no output styles",
			persona: model.PersonaCustom,
			want:    persona.OutputStylePaths{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := persona.ResourcePlanFor(tt.persona).OutputStylePaths(dir)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ResourcePlanFor(%q).OutputStylePaths() = %#v, want %#v", tt.persona, got, tt.want)
			}
		})
	}
}
