package capabilitymanifest

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/desarrollohg01/hgtran-ai/v2/internal/catalog"
	"github.com/desarrollohg01/hgtran-ai/v2/internal/model"
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
	// Digests pin the four providers with an enforceable fresh-reviewer
	// boundary: Claude Code's generated reviewer has no live tools, OpenCode
	// relays one ordinary task through Go-owned admission, Codex's provider
	// subprocess reaches the same contract, and hgtran-pi's host relay
	// forwards the Go-issued opaque task to a fresh locked-down pi
	// subprocess (hgtran-pi#311, hgtran-ai#3249).
	wantManifestDigests := map[model.AgentID]string{
		model.AgentAntigravity:   "sha256:c8ec4383509e0dd1a404d16d17e13ad5a21f68d535d83a1b0c2d052b5aa2b324",
		model.AgentClaudeCode:    "sha256:f6661f4132c975f711410aceddbc4f8058b676b039b2ca22f7eaaa02f0e31e6d",
		model.AgentCodex:         "sha256:6d0e669565596d9628a4676092806870dc549fc86f2343ba6b9ca6d9beda9295",
		model.AgentCursor:        "sha256:60946b8dc9020d4ed6f0a4e2d02b36c065172998debb2a0a4dba935269321ef7",
		model.AgentGeminiCLI:     "sha256:ca9ec313a6af46523d455e2fdac144de2041382d1ad03230e2213cb956153c2e",
		model.AgentHermes:        "sha256:755477220bcfc4eb82a58860a2da70e97d13c0ecea45c5d271dcbcfe1856f780",
		model.AgentKilocode:      "sha256:7333f2398c01c40dc59a3de49fd141ad289fa1b179ec36bfefa43e0de3ad2d91",
		model.AgentKimi:          "sha256:883fe92b92e39037d0ae2e504fc143767a6fadf1d80c2c7a02fa07d57c93639f",
		model.AgentKiroIDE:       "sha256:7a992ebd667e0f8b7b7d4e3908d587db3dc596f0658446714feec96046d3b51a",
		model.AgentOpenClaw:      "sha256:0cda5f136f4db1da2625e0bf032472d8565491458f4b353ea03d1f3f0a1f5af8",
		model.AgentOpenCode:      "sha256:ad6d7c62e5b22ba9a34a3ff52e4b1f666a6e2582d900258cb496bf0fce5cfe49",
		model.AgentPi:            "sha256:fa04e88324ed3fcc426acef3ce378399f1f379900f09933d975d670f9348788f",
		model.AgentQwenCode:      "sha256:ca8191bd5697cfabff8fc4199d561f494c1b2ab878fd29598a65ae9e49418180",
		model.AgentTrae:          "sha256:94b9932efe9280c2c5992d0647706f794dd1670abdb53e2898c2336128b8eec0",
		model.AgentVSCodeCopilot: "sha256:2371f77df6066c1d9b9670e1c8e2ad33301b3a61d67684cea1b94b895ef24513",
		model.AgentWindsurf:      "sha256:aed72bde9c6890e5f7b1d43322c52cd89d27d8f8d84402ec8fbf8582143d757b",
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
			wantImmutableExecutor := agent == model.AgentClaudeCode || agent == model.AgentOpenCode || agent == model.AgentCodex || agent == model.AgentPi
			if got := manifest.Advertises(ContractImmutableReviewExecutorV1); got != wantImmutableExecutor {
				t.Fatalf("immutable reviewer execution advertised = %t, want %t", got, wantImmutableExecutor)
			}
			wantExposure := ContractExposureDormant
			if wantImmutableExecutor {
				wantExposure = ContractExposureAdvertised
			}
			if got := manifest.Contracts.ImmutableReviewExecutorV1.Exposure; got != wantExposure {
				t.Fatalf("immutable reviewer execution exposure = %q, want %q", got, wantExposure)
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

// TestEveryManifestDigestStaysByteStable pins every non-Pi row at the closed
// review-transport baseline. The 12 non-RDD rows change only because their
// transport claim becomes dormant; the three non-Pi RDD rows remain unchanged.
func TestEveryManifestDigestStaysByteStable(t *testing.T) {
	t.Parallel()

	wantNonPiDigests := map[model.AgentID]string{
		model.AgentAntigravity:   "sha256:c8ec4383509e0dd1a404d16d17e13ad5a21f68d535d83a1b0c2d052b5aa2b324",
		model.AgentClaudeCode:    "sha256:f6661f4132c975f711410aceddbc4f8058b676b039b2ca22f7eaaa02f0e31e6d",
		model.AgentCodex:         "sha256:6d0e669565596d9628a4676092806870dc549fc86f2343ba6b9ca6d9beda9295",
		model.AgentCursor:        "sha256:60946b8dc9020d4ed6f0a4e2d02b36c065172998debb2a0a4dba935269321ef7",
		model.AgentGeminiCLI:     "sha256:ca9ec313a6af46523d455e2fdac144de2041382d1ad03230e2213cb956153c2e",
		model.AgentHermes:        "sha256:755477220bcfc4eb82a58860a2da70e97d13c0ecea45c5d271dcbcfe1856f780",
		model.AgentKilocode:      "sha256:7333f2398c01c40dc59a3de49fd141ad289fa1b179ec36bfefa43e0de3ad2d91",
		model.AgentKimi:          "sha256:883fe92b92e39037d0ae2e504fc143767a6fadf1d80c2c7a02fa07d57c93639f",
		model.AgentKiroIDE:       "sha256:7a992ebd667e0f8b7b7d4e3908d587db3dc596f0658446714feec96046d3b51a",
		model.AgentOpenClaw:      "sha256:0cda5f136f4db1da2625e0bf032472d8565491458f4b353ea03d1f3f0a1f5af8",
		model.AgentOpenCode:      "sha256:ad6d7c62e5b22ba9a34a3ff52e4b1f666a6e2582d900258cb496bf0fce5cfe49",
		model.AgentQwenCode:      "sha256:ca8191bd5697cfabff8fc4199d561f494c1b2ab878fd29598a65ae9e49418180",
		model.AgentTrae:          "sha256:94b9932efe9280c2c5992d0647706f794dd1670abdb53e2898c2336128b8eec0",
		model.AgentVSCodeCopilot: "sha256:2371f77df6066c1d9b9670e1c8e2ad33301b3a61d67684cea1b94b895ef24513",
		model.AgentWindsurf:      "sha256:aed72bde9c6890e5f7b1d43322c52cd89d27d8f8d84402ec8fbf8582143d757b",
	}

	nonPiAgents := make([]model.AgentID, 0, len(wantNonPiDigests))
	for agent := range wantNonPiDigests {
		nonPiAgents = append(nonPiAgents, agent)
	}

	if got := len(nonPiAgents); got != 15 {
		t.Fatalf("want 15 non-Pi agents, got %d", got)
	}

	for _, agent := range nonPiAgents {
		agent := agent
		wantDigest := wantNonPiDigests[agent]
		t.Run(string(agent), func(t *testing.T) {
			t.Parallel()

			manifest := MustForAgent(agent)
			gotDigest, err := manifest.Digest()
			if err != nil {
				t.Fatalf("Digest() error = %v", err)
			}
			if gotDigest != wantDigest {
				t.Fatalf("Digest() = %q, want %q (byte-stable contract)", gotDigest, wantDigest)
			}
		})
	}
}

func TestReviewTransportAdvertisementIsClosedCatalogSet(t *testing.T) {
	const wantExposed = 4

	exposed := 0
	for _, agent := range catalog.AllAgents() {
		t.Run(string(agent.ID), func(t *testing.T) {
			manifest := MustForAgent(agent.ID)
			want := agent.ID == model.AgentClaudeCode ||
				agent.ID == model.AgentOpenCode ||
				agent.ID == model.AgentCodex ||
				agent.ID == model.AgentPi
			if got := manifest.Advertises(ContractReviewTransportV1); got != want {
				t.Fatalf("review transport advertised = %t, want %t", got, want)
			}
			if want {
				exposed++
			}
		})
	}
	if exposed != wantExposed {
		t.Fatalf("advertised review transport runtimes = %d, want %d", exposed, wantExposed)
	}
}

func TestForAgentRejectsUnknownAgent(t *testing.T) {
	t.Parallel()

	_, err := ForAgent(model.AgentID("unknown"))
	if !errors.Is(err, ErrUnsupportedAgent) {
		t.Fatalf("ForAgent() error = %v, want ErrUnsupportedAgent", err)
	}
}
