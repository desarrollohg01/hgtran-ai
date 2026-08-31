# Proposal: Trae IDE Agent Support

## Intent

Trae IDE (trae.ai, by ByteDance) is a desktop AI-powered code editor with MCP support, custom agent instructions, and a skill system — making it compatible with hgtran-ai's ecosystem management capabilities. Currently hgtran-ai has no Trae adapter, so users who run both tools must manage their configs manually. This change adds Trae as a first-class agent with full ecosystem support: system prompt injection, MCP config, and skill installation.

## Scope

### In Scope
- `internal/agents/trae/adapter.go` — new Adapter implementing the full `agents.Adapter` interface
- `internal/agents/trae/adapter_test.go` — table-driven tests covering detection, config paths, and strategies
- `internal/model/types.go` — add `AgentTrae AgentID = "trae"`
- `internal/agents/factory.go` — register `trae.NewAdapter()` in `NewAdapter()` switch and `defaultAgentIDs`
- `internal/catalog/agents.go` — add Trae entry to `allAgents`
- `internal/system/config_scan.go` — add `~/.trae` to `knownAgentConfigDirs`

### Out of Scope
- Trae-specific workflow files (no `.trae/workflows/` equivalent documented)
- Sub-agent support (not documented for Trae)
- Slash commands support (not documented for Trae)
- Auto-install (Trae is a desktop app with no CLI installer)

## Config Layout

Trae stores its config at `~/.trae/` (cross-platform, no OS-specific split):

```
~/.trae/
├── mcp.json            # MCP server configs
├── user_rules/         # User-level rules
│   └── hgtran-ai.md   # System prompt written by persona.Inject()
├── skills/             # Skill files
└── skill-config.json  # Disabled skills registry (read-only for hgtran-ai)
```

## Approach

1. **Model**: Add `AgentTrae AgentID = "trae"` to `internal/model/types.go`.
2. **Adapter**: Implement `internal/agents/trae/adapter.go` modeled after the Windsurf adapter (desktop app, no binary on PATH). Detection: `~/.trae` directory presence.
   - `SystemPromptStrategy` → `StrategyMarkdownSections` (hgtran-ai markers in `user_rules/hgtran-ai.md`)
   - `MCPStrategy` → `StrategyMCPConfigFile` (writes `~/.trae/mcp.json`)
   - `SkillsDir` → `~/.trae/skills/`
   - `SupportsSkills`, `SupportsSystemPrompt`, `SupportsMCP` → `true`
   - `SupportsAutoInstall` → `false`
3. **Registration**: Wire into `factory.go`, `catalog/agents.go`, `system/config_scan.go`.
4. **Tests**: Table-driven tests for Detect (found/missing/stat-error), all config path methods, strategy accessors, and the not-installable error.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/model/types.go` | Modified | Add `AgentTrae` constant |
| `internal/agents/trae/` | New | Full adapter package |
| `internal/agents/factory.go` | Modified | Register Trae in switch + defaultAgentIDs |
| `internal/catalog/agents.go` | Modified | Add to allAgents |
| `internal/system/config_scan.go` | Modified | Add `~/.trae` to knownAgentConfigDirs |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Trae config path changes in future versions | Low | Path is `~/.trae` — simple and stable; update adapter if docs change |
| `user_rules/` directory may not exist on fresh install | Low | hgtran-ai's `persona.Inject()` creates parent dirs before writing |
| StrategyMarkdownSections may conflict with Trae's native rule parsing | Low | Markers are HTML comments — transparent to Trae's parser |

## Rollback Plan

1. Revert the branch — all changes are additive (new package, new constants, new catalog/factory entries)
2. No user-facing config is mutated until `gga sync` is run with Trae detected

## Success Criteria

- [ ] `go test ./...` passes including new `trae` package tests
- [ ] `go vet ./...` reports no issues
- [ ] `gga` detects Trae when `~/.trae` exists
- [ ] System prompt written to `~/.trae/user_rules/hgtran-ai.md`
- [ ] MCP config written to `~/.trae/mcp.json`
- [ ] Skills written to `~/.trae/skills/`
- [ ] Trae appears in `gga list` output
