# ADR: Skill Authoring Standard Compliance Across engram, hgtran-ai, hgtran-guardian-angel

- **Status:** Proposed
- **Date:** 2026-09-04
- **Owners:** Ivan Villarreal
- **Scope:** All `SKILL.md` files in `engram`, `hgtran-ai`, and `hgtran-guardian-angel` (workspace root: `HgTransportaciones-workspace/ai`)
- **Normative reference:** `hgtran-ai/docs/skill-style-guide.md`

## Context

`hgtran-ai/docs/skill-style-guide.md` defines the single LLM-first authoring standard for `SKILL.md` files: a fixed 7-section structure (Frontmatter → Activation Contract → Hard Rules → Decision Gates → Execution Steps → Output Contract → References), strict frontmatter rules (single-line quoted `description` with `Trigger:` first, complete `name`/`license`/`metadata.author`/`metadata.version`), and a body budget (180–450 target tokens, 700 recommended max, 1000 hard max).

This guide is authored and owned by `hgtran-ai`, but skills adapted for HG (`engram`, `hgtran-guardian-angel`) were written independently, largely predating or ignoring this guide. Three parallel audits (one per repo, using `skill-improver`) compared every `SKILL.md` against the guide to establish an actual compliance baseline before further skill work continues.

## Audit Method

Each repo was audited by a fresh agent that read the style guide, then read every `SKILL.md` under that repo's skill directories, checking: required section presence/order, frontmatter completeness and formatting, body length vs. token budget, DON'T-rule violations (tutorial prose, duplicated docs, non-actionable advice, external URLs as primary references), and References-section hygiene. Audit-only — no files were modified.

## Findings

### Aggregate

| Repo | Skills audited | Fully compliant | Partial | Non-compliant |
|---|---|---|---|---|
| `hgtran-ai` (owns the guide) | 30 | 8 | 2 | 20 |
| `engram` | 21 | 0 | 17 | 4 |
| `hgtran-guardian-angel` | 6 | 0 | 0 | 6 |
| **Total** | **57** | **8 (14%)** | **19 (33%)** | **30 (53%)** |

Only 8 of 57 skills across the workspace are fully compliant with the standard the workspace itself defines: `chained-pr`, `go-testing`, `hermes-ephemeral-delegation`, `judgment-day`, `sdd-init`, `skill-creator`, `skill-registry`, `skill-improver` (all in `hgtran-ai`).

### hgtran-guardian-angel — worst offender (0/6 compliant)

All six skills (`branch-pr`, `commit-hygiene`, `docs-alignment`, `issue-creation`, `shellcheck-standards`, `testing-coverage`) have **zero YAML frontmatter** — no `name`, `description`, `license`, or `metadata`. They are structurally undiscoverable by any frontmatter-based skill registry. None uses the required section names (they use ad hoc headers like `Purpose`, `When to Use`, `Cookbook`). None has an `Output Contract`. `testing-coverage` additionally cites `https://shellspec.info/` as a primary reference (should be local).

**Impact:** these skills cannot currently be indexed or auto-triggered by any registry that relies on frontmatter — they only work if a human or agent already knows to open the file directly.

### engram — systemically off-format (0/21 compliant, 17 partial)

Every file under `engram/skills/*` uses a YAML **folded block scalar** (`description: >`) for the description instead of the required single quoted line — this alone breaks the frontmatter contract workspace-wide. None uses the required section names (freeform headers like `Core Guardrails`, `Decision Rules`, `Workflow`, `Resources` instead). No file has a labeled `Output Contract`. Notable body-budget outliers: `backlog-triage` (~2400 tokens, 2.4× the hard max), `branch-pr` (~1250 tokens) and `issue-creation` (~1200 tokens) both duplicate large chunks of each other's content (conventional-commit rules, PR templates) instead of sharing one source. The two `plugin/*/skills/memory/SKILL.md` variants (claude-code and codex) are near-duplicates of each other, both missing `license`/`metadata.author`/`metadata.version`, and both ~1140 tokens (over hard max).

**Impact:** even where content quality is good, none of engram's skills would pass automated frontmatter validation; duplicated content (branch-pr/commit-hygiene, the two memory plugin variants) creates a two-places-to-update maintenance burden.

### hgtran-ai — best but still majority non-compliant (8/30 compliant)

This is the repo that owns the standard, yet 20 of 30 skills don't follow it. The `sdd-*` family (`sdd-design`, `sdd-explore`, `sdd-propose`, `sdd-spec`, `sdd-tasks`, `sdd-apply`, `sdd-archive`, `sdd-onboard`) uses a different, older template shape (`Purpose` / `What You Receive` / `Rules` at the bottom instead of Hard Rules near the top) and several embed large templates inline that belong in `assets/`/`references/` — `sdd-tasks` and `sdd-apply` are the worst body-budget offenders (~1800–2800 tokens). The four domain-standard skills (`db-change-standard`, `frontend-crud-standard`, `backend-crud-standard`, `worker-service-standard`) are written as freeform Spanish prose with no required sections at all; `worker-service-standard` additionally violates the "no tutorial background" rule extensively. 13 of 30 files put the "what it does" clause before `Trigger:` in the description, inverting the required order.

**Anomaly requiring human review:** `sdd-apply` and `sdd-verify` each contain **two full YAML frontmatter blocks** in one file (labeled `model-capable` / `model-small`), where only the first is real frontmatter and the second is inert duplicate body text. This is unusual enough that it should be confirmed as intentional (a model-tiering mechanism) or fixed as a copy-paste artifact before any automated tooling parses these files.

## Decision

1. **Adopt `hgtran-ai/docs/skill-style-guide.md` as the single cross-repo standard** for every `SKILL.md` in `engram`, `hgtran-ai`, and `hgtran-guardian-angel` — no repo-local dialect.
2. **Treat frontmatter-completeness as the highest-priority fix**, since it is the precondition for any registry to discover a skill at all. Fix order:
   - P0: `hgtran-guardian-angel`'s 6 skills (no frontmatter — completely undiscoverable).
   - P0: `engram`'s 21 skills (folded-scalar description breaks single-line requirement workspace-wide) + the 2 `plugin/*/skills/memory` variants (missing `license`/`metadata.*`).
   - P1: `hgtran-ai`'s `sdd-*` family and 4 `*-standard` skills (wrong section shape, but at least have valid frontmatter in most cases).
3. **Resolve the `sdd-apply`/`sdd-verify` duplicate-frontmatter anomaly** before any further edits to those files — confirm intent with the author, then either formalize the model-tiering pattern as a documented style-guide exception or collapse it to one frontmatter block.
4. **De-duplicate overlapping content**: merge `branch-pr`/`commit-hygiene` overlap (both `engram` and `hgtran-ai` versions) into one shared source per repo; unify the two `memory` plugin variants in `engram` around one canonical body with thin per-platform wrappers.
5. **Defer body-budget trims** (moving inline templates/examples to `assets/`/`references/`) to a second pass, after structural/frontmatter compliance is fixed — reformatting an already-noncompliant file twice wastes effort.
6. Use `skill-improver` in **apply mode**, skill-by-skill, to execute the remediation — audit-only was used here deliberately; no files were changed as part of this ADR.

## Consequences

- Until P0 fixes land, `hgtran-guardian-angel`'s skills and `engram`'s skills cannot be reliably auto-discovered by any registry that reads frontmatter — they are effectively invisible to automated skill routing today, however well-written their content is.
- Fixing frontmatter and section structure first, body-budget second, means some skills will pass discovery checks before they pass token-budget checks — this is an accepted intermediate state, not a final compliance bar.
- Consolidating duplicated content (branch-pr/commit-hygiene, memory plugin variants) reduces the number of places a future policy change (e.g. commit message format) must be edited, at the cost of introducing a shared-reference file that both variants must point to correctly.
- No skill behavior changes as a result of this ADR alone — it only records the baseline and remediation plan. Actual edits happen in follow-up `skill-improver` apply-mode passes, one skill/repo at a time, so each change stays reviewable.

## Open Question for Human Review

Is the `sdd-apply`/`sdd-verify` dual-frontmatter block (`model-capable` / `model-small`) an intentional model-tiering mechanism that the style guide should be extended to document, or a copy-paste artifact to clean up? This must be resolved before those two files are touched by any remediation pass.
