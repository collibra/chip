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

**`create_asset`** — Create a new asset of any type in Collibra in a single call. Accepts human-friendly identifiers and resolves them server-side: `assetType` matches against UUID, publicId (e.g. `"BusinessTerm"`), or display name (e.g. `"Business Term"`); `domain` against UUID or display name; `status` against UUID or status name (e.g. `"Candidate"`); `attributes` reference attribute types by `name` (e.g. `"Definition"`) or by `typeId`. Markdown in `RICH_TEXT` attribute values is converted to HTML server-side so it renders correctly in the Collibra UI. By default, `allowDuplicate=false` returns `status=duplicate_found` without writing if an asset with the same name already exists in the resolved (assetType, domain); pass `allowDuplicate=true` to bypass. Validation errors (e.g. unknown asset type, type not allowed in domain, unknown attribute) return suggestion-rich messages so the agent can self-correct in one round. Returns one of: `success`, `duplicate_found`, `validation_error`, or `error`. Destructive (creates a new asset).

**`prepare_create_asset`** — Read-only companion to `create_asset`. Use **only** when the agent needs to *browse* what's creatable on this instance or *inspect* an asset type's full schema before composing a create. Without inputs, returns available asset types. With just `assetType`, returns the domains compatible with that type. With just `domain`, returns the asset types creatable in that domain. With both, returns the scoped attribute and relation schema (pass `includeStringType=true` to also populate each attribute's `stringType` like `RICH_TEXT` plus its description). Calling this before `create_asset` is **optional** — `create_asset` does its own resolution, validation, and duplicate-check. Read-only.

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

### Data Access

These tools query Collibra Data Access — the system that manages who can access what data, through grants, masks, filters, and groups.

**`search_data_access_identities`** — Search for Data Access users (identities) by name and/or email. Providing `email` performs an exact lookup via `GetUserByEmail`. Providing `name` performs a server-side case-insensitive contains search via `SearchUsers`. Both can be combined: email resolves the user, name filters the result client-side. Name-only searches are paginated (25 per page) — use the returned `nextCursor` to fetch subsequent pages.

**`search_data_access_objects`** — Search for data objects in Collibra Data Access (tables, columns, schemas, views, and other entities tracked in registered data sources). Filters can be combined: `name` (case-insensitive contains), `dataSources` (data source IDs), `types` (e.g. `table`, `column`, `schema`, `view`), `parents` / `ancestors` (other data object IDs to scope the search to a sub-tree), and `includeDeleted`. Returns up to `pageSize` matches (default 25, max 25). Each result includes the data object ID, name, fully qualified name, type, data type, deleted flag, description, data source ID, and `applicablePermissions` — the list of source-system permissions (each with a `name` and `description`) that can be requested on the object. Use those names when populating `what[].permissions` for `create_data_access_request`.

**`create_data_access_request`** — Create a new Collibra Data Access request on behalf of one or more users for one or more data objects. Destructive. Required behavior:

- **Minimum input is WHO, WHAT, and a purpose.** Do not call this tool until all three are supplied.
- **WHO** must be resolved via `search_data_access_identities` (by email or name) — pass the returned user IDs in `userIds`. Never pass raw emails or names.
- **WHAT** must be resolved via `search_data_access_objects` — pass the returned data object IDs in `what[].dataObjectId`. Per item, `permissions` should be empty and `globalPermissions` must always be READ.
- **Purpose** is mandatory and must come from the user — it is the business justification for the request. If the user has not stated a purpose, ask them for one before calling the tool. Do not invent a purpose. The tool always appends a note stating that the request was created by AI.
- **Name** is optional. If the user does not provide one, omit `name` on the first call. The tool will return status `needs_name_confirmation` with a `suggestedName` derived from the purpose — present that suggestion to the user, get their confirmation (or an alternative), and call again with the confirmed value in `name`.

### Data Contracts

**`list_data_contract`** — List data contracts with cursor-based pagination. Filter by `manifestId`. Use this to find a contract's UUID.

**`pull_data_contract_manifest`** — Download the manifest for a data contract by UUID.

**`push_data_contract_manifest`** — Upload/update a manifest for a data contract by UUID.

---

## Common Workflows

### Create an asset
Single call — `create_asset` does its own resolution, validation, and duplicate check.

1. `create_asset` with the asset's `name`, `assetType` (e.g. `"Business Term"` or `"BusinessTerm"`), `domain` (name or UUID), optional `status` (name or UUID), and optional `attributes` by `name` (e.g. `[{"name": "Definition", "value": "Monthly Recurring Revenue"}]`)
2. Read the response status:
   - `success` → done; the response includes the new asset's UUID and a per-attribute outcome list
   - `duplicate_found` → an asset with the same name already exists in this (assetType, domain). The response includes the existing asset's ID. Confirm with the user, then re-call with `allowDuplicate: true` to create anyway
   - `validation_error` → the message includes suggestions (available asset types, compatible domains, valid attribute names, etc.). Self-correct and retry
   - `error` → unexpected downstream Collibra failure; surface the message to the user

**When to call `prepare_create_asset` first** (optional):
- The user is browsing ("what asset types can I create?", "what domains accept a Business Term?")
- The agent wants to know which attributes are required or `RICH_TEXT` before composing values
- A previous `create_asset` returned `validation_error` and the agent wants to enumerate the full set of options rather than rely on the message's truncated suggestions

For straightforward creates where the user provides asset type + domain, skip `prepare_create_asset` and call `create_asset` directly.

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

### Find and inspect data access roles
1. `get_data_access_control_details` with a specific ID → full details including grant category, policy rule, timestamps

### Find who has been granted access (roles)
1. `search_data_access_roles` with optional name/state filters → returns only Grant-type controls
2. `get_data_access_control_details` for any result ID → full grant details

### Look up a Data Access user by email or name
1. `search_data_access_identities` with `email` → exact lookup, returns the user's id, display name, and type
   — or with `name` → paginated server-side contains search across all users

### Create a Data Access request
1. Make sure the user has stated a `purpose` — the business justification for the request. If missing, ask for it before continuing.
2. `search_data_access_identities` for every beneficiary → collect the user IDs (the WHO)
3. `search_data_access_objects` for every data object the users need → collect the data object IDs (the WHAT)
4. `create_data_access_request` with `purpose`, `userIds`, and `what` — if the user has not provided a name, omit `name`. The tool returns `needs_name_confirmation` with a `suggestedName` derived from the purpose; confirm it with the user, then call again with `name` set. The purpose is used as the description, with an AI-created note appended automatically.

### Manage a data contract
1. `list_data_contract` to find the contract UUID
2. `pull_data_contract_manifest` to download, edit, then `push_data_contract_manifest` to update

---

## Tips

- **`create_asset` is self-sufficient — don't pre-flight it.** It resolves names to UUIDs, validates, and gates on duplicates internally. Calling `prepare_create_asset` first is purely optional and only useful for browsing or schema inspection (see the workflow above).
- **For RICH_TEXT attributes, write Markdown.** `create_asset` detects `RICH_TEXT` attributes (e.g. `Definition`) and converts Markdown to HTML server-side before writing. Use bold (`**...**`), links (`[text](url)`), bullet lists, and headings naturally; they render correctly in the Collibra UI. Plain-text attributes pass through unchanged.
- **UUIDs are required for most read tools.** When you only have a name, start with `search_asset_keyword` or the natural language discovery tools to get the UUID first.
- **`discover_data_assets` vs `search_asset_keyword`**: Prefer `discover_data_assets` for open-ended semantic questions; prefer `search_asset_keyword` when you know the exact name or need to filter by type/community/domain.
- **Permissions**: `discover_data_assets` and `discover_business_glossary` require the `dgc.ai-copilot` permission. Classification tools require `dgc.classify` + `dgc.catalog`. If a tool fails with a permission error, let the user know which permission is needed.
- **Pagination**: `search_asset_keyword`, `list_asset_types`, `search_data_class`, and `search_data_classification_match` use `limit`/`offset`. `list_data_contract` and `get_asset_details` (for relations) use cursor-based pagination — carry the cursor from the previous response. Lineage tools (`search_lineage_entities`, `get_lineage_upstream`, `get_lineage_downstream`, `search_lineage_transformations`) and data access tools (`search_data_access_controls`, `search_data_access_roles`) also use cursor-based pagination.
- **Error handling**: Validation errors are returned in the output `error` field (not as Go errors), so always check `error` and `success`/`found` fields in the response before using the data.
