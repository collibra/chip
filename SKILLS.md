# Skills

Chip ships per-domain skill guides embedded in the binary and served via two MCP tools:

- `list_collibra_skills` — list available skills, optionally with one-line descriptions
- `load_collibra_skill` — load a skill body, header-only metadata, or a bundled reference

A connected MCP client (Claude Code, VS Code, Gemini CLI, …) discovers them at runtime via
those tools. Skill content lives in [`pkg/skills/files/collibra/`](pkg/skills/files/collibra/).

## Current skills

| Name | Topic |
|---|---|
| `collibra/index` | Navigator — start here when unsure which skill applies |
| `collibra/discovery` | Semantic vs keyword search; resolving names to UUIDs |
| `collibra/lineage` | Technical lineage; DGC UUID ↔ lineage entity ID bridge; column-level workaround |
| `collibra/asset-create` | `create_asset` workflow; RICH_TEXT Markdown handling; duplicate gating |
| `collibra/asset-edit` | `edit_asset` operation types |

Each skill is one `SKILL.md` per directory, with frontmatter (`description`, `related`, `shared`,
`requires`) and an optional `references/` directory for bundled reference documents.

## Capability-gated skills

A skill whose workflow depends on tools that are only registered behind a capability flag declares
that with `requires:` in its frontmatter:

```yaml
---
description: …
related: collibra/discovery
requires: data-quality
---
```

Such a skill is left out of the catalog unless the capability is on, so the agent is never handed a
guide for tools it cannot call. `data-quality` (the `--data-quality` flag) is the only capability a
skill can require today; any other value fails catalog load. This applies to external skills loaded
from `--skills-dir` as well, so your own skills can gate themselves.

Because filtering the catalog does not rewrite markdown, `collibra/index` does not name the
capability-gated skills — they are discoverable through `list_collibra_skills` when served.

## Adding or updating a skill

1. Edit or create `pkg/skills/files/collibra/<name>/SKILL.md`.
2. For long-form supporting material, add files under `pkg/skills/files/collibra/<name>/references/`.
3. Rebuild — `//go:embed` picks them up automatically. The catalog parses the embedded
   filesystem at server startup.

Skills are intentionally narrow: write one only when a workflow requires multiple tools, an
ID-bridging rule, an error-recovery loop, or a format quirk that the tool's own description
cannot carry. A skill that only restates a tool's description does not need to exist.

## Bootstrap

`pkg/chip/instructions.md` is served on `initialize` and tells the model to discover skills
via `list_collibra_skills` before composing multi-step workflows. That is the entry point —
no additional handshake is required from clients.
