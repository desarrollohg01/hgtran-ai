package capabilitymanifest

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"bitbucket.org/hgt_development/hgtran-ai/v2/internal/model"
)

func TestCanonicalImplementationRoutingBoundaries(t *testing.T) {
	t.Parallel()

	got := CanonicalImplementationRouting()
	want := ImplementationRoutingFacts{
		DirectInline: DirectInlineFacts{
			MinUnderstandingFiles:                    1,
			MaxUnderstandingFiles:                    3,
			MaxMechanicalWriteFiles:                  1,
			MechanicalWriteMustBeAlreadyUnderstood:   true,
			MechanicalWriteMustNotRequireResearch:    true,
			MechanicalWriteMustNotHaveOpenDesignWork: true,
		},
		DelegatedDirect: DelegatedDirectFacts{
			MappingMinUnderstandingFiles:  4,
			WriterMinNonTrivialFiles:      2,
			DelegateWhenReadPreparesWrite: true,
			DelegateWhenBroadResearch:     true,
		},
		SDD: SDDProposalFacts{
			ProposeWhenSubstantialOrAmbiguous:     true,
			DurableArtifactsMustReduceUncertainty: true,
			SelectionPolicy:                       SDDSelectionExplicitRequestOrAcceptedProposal,
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CanonicalImplementationRouting() = %#v, want %#v", got, want)
	}
}

func TestManifestRejectsWeakenedRoutingFacts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		weaken func(*AgentCapabilityManifest)
	}{
		{
			name: "direct understanding starts below one file",
			weaken: func(manifest *AgentCapabilityManifest) {
				manifest.ImplementationRouting.DirectInline.MinUnderstandingFiles = 0
			},
		},
		{
			name: "direct understanding exceeds three files",
			weaken: func(manifest *AgentCapabilityManifest) {
				manifest.ImplementationRouting.DirectInline.MaxUnderstandingFiles = 4
			},
		},
		{
			name: "mapping starts after four files",
			weaken: func(manifest *AgentCapabilityManifest) {
				manifest.ImplementationRouting.DelegatedDirect.MappingMinUnderstandingFiles = 5
			},
		},
		{
			name: "writer starts after two non-trivial files",
			weaken: func(manifest *AgentCapabilityManifest) {
				manifest.ImplementationRouting.DelegatedDirect.WriterMinNonTrivialFiles = 3
			},
		},
		{
			name: "read preparing write no longer delegates",
			weaken: func(manifest *AgentCapabilityManifest) {
				manifest.ImplementationRouting.DelegatedDirect.DelegateWhenReadPreparesWrite = false
			},
		},
		{
			name: "broad research no longer delegates",
			weaken: func(manifest *AgentCapabilityManifest) {
				manifest.ImplementationRouting.DelegatedDirect.DelegateWhenBroadResearch = false
			},
		},
		{
			name: "substantial ambiguity no longer proposes SDD",
			weaken: func(manifest *AgentCapabilityManifest) {
				manifest.ImplementationRouting.SDD.ProposeWhenSubstantialOrAmbiguous = false
			},
		},
		{
			name: "SDD proposal need not reduce durable uncertainty",
			weaken: func(manifest *AgentCapabilityManifest) {
				manifest.ImplementationRouting.SDD.DurableArtifactsMustReduceUncertainty = false
			},
		},
		{
			name: "SDD selection bypasses explicit consent",
			weaken: func(manifest *AgentCapabilityManifest) {
				manifest.ImplementationRouting.SDD.SelectionPolicy = "automatic"
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			manifest := MustForAgent(model.AgentClaudeCode)
			test.weaken(&manifest)
			if err := manifest.Validate(); err == nil {
				t.Fatal("Validate() = nil, want non-canonical routing rejection")
			}
		})
	}
}

func TestEveryManifestKeepsWorkRoutingDormantAndHashesCanonically(t *testing.T) {
	t.Parallel()

	const wantRoutingDigest = "sha256:8e1a59ce22ec310983924b512056ee21a0674aca684620ab2a336cee3b0e30c6"
	// Digests intentionally changed by the identity rename: the schema and
	// contract identifiers that feed the digest's domain separation moved
	// from the gentle-ai namespace to hgtran-ai, which the
	// canonical JSON payload — and therefore the digest — legitimately
	// covers for every agent.
	wantManifestDigests := map[model.AgentID]string{
		model.AgentAntigravity:   "sha256:072df9932753131b8d46449d5edc2ceda9b393a4c916e355d6ed4699e0f755a7",
		model.AgentClaudeCode:    "sha256:a129849f2db6b46e4446f3ad66e78bc8b2ae36b83ff45627ea6bdfb119c4ff7d",
		model.AgentCodex:         "sha256:e4e8c0a960f9ff6bbc6930ea6bad0ba692ac22cf61813a28f12ff40b9c99592b",
		model.AgentCursor:        "sha256:fcc90d1fe91c767d0c576d273e8a1bc0778e19036293d5f34ba66a12a85f3752",
		model.AgentGeminiCLI:     "sha256:1cda1ca0332d261e54c72fe94704c2973a714ec2533b129b6b76ad6e424a9675",
		model.AgentHermes:        "sha256:f91962f610c72f92731b6fd36ecb39cff77c9518e8c8ac5623f45996c806f507",
		model.AgentKilocode:      "sha256:05811636338cdd54c21a0ac866f8563c045aff88d4617940ea1a1e2da36cdc00",
		model.AgentKimi:          "sha256:92dc369f3d95801925a536be3c3592ffecc4a565e84d066bd53e4740a425e446",
		model.AgentKiroIDE:       "sha256:621b5efe6ca847e4aa3f7bfba92b3e176e1e1f2b7cc5d24d997919b5b02ecf6f",
		model.AgentOpenClaw:      "sha256:22ba7956f60ef5bde9aba267b5a070372d9f0b81bf915ea0ac1f4a2982367f04",
		model.AgentOpenCode:      "sha256:0195d69978f95a1dc08e739b405385d855dd1c9bb69c1257178f404309cb49d5",
		model.AgentPi:            "sha256:2b3e41ede38c7ec65d98547f9a7cc61bac1e8d1a9e2191d079dafbfa20274384",
		model.AgentQwenCode:      "sha256:89edbdb35559aec995a31b41a0dcfde7e3c659715d85df8d2daa68dc0adfd6e3",
		model.AgentTrae:          "sha256:57452308ab26097a10aec624602d955d46bf2ed05cde74930d4be5faad122395",
		model.AgentVSCodeCopilot: "sha256:ca927574219a1609a50a5442c02ce681acfa6fe79aaeb9791b6989aa3b8c7598",
		model.AgentWindsurf:      "sha256:1911cd100ed5af461a64b067adf9f0ebe962e317320fd3e908e932e5ae22b1dc",
	}

	for agent, wantDigest := range wantManifestDigests {
		agent := agent
		wantDigest := wantDigest
		t.Run(string(agent), func(t *testing.T) {
			t.Parallel()

			manifest := MustForAgent(agent)
			if err := manifest.Validate(); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			if manifest.Contracts.WorkRoutingV1.Exposure != ContractExposureDormant {
				t.Fatalf("work-routing exposure = %q, want %q", manifest.Contracts.WorkRoutingV1.Exposure, ContractExposureDormant)
			}
			if manifest.Advertises(ContractWorkRoutingV1) {
				t.Fatal("work-routing must remain unadvertised before final activation")
			}

			payload, err := manifest.CanonicalJSON()
			if err != nil {
				t.Fatalf("CanonicalJSON() error = %v", err)
			}
			var roundTrip AgentCapabilityManifest
			if err := json.Unmarshal(payload, &roundTrip); err != nil {
				t.Fatalf("Unmarshal(CanonicalJSON()) error = %v", err)
			}
			if roundTrip != manifest {
				t.Fatalf("canonical JSON round trip = %#v, want %#v", roundTrip, manifest)
			}

			gotDigest, err := roundTrip.Digest()
			if err != nil {
				t.Fatalf("Digest() error = %v", err)
			}
			if gotDigest != wantDigest {
				t.Fatalf("Digest() = %q, want %q", gotDigest, wantDigest)
			}

			gotRoutingDigest, err := manifest.RoutingDigest()
			if err != nil {
				t.Fatalf("RoutingDigest() error = %v", err)
			}
			if gotRoutingDigest != wantRoutingDigest {
				t.Fatalf("RoutingDigest() = %q, want %q", gotRoutingDigest, wantRoutingDigest)
			}
		})
	}
}

func TestForAgentRejectsUnknownAgent(t *testing.T) {
	t.Parallel()

	_, err := ForAgent(model.AgentID("unknown"))
	if !errors.Is(err, ErrUnsupportedAgent) {
		t.Fatalf("ForAgent() error = %v, want ErrUnsupportedAgent", err)
	}
}
