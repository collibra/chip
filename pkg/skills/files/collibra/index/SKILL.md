---
description: Navigator for chip's Collibra skills. Start here when unsure which skill applies.
related: collibra/discovery, collibra/lineage, collibra/asset-create, collibra/asset-edit, collibra/data-product-create
---

# Collibra skills — navigator

This MCP server is a bridge to Collibra Data Governance Center. Skills under
`collibra/*` document the non-trivial workflows: when tools must be chained, when one ID space
must be bridged to another, and which permissions are required.

## When to load which skill

| Task | Skill |
|---|---|
| Find data about a topic by meaning (semantic) vs by exact name (keyword) | `collibra/discovery` |
| Trace upstream sources, downstream consumers, or impact of a change | `collibra/lineage` |
| Create any new asset (Business Term, Table, Column, KPI, …) | `collibra/asset-create` |
| Modify an existing asset's attributes, relations, tags, status, or owners | `collibra/asset-edit` |
| Register a table (and its dimension tables) as a Collibra Data Product with ports | `collibra/data-product-create` |

If a task is a single tool call with no chaining (e.g. `get_asset_details` by UUID,
`list_asset_types`, `pull_data_contract_manifest`), no skill is needed — the tool's own
description is sufficient.

## Hard rules that apply across skills

1. **Lineage entity IDs are not DGC asset UUIDs.** Never pass a DGC UUID directly to
   `get_lineage_upstream` or `get_lineage_downstream`. Bridge via `search_lineage_entities`
   first. See `collibra/lineage` for the full rule.

2. **UUIDs are required for most read tools.** When the user gives you a name, resolve it
   first — `search_asset_keyword` for exact/filterable lookup, `discover_data_assets` or
   `discover_business_glossary` for natural-language search. See `collibra/discovery`.

3. **RICH_TEXT attributes accept Markdown.** `create_asset` and `edit_asset` convert Markdown to
   HTML server-side for RICH_TEXT attributes (e.g. `Definition`). Write Markdown naturally;
   never pre-render to HTML. See `collibra/asset-create`.

4. **Surface permission errors verbatim.** When a tool fails with a missing-scope error
   (`dgc.ai-copilot`, `dgc.classify`, `dgc.catalog`, `dgc.data-classes-read`,
   `dgc.data-classes-edit`, `dgc.data-contract`), tell the user which scope is missing rather
   than retrying or working around it.

## Pagination

- `search_asset_keyword`, `list_asset_types`, `search_data_class`,
  `search_data_classification_match` — `limit` / `offset`.
- `list_data_contract`, `get_asset_details` (relations), and all lineage tools — cursor based.
  Carry the cursor from the previous response.
