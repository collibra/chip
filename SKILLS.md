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

**`data_assets_discover`** — Natural language semantic search over data assets (tables, columns, datasets). Use when the user asks open-ended questions like "what data do we have about customers?". Requires `dgc.ai-copilot` permission.

**`business_glossary_discover`** — Natural language semantic search over the business glossary (terms, acronyms, KPIs, definitions). Use when the user asks about the meaning of a business concept. Requires `dgc.ai-copilot` permission.

**`asset_keyword_search`** — Wildcard keyword search. Returns names, IDs, and metadata but not full asset details. Use this to find an asset's UUID when you only know its name. Supports filtering by resource type, community, domain, asset type, status, and creator. Paginated via `limit`/`offset`.

**`asset_types_list`** — List all asset type names and UUIDs. Use this when you need a type UUID to filter `asset_keyword_search` results.

### Asset Details

**`asset_details_get`** — Retrieve full details for a single asset by UUID: attributes, relations, and metadata. Returns a direct link to the asset in the Collibra UI. Relations are paginated (50 per page); use `outgoingRelationsCursor` and `incomingRelationsCursor` from the previous response to page through them.

### Semantic Graph Traversal

These tools walk the Collibra asset relation graph to answer lineage and semantic questions. All require asset UUIDs as input.

**`column_semantics_get`** — Given a column UUID, returns all connected Data Attributes with their descriptions, linked Measures, and generic business assets. Use to answer "what does this column mean semantically?".

**`table_semantics_get`** — Given a table UUID, returns all columns with their Data Attributes and connected Measures. Use to answer "what metrics use data from this table?" or "what is the semantic context of this table?".

**`measure_data_get`** — Given a measure UUID, traces backward through Data Attributes to the underlying Columns and their parent Tables. Use to answer "what physical data feeds this metric?".

**`business_term_data_get`** — Given a business term UUID, traces through Data Attributes to connected Columns and Tables. Use to answer "what physical data is associated with this business term?".

### Data Classification

**`data_class_search`** — Search for data classes by name or description. Use this to find a classification UUID before applying it to an asset. Requires `dgc.data-classes-read` permission.

**`data_classification_match_search`** — Search existing classification matches (associations between data classes and assets). Filter by asset IDs, classification IDs, or status (`ACCEPTED`, `REJECTED`, `SUGGESTED`). Requires `dgc.classify` + `dgc.catalog`.

**`data_classification_match_add`** — Apply a data class to an asset. Requires both the asset UUID and classification UUID. Requires `dgc.classify` + `dgc.catalog`.

**`data_classification_match_remove`** — Remove a classification match. Requires `dgc.classify` + `dgc.catalog`.

### Data Contracts

**`data_contract_list`** — List data contracts with cursor-based pagination. Filter by `manifestId`. Use this to find a contract's UUID.

**`data_contract_manifest_pull`** — Download the manifest for a data contract by UUID.

**`data_contract_manifest_push`** — Upload/update a manifest for a data contract by UUID.

---

## Common Workflows

### Find an asset and get its details
1. `asset_keyword_search` with the asset name → get UUID from results
2. `asset_details_get` with the UUID → get full attributes and relations

### Classify a column
1. `asset_keyword_search` to find the column UUID
2. `data_class_search` to find the data class UUID
3. `data_classification_match_add` with both UUIDs

### Understand what a table means
1. `asset_keyword_search` to find the table UUID
2. `table_semantics_get` → columns → data attributes → measures

### Trace a metric to its source data
1. `asset_keyword_search` to find the measure UUID
2. `measure_data_get` → data attributes → columns → tables

### Trace a business term to physical data
1. `asset_keyword_search` to find the business term UUID
2. `business_term_data_get` → data attributes → columns → tables

### Manage a data contract
1. `data_contract_list` to find the contract UUID
2. `data_contract_manifest_pull` to download, edit, then `data_contract_manifest_push` to update

---

## Tips

- **UUIDs are required for most tools.** When you only have a name, start with `asset_keyword_search` or the natural language discovery tools to get the UUID first.
- **`data_assets_discover` vs `asset_keyword_search`**: Prefer `data_assets_discover` for open-ended semantic questions; prefer `asset_keyword_search` when you know the exact name or need to filter by type/community/domain.
- **Permissions**: `data_assets_discover` and `business_glossary_discover` require the `dgc.ai-copilot` permission. Classification tools require `dgc.classify` + `dgc.catalog`. If a tool fails with a permission error, let the user know which permission is needed.
- **Pagination**: `asset_keyword_search`, `asset_types_list`, `data_class_search`, and `data_classification_match_search` use `limit`/`offset`. `data_contract_list` and `asset_details_get` (for relations) use cursor-based pagination — carry the cursor from the previous response.
- **Error handling**: Validation errors are returned in the output `error` field (not as Go errors), so always check `error` and `success`/`found` fields in the response before using the data.
