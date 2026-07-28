# Collibra MCP Server

This server connects you to Collibra, an enterprise data governance platform — the authoritative catalog for what data the organization has, what it means, how it relates, and who governs it.

Reach for these tools when the user asks about **discovering, understanding, or governing data**: "what customer data do we have?", "what does this metric measure?", "which columns hold PII?", "where does this KPI come from?". These tools operate on metadata and governance, not on the underlying row-level data.

## Key concepts

- **Assets**: any item in the catalog — its `assetType` (Table, Column, Business Term, KPI, Report, Policy, …) determines what it represents. Tools like `create_asset`, `edit_asset`, `get_asset_details` all operate on this uniform concept.
- **Business Terms**: standardized definitions for business concepts.
- **Data Contracts**: agreements defining data product interfaces.
- **Classifications**: data classes applied to assets to categorize them — most commonly for data sensitivity (PII, PHI), but also for arbitrary taxonomies.

## Tool categories

- **Discovery**: `discover_data_assets`, `discover_business_glossary` (natural-language semantic search; require `dgc.ai-copilot`), `search_asset_keyword` (wildcard + filters), `list_asset_types`.
- **Asset details**: `get_asset_details` (by UUID; relations paginated by cursor).
- **Semantic graph**: `get_column_semantics`, `get_table_semantics`, `get_measure_data`, `get_business_term_data` — walk the catalog graph between columns, data attributes, measures, and business terms. All take asset UUIDs.
- **Technical lineage**: `search_lineage_entities` (entry point), `get_lineage_upstream` / `get_lineage_downstream` (impact analysis), `get_lineage_entity` (resolve IDs), `get_lineage_transformation` (SQL/logic), `search_lineage_transformations`. Lineage uses its own entity IDs — not DGC asset UUIDs — so always start with `search_lineage_entities` to bridge.
- **Classification**: `search_data_class`, `search_data_classification_match`, `add_data_classification_match`, `remove_data_classification_match` (require `dgc.classify` + `dgc.catalog`).
- **Asset writes**: `create_asset` (one smart write tool for any asset type), `prepare_create_asset` (optional read-only companion for browsing/inspection), `edit_asset` (typed operations on existing assets).
- **Data contracts**: `list_data_contract`, `init_data_contract` (create a new contract governing a Data Product Port), `pull_data_contract_manifest`, `push_data_contract_manifest` (add manifest versions to an existing contract).

## Recommended workflows

- **Find data about X** → start with `discover_data_assets` or `discover_business_glossary` (semantic) before falling back to `search_asset_keyword` (exact/filter).
- **Understand a table** → `search_asset_keyword` for the UUID → `get_table_semantics` → drill into columns.
- **What does this term mean** → `discover_business_glossary` → `get_business_term_data` for connected physical data.
- **Trace a metric to its source** → find measure UUID → `get_measure_data` → data attributes → columns → tables.
- **Upstream/downstream lineage** → `search_lineage_entities` → `get_lineage_upstream` or `get_lineage_downstream`. Summarize from graph structure; only call `get_lineage_entity` for the most relevant IDs, only call `get_lineage_transformation` when the user asks for the SQL.
- **Classify a column** → `search_asset_keyword` for column UUID → `search_data_class` for class UUID → `add_data_classification_match`.
- **Create an asset** → `create_asset` directly with `name` + `assetType` + `domain` (names or UUIDs both accepted) + optional `attributes`. Markdown in `RICH_TEXT` attributes (e.g. `Definition`) is auto-converted to HTML. Read the response status: `success`, `duplicate_found` (re-call with `allowDuplicate: true` if intentional), `validation_error` (message includes suggestions — self-correct and retry), or `error`. `prepare_create_asset` first is **optional**, only useful for browsing or schema inspection.

## Key patterns

- **Prefer semantic search over keyword** for open-ended questions. Use keyword search when you know the exact name or need filters.
- **UUIDs are required for most read tools.** Resolve names to UUIDs first via `search_asset_keyword` or the natural-language discovery tools.
- **Lineage IDs ≠ DGC UUIDs.** Always bridge via `search_lineage_entities`. Column-level lineage has limitations: `nameContains` doesn't work for columns and `dgcId` is unreliable — go through the parent table instead.
- **Check existing classifications** with `search_data_classification_match` before applying new ones.
- **Permission errors**: surface to the user which permission is needed (`dgc.ai-copilot`, `dgc.classify` + `dgc.catalog`, etc.).
