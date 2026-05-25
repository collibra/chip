---
description: Pick between natural-language semantic search and keyword/filter search to find Collibra assets and terms.
related: collibra/lineage, collibra/asset-create
---

# Discovery — finding assets and business terms

Chip has three discovery entry points. They serve different shapes of question; picking the
wrong one is the most common cause of empty results.

## Decision rule

- **Open-ended, conceptual, or paraphrased question** ("what customer data do we have?",
  "find tables with revenue figures") → `discover_data_assets` (semantic search over data
  assets) or `discover_business_glossary` (semantic search over the business glossary).
- **Exact name known, or filtering by type/community/domain/status** → `search_asset_keyword`.
- **Need a type UUID** to filter `search_asset_keyword` by `assetType` → `list_asset_types`.

Do not start with `search_asset_keyword` for paraphrased questions. The keyword search will
miss synonyms, plurals, and definitional matches that semantic search returns.

## Permission gates

- `discover_data_assets` and `discover_business_glossary` require the `dgc.ai-copilot` scope.
  If a call returns a permission error, surface the scope name to the user — do not silently
  fall back to keyword search, because the result set will be qualitatively different.
- `search_asset_keyword` and `list_asset_types` work with default catalog read scopes.

## Workflow: from name to UUID

Most chip tools require a UUID. The pattern:

1. Call `discover_data_assets` (or `discover_business_glossary`, or `search_asset_keyword`)
   with the user's phrasing.
2. From the response, pick the UUID of the asset whose name and type match the user's intent.
   If multiple plausible matches exist, present them to the user and ask which they meant —
   do not guess.
3. Pass that UUID to the next tool (`get_asset_details`, `get_table_semantics`,
   `get_column_semantics`, `get_measure_data`, `get_business_term_data`, etc.).

## Hard rules

1. **Do not paraphrase the user when searching.** The semantic search ranks against the user's
   own wording — passing a rewritten query loses signal.
2. **Resolve ambiguity before drilling down.** If two assets share a name across domains, ask
   the user which they want. Do not call detail tools on a guess.
3. **`list_asset_types` is for type UUIDs, not for browsing.** Use it when you need a UUID to
   pass to `search_asset_keyword`'s `assetType` filter — not as a first-step exploration tool.

## Common follow-ups

- Found a **business term** → `get_business_term_data` to trace it to physical columns.
- Found a **table** → `get_table_semantics` for its columns + linked attributes/measures.
- Found a **column** → `get_column_semantics` for its data attributes and measures.
- Found a **measure** → `get_measure_data` to trace it to underlying columns and tables.
- Found a data asset whose lineage matters → `collibra/lineage`.
