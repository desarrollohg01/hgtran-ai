package opencode

import (
	"os"
	"path/filepath"
	"sort"
<<<<<<< HEAD
	"strings"
	"time"

	"bitbucket.org/hgt_development/hgtran-ai/v2/internal/statepath"
=======
>>>>>>> v2.5.0
)

// DefaultSettingsPath returns the default path to the OpenCode settings file.
func DefaultSettingsPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "opencode", "opencode.json")
}

// ModelCost holds the per-million-token pricing.
type ModelCost struct {
	Input  float64 `json:"input"`
	Output float64 `json:"output"`
}

// ModelLimit holds context and output token limits.
type ModelLimit struct {
	Context int `json:"context"`
	Output  int `json:"output"`
}

// Model represents a single model within a provider.
type Model struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Family    string     `json:"family"`
	ToolCall  bool       `json:"tool_call"`
	Reasoning bool       `json:"reasoning"`
	Cost      ModelCost  `json:"cost"`
	Limit     ModelLimit `json:"limit"`
	Variants  []string   `json:"-"`
}

// Provider represents a runtime model provider and its catalog.
type Provider struct {
	ID     string           `json:"id"`
	Name   string           `json:"name"`
	Models map[string]Model `json:"models"`
}

// FilterModelsForSDD returns models from a provider that support tool_call (required for SDD phases).
// Results are sorted by model name.
func FilterModelsForSDD(provider Provider) []Model {
	var models []Model
	for _, m := range provider.Models {
		if m.ToolCall {
			models = append(models, m)
		}
	}

	sort.Slice(models, func(i, j int) bool {
		return models[i].Name < models[j].Name
	})

	return models
}

// EffortLevels returns the available reasoning effort levels for this model.
// Returns nil if the model has no variants (effort picker should be skipped).
func (m Model) EffortLevels() []string {
	if len(m.Variants) == 0 {
		return nil
	}
	return m.Variants
}

<<<<<<< HEAD
// DefaultVariantsCachePath returns the path to the plugin-generated model variants file.
func DefaultVariantsCachePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return statepath.ModelVariantsCache(home)
}

// LoadVariants reads the plugin-generated model-variants.json file.
func LoadVariants(variantsPath string) (map[string]map[string][]string, error) {
	data, err := os.ReadFile(variantsPath)
	if err != nil {
		return nil, err
	}
	var variants map[string]map[string][]string
	if err := json.Unmarshal(data, &variants); err != nil {
		return nil, err
	}
	return variants, nil
}

// EnrichWithVariants merges variant data from the plugin cache file into
// cache-loaded providers. If the file is missing or invalid, models keep nil Variants.
func EnrichWithVariants(cached map[string]Provider, variantsPath string) {
	variants, err := LoadVariants(variantsPath)
	if err != nil {
		return
	}

	// Pass 1: Process exact provider matches first.
	for provID, models := range variants {
		cachedProv, ok := cached[provID]
		if !ok {
			continue
		}
		for modelID, levels := range models {
			if cachedModel, ok := cachedProv.Models[modelID]; ok {
				cachedModel.Variants = levels
				cachedProv.Models[modelID] = cachedModel
			}
		}
		cached[provID] = cachedProv
	}

	// Pass 2: Deterministic fallback for models that remain unassigned.
	// Sort keys to eliminate Go map iteration nondeterminism.
	variantKeys := make([]string, 0, len(variants))
	for provID := range variants {
		variantKeys = append(variantKeys, provID)
	}
	sort.Strings(variantKeys)

	cachedKeys := make([]string, 0, len(cached))
	for cachedID := range cached {
		cachedKeys = append(cachedKeys, cachedID)
	}
	sort.Strings(cachedKeys)

	for _, provID := range variantKeys {
		models := variants[provID]
		modelKeys := make([]string, 0, len(models))
		for modelID := range models {
			modelKeys = append(modelKeys, modelID)
		}
		sort.Strings(modelKeys)

		for _, modelID := range modelKeys {
			levels := models[modelID]
			for _, cachedID := range cachedKeys {
				p := cached[cachedID]
				if cachedModel, ok := p.Models[modelID]; ok && len(cachedModel.Variants) == 0 {
					cachedModel.Variants = levels
					p.Models[modelID] = cachedModel
					cached[cachedID] = p
				}
			}
		}
	}
}

// ConfigModel represents a model entry in the opencode.json provider section.
type ConfigModel struct {
	Name     string `json:"name"`
	ToolCall bool   `json:"tool_call"`
}

// ConfigProvider represents a custom provider defined in opencode.json.
type ConfigProvider struct {
	Name   string                 `json:"name"`
	URL    string                 `json:"url"`
	Models map[string]ConfigModel `json:"models"`
}

func FetchDynamicModels(ctx context.Context, baseURL string) ([]ConfigModel, error) {
	if baseURL == "" {
		return nil, errors.New("empty baseURL")
	}
	client := &http.Client{Timeout: 1 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/models", nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var res struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}

	models := make([]ConfigModel, 0, len(res.Data))
	for _, m := range res.Data {
		models = append(models, ConfigModel{Name: m.ID})
	}
	return models, nil
}

// LoadConfigProviders reads the provider section from an opencode.json settings file.
// Returns an empty map with nil error if the file is missing or has no provider key.
func LoadConfigProviders(path string) (map[string]ConfigProvider, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]ConfigProvider{}, nil
		}
		return map[string]ConfigProvider{}, err
	}

	var raw struct {
		Provider map[string]ConfigProvider `json:"provider"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return map[string]ConfigProvider{}, fmt.Errorf("parse opencode settings %q: %w", path, err)
	}
	if raw.Provider == nil {
		return map[string]ConfigProvider{}, nil
	}
	return raw.Provider, nil
}

// MergeCustomProviders merges custom providers from opencode.json into the cache-loaded
// providers map. Custom models use the tool_call value from opencode.json, defaulting to false
// when omitted. Custom entries win on ID collision (user-managed beats cached catalog).
// Returns the original providers map unchanged when config is empty; otherwise returns a
// merged copy without mutating the input.
func MergeCustomProviders(providers map[string]Provider, config map[string]ConfigProvider) map[string]Provider {
	if len(config) == 0 {
		return providers
	}

	merged := make(map[string]Provider, len(providers)+len(config))
	for id, p := range providers {
		clone := Provider{ID: p.ID, Name: p.Name, Env: append([]string(nil), p.Env...), Models: make(map[string]Model, len(p.Models))}
		for mid, m := range p.Models {
			clone.Models[mid] = m
		}
		merged[id] = clone
	}

	for id, cp := range config {
		// Provider-level collision: when a provider ID already exists in the cache,
		// we keep the cache's Name/Env and merge in the config's models below.
		// The config's provider Name is silently ignored in that case.
		existing, ok := merged[id]
		if !ok {
			existing = Provider{ID: id, Name: cp.Name, Models: make(map[string]Model, len(cp.Models))}
		}
		if existing.Models == nil {
			existing.Models = make(map[string]Model, len(cp.Models))
		}
		for mid, cm := range cp.Models {
			// Custom entry wins on model ID collision (user-managed beats cached catalog).
			name := cm.Name
			if name == "" {
				name = mid
			}
			existing.Models[mid] = Model{ID: mid, Name: name, ToolCall: cm.ToolCall}
		}
		merged[id] = existing
	}

	return merged
}

=======
>>>>>>> v2.5.0
// SDDPhases returns the ordered list of SDD phase sub-agent names.
func SDDPhases() []string {
	return []string{
		"sdd-init",
		"sdd-explore",
		"sdd-research",
		"sdd-propose",
		"sdd-spec",
		"sdd-design",
		"sdd-tasks",
		"sdd-apply",
		"sdd-verify",
		"sdd-archive",
		"sdd-onboard",
	}
}

// JDPhases returns the ordered list of judgment-day sub-agent names.
// These are workflow-level agents (not SDD phases) used by the
// judgment-day skill for parallel adversarial review.
// They support independent model configuration for diversity of perspective.
func JDPhases() []string {
	return []string{
		"jd-judge-a",
		"jd-judge-b",
		"jd-fix-agent",
	}
}

const (
	ReviewRefuterAgent   = "review-refuter"
	ReviewValidatorAgent = "review-validator"
)

// ReviewLensPhases returns the ordered native bounded-review lens agents.
func ReviewLensPhases() []string {
	return []string{
		"review-risk",
		"review-readability",
		"review-reliability",
		"review-resilience",
	}
}

// ReviewPhases returns every agent invoked by the native review lifecycle.
func ReviewPhases() []string {
	phases := ReviewLensPhases()
	return append(phases, ReviewRefuterAgent, ReviewValidatorAgent)
}

// ConfigurableAgentPhases returns all agent names that support per-agent
// model configuration. This includes SDD, Judgment Day, and review agents.
// Used by the inject model assignment table builder and the configurable agent set
// in ReadCurrentModelAssignments. The TUI uses each role family separately
// for row layout control.
func ConfigurableAgentPhases() []string {
	phases := SDDPhases()
	phases = append(phases, JDPhases()...)
	phases = append(phases, ReviewPhases()...)
	return phases
}
