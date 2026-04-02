# SKILLS.md

This file describes the MCP tools available in this server and how Claude agents should use them effectively.

## What is Collibra?

Collibra is a data governance platform — a central catalog where an organization documents, classifies, and governs its data assets. It is the authoritative source for:

- **What data exists**: tables, columns, datasets, reports, APIs, and other data assets across the organization
- **What data means**: a rich business glossary of terms, acronyms, KPIs, and definitions that captures how the business interprets and communicates about data — the authoritative place to resolve ambiguity around business language
- **How data relates**: lineage between physical columns, semantic data attributes, business terms, and measures
- **Who owns and trusts it**: stewards, data contracts, classifications, and quality rules

Reach for Collibra tools when the user's question is about **understanding, discovering, or governing data in the organization** — e.g. "what customer data do we have?", "what does this metric measure?", "which columns contain PII?", or "where does this KPI come from?". These tools are not appropriate for querying the actual data values in a database; they operate on the metadata and governance layer above the data.

## Tool Inventory

### Discovery & Search

**`discover_data_assets`** — Natural language semantic search over data assets (tables, columns, datasets). Use when the user asks open-ended questions like "what data do we have about customers?". Requires `dgc.ai-copilot` permission.

**`discover_business_glossary`** — Natural language semantic search over the business glossary (terms, acronyms, KPIs, definitions). Use when the user asks about the meaning of a business concept. Requires `dgc.ai-copilot` permission.

**`search_asset_keyword`** — Wildcard keyword search. Returns names, IDs, and metadata but not full asset details. Use this to find an asset's UUID when you only know its name. Supports filtering by resource type, community, domain, asset type, status, and creator. Paginated via `limit`/`offset`.

**`list_asset_types`** — List all asset type names and UUIDs. Use this when you need a type UUID to filter `search_asset_keyword` results.

### Asset Details

**`get_asset_details`** — Retrieve full details for a single asset by UUID: attributes, relations, and metadata. Returns a direct link to the asset in the Collibra UI. Relations are paginated (50 per page); use `outgoingRelationsCursor` and `incomingRelationsCursor` from the previous response to page through them.

### Semantic Graph Traversal

These tools walk the Collibra asset relation graph to answer lineage and semantic questions. All require asset UUIDs as input.

**`get_column_semantics`** — Given a column UUID, returns all connected Data Attributes with their descriptions, linked Measures, and generic business assets. Use to answer "what does this column mean semantically?".

**`get_table_semantics`** — Given a table UUID, returns all columns with their Data Attributes and connected Measures. Use to answer "what metrics use data from this table?" or "what is the semantic context of this table?".

**`get_measure_data`** — Given a measure UUID, traces backward through Data Attributes to the underlying Columns and their parent Tables. Use to answer "what physical data feeds this metric?".

**`get_business_term_data`** — Given a business term UUID, traces through Data Attributes to connected Columns and Tables. Use to answer "what physical data is associated with this business term?".

### Data Classification

**`search_data_class`** — Search for data classes by name or description. Use this to find a classification UUID before applying it to an asset. Requires `dgc.data-classes-read` permission.

**`search_data_classification_match`** — Search existing classification matches (associations between data classes and assets). Filter by asset IDs, classification IDs, or status (`ACCEPTED`, `REJECTED`, `SUGGESTED`). Requires `dgc.classify` + `dgc.catalog`.

**`add_data_classification_match`** — Apply a data class to an asset. Requires both the asset UUID and classification UUID. Requires `dgc.classify` + `dgc.catalog`.

**`remove_data_classification_match`** — Remove a classification match. Requires `dgc.classify` + `dgc.catalog`.

### Technical Lineage

These tools query the technical lineage graph — a map of all data objects and transformations across external systems, including unregistered assets, temporary tables, and source code. Unlike business lineage (which only covers assets in the Collibra Data Catalog), technical lineage covers the full physical data flow.

**`search_lineage_entities`** — Search for data entities in the technical lineage graph by name, type, or DGC UUID. Use this as a starting point when you don't have an entity ID. Supports partial name matching and type filtering (e.g. `table`, `column`, `report`). Paginated.

**`get_lineage_entity`** — Get full metadata for a specific lineage entity by ID: name, type, source systems, parent entity, and linked DGC identifier. Use after obtaining an entity ID from a search or lineage traversal.

**`get_lineage_upstream`** — Get all upstream entities (sources) for a data entity, along with the transformations connecting them. Use to answer "where does this data come from?". Paginated.

**`get_lineage_downstream`** — Get all downstream entities (consumers) for a data entity, along with the transformations connecting them. Use to answer "what depends on this data?" or "what is impacted if this changes?". Paginated.

**`search_lineage_transformations`** — Search for transformations by name. Returns lightweight summaries. Use to discover ETL jobs or SQL queries by name.

**`get_lineage_transformation`** — Get the full details of a transformation, including its SQL or script logic. Use after finding a transformation ID in an upstream/downstream result or search.

### Data Contracts

**`list_data_contract`** — List data contracts with cursor-based pagination. Filter by `manifestId`. Use this to find a contract's UUID.

**`pull_data_contract_manifest`** — Download the manifest for a data contract by UUID.

**`push_data_contract_manifest`** — Upload/update a manifest for a data contract by UUID.

---

## Common Workflows

### Find an asset and get its details
1. `search_asset_keyword` with the asset name → get UUID from results
2. `get_asset_details` with the UUID → get full attributes and relations

### Classify a column
1. `search_asset_keyword` to find the column UUID
2. `search_data_class` to find the data class UUID
3. `add_data_classification_match` with both UUIDs

### Understand what a table means
1. `search_asset_keyword` to find the table UUID
2. `get_table_semantics` → columns → data attributes → measures

### Trace a metric to its source data
1. `search_asset_keyword` to find the measure UUID
2. `get_measure_data` → data attributes → columns → tables

### Trace a business term to physical data
1. `search_asset_keyword` to find the business term UUID
2. `get_business_term_data` → data attributes → columns → tables

### Trace upstream lineage for a data asset
1. `search_lineage_entities` with the asset name → get entity ID
2. `get_lineage_upstream` → relations with source entity IDs and transformation IDs
3. `get_lineage_entity` for any source entity to get its details
4. `get_lineage_transformation` for any transformation ID to see the logic

### Perform impact analysis (downstream)
1. `search_lineage_entities` with the asset name → get entity ID
2. `get_lineage_downstream` → relations with consumer entity IDs
3. Follow up with `get_lineage_entity` for specific consumers as needed

### Manage a data contract
1. `list_data_contract` to find the contract UUID
2. `pull_data_contract_manifest` to download, edit, then `push_data_contract_manifest` to update

---

## Tips

- **UUIDs are required for most tools.** When you only have a name, start with `search_asset_keyword` or the natural language discovery tools to get the UUID first.
- **`discover_data_assets` vs `search_asset_keyword`**: Prefer `discover_data_assets` for open-ended semantic questions; prefer `search_asset_keyword` when you know the exact name or need to filter by type/community/domain.
- **Permissions**: `discover_data_assets` and `discover_business_glossary` require the `dgc.ai-copilot` permission. Classification tools require `dgc.classify` + `dgc.catalog`. If a tool fails with a permission error, let the user know which permission is needed.
- **Pagination**: `search_asset_keyword`, `list_asset_types`, `search_data_class`, and `search_data_classification_match` use `limit`/`offset`. `list_data_contract` and `get_asset_details` (for relations) use cursor-based pagination — carry the cursor from the previous response. Lineage tools (`search_lineage_entities`, `get_lineage_upstream`, `get_lineage_downstream`, `search_lineage_transformations`) also use cursor-based pagination.
- **Error handling**: Validation errors are returned in the output `error` field (not as Go errors), so always check `error` and `success`/`found` fields in the response before using the data.
