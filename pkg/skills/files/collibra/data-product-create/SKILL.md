---
description: Create a Collibra Data Product from a source table — discover related dimension tables, propose a star schema, then register the Data Product with one or two ports and link the tables.
related: collibra/asset-create, collibra/asset-edit, collibra/discovery
---

# Creating a Data Product from a table

Turns a single source table into a Collibra Data Product with one or two Ports and the right
`groups` / `exposes data as` relations. Phases 1–4 are read-only discovery and a proposal;
phase 5 is the writes, only after the user signs off. Tool-call mechanics (argument
resolution, status branches, RICH_TEXT, operation shapes) live in `collibra/asset-create`
and `collibra/asset-edit` — this skill is the DP-specific layer on top.

## Hard rules

1. **Write order is fixed.** All assets created before any relations; every relation source
   must already have a UUID. See Phase 5 for the exact sequence.
2. **Conservative auto-inclusion.** Only HIGH-confidence dimension tables join automatically.
   MEDIUM candidates require user confirmation — false positives in the join graph are worse
   than asking one extra question.
3. **Multi-tenant join template.** All same-schema joins are `fact.<fk> = dim.id AND
   fact.instance_name = dim.instance_name AND fact.date_partition = dim.date_partition`.
   Single-column joins on `fk = id` alone are wrong here.
4. **Infrastructure columns are never join keys.** Strip `instance_name`, `date_partition`,
   `creator`, `modifiedby`, `lastmodified`, `creationdate` before pattern-matching column
   names for FKs.
5. **Carry forward acceptable-use warnings.** If the source table has confidentiality or
   policy attributes, restate them in the Data Product `Description`.

## Phase 1 — Locate the source table

1. `search_asset_keyword` with `query=<user's term>`, `resourceTypeFilters=["Asset"]`,
   `limit=20`. If more than one result, list numbered and let the user pick.
2. `get_asset_details` on the chosen UUID. Read off: name, id, parent domain; schema (the
   `incomingRelations` source with `type.name == "Schema"`); columns (`outgoingRelations`
   targets with `type.name == "Column"`); existing description / sensitivity / refresh /
   acceptable-use attributes.

## Phase 2 — Find dimension tables

1. **Enumerate siblings.** `search_asset_keyword` with `query=<schema name>` and `limit=100`.
   Keep only results whose `name` starts with the full schema path
   (e.g. `redshift-prd-published>published>metadata_collector>`).
2. **Fetch each candidate** with `get_asset_details` and read its columns.
3. **Classify** after stripping infrastructure columns (rule 4):

   **HIGH — auto-include:**
   - Fact column `xxx_id` and a sibling table named like `xxx` (singular/plural fine) with
     an `id` column. Example: `attr_type_id` ↔ table `dgc_attribute_types` (column `id`).
   - Same pattern for `xxx_type` ↔ enum/type tables.

   **MEDIUM — ask the user:**
   - Shared non-infrastructure column names between fact and dimension (e.g. `role`,
     `vocabulary`, `assigned_type`) without a clean FK pattern.
   - Sibling tables that look like status/classification lookups but lack an `<x>_id` link.

4. Present HIGH (auto-included, with the detected FK) and MEDIUM (with reasoning); wait for
   the user to confirm which MEDIUM tables to include.

## Phase 3 — Detect the semantic layer

1. `get_table_semantics` on the source table and each selected dimension.
2. Walk `semanticHierarchy[*].connectedDataAttributes` in each result.
   - Any non-empty → **semantic layer exists** → plan **two ports** (physical/SQL + semantic/BI).
   - All empty → **one port** (physical only).

## Phase 4 — Propose the structure

Show the user, terse and skimmable: one block per asset (Data Product + each Port) with its
name and key attributes; under each Port, the list of tables to link, with the join
expression for each non-fact table written out per rule 3. End with a numbered action list
that mirrors the write order in rule 1 (assets first, then relations).

Ask "Proceed?" and do not start writing until the user confirms.

## Phase 5 — Create

Use `create_asset` and `edit_asset` per their skills — those cover argument resolution,
status branches, partial-failure semantics, and Markdown handling. This phase only specifies
*what* to write, in this order:

1. **Create assets** with `create_asset`, `domain: "Data Products"`:
   - **Data Product** — `assetType: "Data Product"`. Attributes: `Description`,
     `Business Case`, `Business Value` (RICH_TEXT — write Markdown), `Data Product Category`
     (`"Foundational"` or `"Derived"`), optional `Target Delivery Date`.
   - **Port(s)** — `assetType: "Data Product Port"`. Attributes: `Description` (RICH_TEXT),
     `Access Method`, `Access Instructions` (RICH_TEXT), optional `Area`,
     `Target Delivery Date`. One Port per output channel (Phase 3 decides if there are two).

2. **Wire relations** with `edit_asset add_relation`. The edited asset is always the head:
   - On the Data Product: one operation per Port, `relationType: "exposes data as"`,
     `targetAssetId: <port UUID>`.
   - On each Port: one operation per linked table, `relationType: "groups"`,
     `targetAssetId: <table UUID>`. Batch all of one Port's `groups` ops into a single
     `edit_asset` call.

3. **Report** Data Product UUID, Port UUIDs, count of linked tables. Surface any
   non-`success` status or per-operation failure verbatim — do not silently retry across
   phase 1 → 2.

## Naming

- **Data Product** — human-readable, derived from the source table.
  `metadata_collector.customer_metadata` → `"MDH Customer Metadata"`.
- **Port** — `<Data Product name> — <Access Method>`.
  `"MDH Customer Metadata — Redshift Published"`.

## Reference: DP-specific attributes and relations

General attribute/operation mechanics are in `collibra/asset-create` and
`collibra/asset-edit`. The tables below cover only what is unique to Data Products and
Ports.

| Attribute                       | Applies to                                 |
|---------------------------------|--------------------------------------------|
| Description (RICH_TEXT)         | Data Product, Port                         |
| Business Case (RICH_TEXT)       | Data Product                               |
| Business Value (RICH_TEXT)      | Data Product                               |
| Data Product Category           | Data Product — `Foundational` or `Derived` |
| Target Delivery Date            | Data Product, Port                         |
| Access Method                   | Port                                       |
| Access Instructions (RICH_TEXT) | Port                                       |
| Area                            | Port                                       |

| Head              | Role                   | Tail                       |
|-------------------|------------------------|----------------------------|
| Data Product      | exposes data as        | Data Product Port (output) |
| Data Product      | consumes data through  | Data Product Port (input)  |
| Data Product Port | groups                 | Table / Data Set / Column  |
| Data Contract     | governs functioning of | Data Product Port          |

## When this skill does not apply

- The Data Product already exists and the user wants to add/replace its ports → `edit_asset`
  directly with `add_relation` / `remove_relation` (see `collibra/asset-edit`).
- The user wants to push a Data Contract, not register a Data Product →
  `push_data_contract_manifest` (single-tool call, no skill needed).
- No source table identified yet, just an open question about what data exists →
  `collibra/discovery`.
