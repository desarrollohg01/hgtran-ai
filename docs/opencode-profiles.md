# OpenCode SDD Profiles

← [Back to README](../README.md)

---

You configured your SDD models once, and now every task -- cheap or expensive, experimental or battle-tested -- runs through the same orchestrator. Profiles fix that: **create named model configurations and switch between them with Tab inside OpenCode.**

Hgtran AI supports **two ways** of working with OpenCode profiles. Profiles cover SDD phase agents; Judgment Day agents (`jd-judge-a`, `jd-judge-b`, `jd-fix-agent`) are workflow-level slots with independent model assignments.

1. **Generated multi-profile mode** -- the classic Hgtran AI flow. The base SDD conductor is `hgtran-orchestrator`. Each named profile generates its own `sdd-orchestrator-{name}` plus 10 suffixed SDD phase sub-agents in `opencode.json`, and you switch between them with **Tab**.
2. **External single-active mode** -- for community tools that keep profile files outside `opencode.json` and activate one runtime profile at a time.

That means you can stay with the built-in multi-profile overlay, or plug Hgtran AI into an external profile manager without the two systems fighting each other.

---


## Native background subagents

OpenCode SDD uses native OpenCode subagents through the `task` permission. Hgtran AI no longer installs the legacy `background-agents.ts` plugin by default.

To opt into OpenCode's experimental background subagent execution, start OpenCode with:

```sh
export OPENCODE_EXPERIMENTAL_BACKGROUND_SUBAGENTS=true
opencode
```

Hgtran AI does not currently write process environment variables into `opencode.json`; keep the flag in your shell, terminal profile, or launcher until OpenCode provides a stable config-level switch.

## Quick Start (TUI)

1. Launch the installer: `hgtran-ai` (or `go run ./cmd/hgtran-ai`).
2. Select **"OpenCode SDD Profiles"** from the welcome screen.
3. Select **"Create new profile"** (or press `n`).
4. Enter a profile name in slug format (lowercase, hyphens ok). Example: `cheap`.
5. Pick the orchestrator model (provider, then model -- reuses the existing model picker).
6. Assign sub-agent models (use "Set all phases" for a uniform config, or set each phase individually).
7. Confirm -- the installer writes the profile to `opencode.json` and runs sync.

Open OpenCode and press **Tab** -- your new orchestrator appears alongside `hgtran-orchestrator`, the default OpenCode SDD conductor.

### Reasoning effort levels (per-model variants)

For models that expose reasoning effort variants (e.g. OpenAI `gpt-5` with `low`/`medium`/`high`/`xhigh`), the picker shows an extra **Select reasoning effort level** step right after you choose the model. Pick `default` to use the provider's default, or pick a specific level to lock the assignment to that effort.

The effort options are populated from a cache file written by the bundled `model-variants` OpenCode plugin at `~/.hgtran-ai/cache/model-variants.json`. The plugin runs the first time OpenCode starts after `hgtran-ai sync` and refreshes the cache on every subsequent start.

**First-run order matters:**

1. Run `hgtran-ai` (installs the plugin into `~/.config/opencode/plugins/`).
2. Run `opencode` once -- on startup the plugin queries the provider list and writes `~/.hgtran-ai/cache/model-variants.json`.
3. Re-run `hgtran-ai` and open the model picker. Reasoning models now show the effort selector.

If the JSON does not exist yet (plugin has not run, no providers expose variants, or the request failed silently), reasoning models still work -- the picker simply skips the effort step and saves the assignment with the provider default. You will not see the `[effort]` annotation next to those rows in the phase list.

## Key Names To Remember

Use this table when reviewing configs or debugging profile sync:

| Agent key | Meaning | Safe to rename manually? |
|---|---|---|
| `hgtran-orchestrator` | Canonical base OpenCode SDD conductor. All `/sdd-*` commands point here by default. | No |
| `sdd-orchestrator` | Legacy base conductor key. Sync migrates it to `hgtran-orchestrator`. | No; let sync migrate it |
| `sdd-orchestrator-{name}` | Generated named profile conductor, such as `sdd-orchestrator-cheap`. | No; use TUI or CLI |
| `sdd-{phase}` | Default sub-agent for a phase, such as `sdd-apply`. | No |
| `sdd-{phase}-{name}` | Named profile sub-agent, such as `sdd-apply-cheap`. | No |

## Quick Start (CLI)

Create a profile during sync with `--profile name:provider/model`:

```bash
hgtran-ai sync --profile cheap:anthropic/claude-haiku-3.5-20241022
```

Multiple profiles in one command:

```bash
hgtran-ai sync \
  --profile cheap:anthropic/claude-haiku-3.5-20241022 \
  --profile premium:anthropic/claude-opus-4-20250514
```

Override a specific phase with `--profile-phase name:phase:provider/model`:

```bash
hgtran-ai sync \
  --profile cheap:anthropic/claude-haiku-3.5-20241022 \
  --profile-phase cheap:sdd-apply:anthropic/claude-sonnet-4-20250514
```

This creates a "cheap" profile where everything runs on Haiku except `sdd-apply`, which uses Sonnet.

## External Profile Managers

If you're using a community tool that stores profiles under `~/.config/opencode/profiles/*.json` and activates them at runtime, Hgtran AI can now sync OpenCode in a compatibility mode.

### Auto-detection

On `hgtran-ai sync`, if OpenCode profile files exist under:

```text
~/.config/opencode/profiles/*.json
```

Hgtran AI automatically switches to **`external-single-active`** strategy for OpenCode sync.

### Manual override

You can also force the strategy explicitly:

```bash
hgtran-ai sync --agent opencode --sdd-profile-strategy external-single-active
```

Or force the classic generated overlay behavior:

```bash
hgtran-ai sync --agent opencode --sdd-profile-strategy generated-multi
```

### What compatibility mode does

In `external-single-active` mode, Hgtran AI:

- keeps writing the base OpenCode SDD assets and shared prompt files
- **does not** auto-regenerate suffixed named profiles from `opencode.json`
- **preserves the current `hgtran-orchestrator` prompt** during sync so external tools can keep their runtime policy / fallback blocks intact

This is the important bit: Hgtran AI still maintains the SDD foundation, but it stops acting like `opencode.json` is the source of truth for every profile.

## Using Profiles in OpenCode

After creating profiles in generated multi-profile mode, each one appears as a selectable orchestrator in OpenCode:

| What you see in Tab | What it runs |
|---|---|
| `hgtran-orchestrator` | Default profile (your original config) |
| `sdd-orchestrator-cheap` | "cheap" profile -- Haiku everywhere |
| `sdd-orchestrator-premium` | "premium" profile -- Opus everywhere |

Press **Tab** to cycle between orchestrators. All SDD slash commands (`/sdd-new`, `/sdd-ff`, `/sdd-explore`, etc.) run against whichever orchestrator is currently selected. The orchestrator delegates to its own suffixed sub-agents (e.g., `sdd-apply-cheap`), so profiles never interfere with each other.

If you're using an external single-active manager instead, you typically keep working with the base `hgtran-orchestrator` while the external tool swaps its active model assignments at runtime.

## Managing Profiles

From the TUI profile list screen:

| Action | Key | Notes |
|---|---|---|
| Edit a profile | `Enter` on the profile | Change models, then sync |
| Delete a profile | `d` on the profile | Removes orchestrator + all sub-agents from JSON |
| Create a new profile | `n` (or select "Create new profile") | Full creation flow |

The `default` profile (`hgtran-orchestrator`) can be edited but not deleted -- it always exists when SDD is configured.

### Profile name rules

| Input | Valid? | Reason |
|---|---|---|
| `cheap` | Yes | Simple slug |
| `premium-v2` | Yes | Hyphens allowed |
| `my profile` | No | Spaces not allowed |
| `default` | No | Reserved for the base orchestrator |
| `LOUD` | Becomes `loud` | Auto-lowercased |

---

<details>
<summary><strong>How It Works</strong></summary>

In generated multi-profile mode, each named profile generates 11 agent entries in `opencode.json`: one orchestrator (`sdd-orchestrator-{name}`, mode `primary`) and 10 SDD phase sub-agents (`sdd-{phase}-{name}`, mode `subagent`, hidden). The base/default conductor remains `hgtran-orchestrator`. Each named profile orchestrator's permissions are scoped so it can only delegate to its own suffixed sub-agents.

Sub-agent prompts are shared across all profiles as files under `~/.config/opencode/prompts/sdd/` (e.g., `sdd-apply.md`). Each agent entry references the shared file via `{file:~/.config/opencode/prompts/sdd/sdd-apply.md}` -- only the `model` field differs between profiles. Orchestrator prompts are inlined per-profile because they contain profile-specific model assignment tables and sub-agent references.

During sync or update, Hgtran AI now uses one of two strategies:

- **`generated-multi`** -- scan `opencode.json` for `sdd-orchestrator-*`, update shared prompts, regenerate profile orchestrators, preserve model assignments, and keep `hgtran-orchestrator` as the canonical base conductor
- **`external-single-active`** -- detect external profile files, keep the shared SDD assets current, and preserve the existing `hgtran-orchestrator` prompt instead of overwriting external runtime extensions

</details>

---

← [Back to README](../README.md)
