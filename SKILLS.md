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

### Asset Creation

**`prepare_create_asset`** — Resolve asset type and domain by name or ID, hydrate the full attribute schema, and check for duplicates. Returns a structured status (`ready`, `incomplete`, `needs_clarification`, `duplicate_found`) with pre-fetched options for missing fields. **Always call this before `create_asset`** to obtain the resolved UUIDs and validate inputs. Read-only.

**`create_asset`** — Create a new data asset in Collibra with optional attributes. Requires the resolved asset type UUID, domain UUID, and asset name — use the values returned by `prepare_create_asset`. Destructive (creates a new asset).

**`prepare_add_business_term`** — Validate business term data, resolve domains by name, check for duplicates, and hydrate the attribute schema for the Business Term type. Returns structured status with pre-fetched options for missing fields. **Always call this before `add_business_term`**. Read-only.

**`add_business_term`** — Create a business term asset with an optional definition and additional attributes. Requires the domain UUID — use the resolved domain from `prepare_add_business_term`. Destructive (creates a new asset).

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

**Workflow**: Almost all lineage questions follow the same pattern: **(1)** `search_lineage_entities` → **(2)** `get_lineage_upstream` or `get_lineage_downstream` → **(3)** optionally `get_lineage_entity` for the most relevant entities only. Do not resolve every entity ID — summarize from the graph structure and only look up entities the user specifically needs details on. Only call `get_lineage_transformation` when the user asks to see actual SQL or logic.

**IMPORTANT — ID types**: Lineage tools use their own internal entity IDs, which are **not** the same as DGC asset UUIDs. You cannot pass a DGC asset UUID directly to `get_lineage_upstream` or `get_lineage_downstream`. To bridge from the catalog to the lineage graph, call `search_lineage_entities` with the asset's UUID as `dgcId` to obtain the lineage entity ID first.

**LIMITATION — Column-level lineage**: Columns cannot be searched by name in `search_lineage_entities` (`nameContains` does not work for columns). The `dgcId` parameter also does not reliably resolve columns because there is no consistent mapping between Collibra catalog column UUIDs and technical lineage entity IDs. To reach a column in the lineage graph, first find its parent table (by name or `dgcId`), then use `get_lineage_upstream` or `get_lineage_downstream` on the table to discover its columns in the lineage graph.

**`search_lineage_entities`** *(entry point)* — Search by name, type, or DGC UUID. **Start here** for almost all lineage questions to resolve an entity name or DGC asset UUID to a lineage entity ID. Supports partial name matching and type filtering (e.g. `table`, `column`, `report`). Paginated. **Note**: name search and DGC UUID lookup do not work reliably for columns — see limitation above.

**`get_lineage_upstream`** *(step 2: trace sources)* — Given a lineage entity ID (not a DGC UUID), returns all upstream source entities and connecting transformations. Use to answer "where does this data come from?". Results contain entity IDs only. Paginated.

**`get_lineage_downstream`** *(step 2: trace consumers)* — Given a lineage entity ID (not a DGC UUID), returns all downstream consumer entities and connecting transformations. Use for impact analysis: "what depends on this?", "what breaks if this changes?". Results contain entity IDs only. Paginated.

**`get_lineage_entity`** *(follow-up: resolve IDs)* — Get full metadata for a specific lineage entity by its lineage ID (not a DGC UUID): name, type, source systems, parent entity, and linked DGC identifier. Only call this for the most relevant entity IDs from upstream/downstream results — do not resolve every ID.

**`get_lineage_transformation`** *(terminal: view logic)* — Get the full details of a transformation, including its SQL or script logic. Only call when the user explicitly asks about the transformation code. Do not call just to understand the lineage graph.

**`search_lineage_transformations`** *(specialized)* — Search for transformations by name. Only use when the user explicitly asks about a transformation by name. This is **not** a general entry point for lineage questions — start with `search_lineage_entities` instead.

### Data Contracts

**`list_data_contract`** — List data contracts with cursor-based pagination. Filter by `manifestId`. Use this to find a contract's UUID.

**`pull_data_contract_manifest`** — Download the manifest for a data contract by UUID.

**`push_data_contract_manifest`** — Upload/update a manifest for a data contract by UUID.

---

## Common Workflows

### Create any asset
1. `prepare_create_asset` with the asset name, asset type (publicId), and domain ID → check status is `ready`
2. `create_asset` with the resolved `assetTypeId` and `domainId` from step 1

### Add a business term
1. `prepare_add_business_term` with the term name and domain name or ID → check status is `ready`
2. `add_business_term` with the resolved `domainId` from step 1, plus optional definition and attributes

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
3. Summarize based on the graph structure — only call `get_lineage_entity` for the most relevant source entities, not all of them
4. Only call `get_lineage_transformation` if the user explicitly asks to see the SQL or logic

### Perform impact analysis (downstream)
1. `search_lineage_entities` with the asset name → get entity ID
2. `get_lineage_downstream` → relations with consumer entity IDs
3. Summarize based on the graph structure — only call `get_lineage_entity` for the most relevant consumers, not all of them

### Manage a data contract
1. `list_data_contract` to find the contract UUID
2. `pull_data_contract_manifest` to download, edit, then `push_data_contract_manifest` to update

---

## Tips

- **Always prepare before creating.** Call `prepare_create_asset` before `create_asset` and `prepare_add_business_term` before `add_business_term`, even if you already have the UUIDs. The prepare tools validate inputs, check for duplicates, and return the attribute schema.
- **UUIDs are required for most tools.** When you only have a name, start with `search_asset_keyword` or the natural language discovery tools to get the UUID first.
- **`discover_data_assets` vs `search_asset_keyword`**: Prefer `discover_data_assets` for open-ended semantic questions; prefer `search_asset_keyword` when you know the exact name or need to filter by type/community/domain.
- **Permissions**: `discover_data_assets` and `discover_business_glossary` require the `dgc.ai-copilot` permission. Classification tools require `dgc.classify` + `dgc.catalog`. If a tool fails with a permission error, let the user know which permission is needed.
- **Pagination**: `search_asset_keyword`, `list_asset_types`, `search_data_class`, and `search_data_classification_match` use `limit`/`offset`. `list_data_contract` and `get_asset_details` (for relations) use cursor-based pagination — carry the cursor from the previous response. Lineage tools (`search_lineage_entities`, `get_lineage_upstream`, `get_lineage_downstream`, `search_lineage_transformations`) also use cursor-based pagination.
- **Error handling**: Validation errors are returned in the output `error` field (not as Go errors), so always check `error` and `success`/`found` fields in the response before using the data.
