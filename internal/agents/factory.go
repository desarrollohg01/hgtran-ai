package agents

import (
	"fmt"

	"github.com/desarrollohg01/hgtran-ai/v2/internal/agents/antigravity"
	"github.com/desarrollohg01/hgtran-ai/v2/internal/agents/claude"
	"github.com/desarrollohg01/hgtran-ai/v2/internal/agents/codex"
	cursoradapter "github.com/desarrollohg01/hgtran-ai/v2/internal/agents/cursor"
	"github.com/desarrollohg01/hgtran-ai/v2/internal/agents/gemini"
	"github.com/desarrollohg01/hgtran-ai/v2/internal/agents/hermes"
	"github.com/desarrollohg01/hgtran-ai/v2/internal/agents/kilocode"
	"github.com/desarrollohg01/hgtran-ai/v2/internal/agents/kimi"
	"github.com/desarrollohg01/hgtran-ai/v2/internal/agents/kiro"
	"github.com/desarrollohg01/hgtran-ai/v2/internal/agents/openclaw"
	"github.com/desarrollohg01/hgtran-ai/v2/internal/agents/opencode"
	"github.com/desarrollohg01/hgtran-ai/v2/internal/agents/pi"
	"github.com/desarrollohg01/hgtran-ai/v2/internal/agents/qwen"
	"github.com/desarrollohg01/hgtran-ai/v2/internal/agents/trae"
	"github.com/desarrollohg01/hgtran-ai/v2/internal/agents/vscode"
	"github.com/desarrollohg01/hgtran-ai/v2/internal/agents/windsurf"
	"github.com/desarrollohg01/hgtran-ai/v2/internal/model"
)

var defaultAgentIDs = []model.AgentID{
	model.AgentClaudeCode,
	model.AgentOpenCode,
	model.AgentKilocode,
	model.AgentGeminiCLI,
	model.AgentCursor,
	model.AgentVSCodeCopilot,
	model.AgentCodex,
	model.AgentAntigravity,
	model.AgentWindsurf,
	model.AgentKimi,
	model.AgentQwenCode,
	model.AgentKiroIDE,
	model.AgentOpenClaw,
	model.AgentPi,
	model.AgentTrae,
	model.AgentHermes,
}

func NewAdapter(agent model.AgentID) (Adapter, error) {
	switch agent {
	case model.AgentClaudeCode:
		return claude.NewAdapter(), nil
	case model.AgentOpenCode:
		return opencode.NewAdapter(), nil
	case model.AgentKilocode:
		return kilocode.NewAdapter(), nil
	case model.AgentGeminiCLI:
		return gemini.NewAdapter(), nil
	case model.AgentCursor:
		return cursoradapter.NewAdapter(), nil
	case model.AgentVSCodeCopilot:
		return vscode.NewAdapter(), nil
	case model.AgentCodex:
		return codex.NewAdapter(), nil
	case model.AgentAntigravity:
		return antigravity.NewAdapter(), nil
	case model.AgentWindsurf:
		return windsurf.NewAdapter(), nil
	case model.AgentKimi:
		return kimi.NewAdapter(), nil
	case model.AgentQwenCode:
		return qwen.NewAdapter(), nil
	case model.AgentKiroIDE:
		return kiro.NewAdapter(), nil
	case model.AgentOpenClaw:
		return openclaw.NewAdapter(), nil
	case model.AgentPi:
		return pi.NewAdapter(), nil
	case model.AgentTrae:
		return trae.NewAdapter(), nil
	case model.AgentHermes:
		return hermes.NewAdapter(), nil
	default:
		return nil, AgentNotSupportedError{Agent: agent}
	}
}

func NewDefaultRegistry() (*Registry, error) {
	adapters := make([]Adapter, 0, len(defaultAgentIDs))

	for _, agent := range defaultAgentIDs {
		adapter, err := NewAdapter(agent)
		if err != nil {
			return nil, fmt.Errorf("create %s adapter: %w", agent, err)
		}
		adapters = append(adapters, adapter)
	}

	registry, err := NewRegistry(adapters...)
	if err != nil {
		return nil, fmt.Errorf("create registry: %w", err)
	}

	return registry, nil
}

// NewMVPRegistry creates a registry with only the MVP agents (Claude Code, OpenCode).
// Kept for backward compatibility.
func NewMVPRegistry() (*Registry, error) {
	claudeAdapter, err := NewAdapter(model.AgentClaudeCode)
	if err != nil {
		return nil, fmt.Errorf("create claude adapter: %w", err)
	}

	opencodeAdapter, err := NewAdapter(model.AgentOpenCode)
	if err != nil {
		return nil, fmt.Errorf("create opencode adapter: %w", err)
	}

	registry, err := NewRegistry(claudeAdapter, opencodeAdapter)
	if err != nil {
		return nil, fmt.Errorf("create registry: %w", err)
	}

	return registry, nil
}
