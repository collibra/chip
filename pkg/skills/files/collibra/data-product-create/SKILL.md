---
description: Create a Collibra Data Product from a source table (name, UUID, or URL) — discover dimension tables, reuse legacy DataSet context, block near-duplicates, then register the product, its ports, and a Data Contract.
related: collibra/asset-create, collibra/asset-edit, collibra/discovery
---

# Creating a Data Product from a table

Turns a single source table — identified by name, DGC UUID, or Collibra URL — into a
Collibra Data Product with one or two Ports, the right `groups` / `exposes data as`
relations, and a Data Contract governing the physical Port. Phases 1–5 are read-only
discovery, overlap checks, and a proposal; phase 6 is the writes, only after the user
signs off. Tool-call mechanics (argument resolution, status branches, RICH_TEXT, operation
shapes) live in `collibra/asset-create` and `collibra/asset-edit` — this skill is the
DP-specific layer on top.

## Hard rules

1. **Write order is fixed.** All assets created before any relations; every relation source
   must already have a UUID. The Data Contract is pushed last, after its Port exists. See
   Phase 6 for the exact sequence.
2. **Conservative auto-inclusion.** Only HIGH-confidence dimension tables join automatically.
   MEDIUM candidates require user confirmation — false positives in the join graph are worse
   than asking one extra question.
3. **Join predicates follow the schema's own conventions.** The default join is
   `fact.<fk> = dim.id`. But first check whether the schema is multi-tenant or
   snapshot-partitioned: tenancy/partition columns (e.g. `tenant_id`, `instance_name`,
   `date_partition`, `snapshot_date`) appearing in nearly every table. If so, every join
   must also equate those columns (`… AND fact.<tenant col> = dim.<tenant col> AND
   fact.<partition col> = dim.<partition col>`) — a bare `fk = id` join in such a schema
   fans out across tenants and snapshots and silently returns wrong rows.
4. **Infrastructure columns are never join keys.** Identify audit, tenancy, and partition
   columns — ones that appear in nearly every table, with names like `tenant_id`,
   `instance_name`, `date_partition`, `creator`, `modifiedby`, `lastmodified`,
   `creationdate` — and strip them before pattern-matching column names for FKs. (They may
   still appear in join predicates per rule 3.)
5. **Carry forward acceptable-use warnings.** If the source table has confidentiality or
   policy attributes, restate them in the Data Product `Description`.
6. **Existing coverage is a stop condition.** If Phase 1 or Phase 4 finds an existing Data
   Product Port or Data Contract already covering substantially the same tables, do not
   create anything — report the existing assets (names + UUIDs) and offer to extend the
   existing product instead. Resume creation only if the user explicitly says to create a
   near-duplicate anyway.
7. **DataSet content is copied only on purpose match plus user confirmation.** A legacy
   DataSet overlapping the proposed tables is a *candidate* source of descriptions, not an
   automatic one. Copy its content into the Data Product only when its purpose matches the
   proposed product's purpose and the user has confirmed the copy in the Phase 5 proposal.

## Phase 1 — Locate the source table

The input is a table **name**, a DGC asset **UUID**, or a Collibra **URL**. Resolve it to a
UUID first:

- **UUID** (8-4-4-4-12 hex) → use it directly; skip search.
- **URL** → extract the UUID from the path — Collibra asset URLs embed it, e.g.
  `https://<instance>/asset/<uuid>` (anything after `/asset/` up to the next `/` or `?`).
  Never pass a URL to `search_asset_keyword`.
- **Name** → `search_asset_keyword` with `query=<user's term>`,
  `resourceTypeFilters=["Asset"]`, `limit=20`. If more than one result, list numbered and
  let the user pick.

Then:

1. `get_asset_details` on the resolved UUID. If the asset is not table-like (Table,
   Database View, or a subtype — e.g. the URL pointed at a Column or a Domain), tell the
   user what it is and stop — do not guess a parent table.
2. Read off: name, id, parent domain; schema (the
   `incomingRelations` source with `type.name == "Schema"`); columns (`outgoingRelations`
   targets with `type.name == "Column"`); existing description / sensitivity / refresh /
   acceptable-use attributes.
3. **Early duplicate exit.** If the `incomingRelations` already include a `groups` relation
   from a `Data Product Port` source, the table is already part of a Data Product. Before
   running Phases 2–3, resolve that Port's product and contract (Phase 4 step 1) and report
   per Phase 4 step 2. One existing product is a strong duplicate signal, not proof — a
   table can legitimately feed multiple products — so report and ask; do not presume.

## Phase 2 — Find dimension tables

1. **Enumerate siblings.** `search_asset_keyword` with `query=<schema name>` and `limit=100`.
   Keep only results whose `name` starts with the full schema path
   (e.g. `<system>><database>><schema>>`).
2. **Pre-filter by name before fetching.** Using the fact table's columns (already known
   from Phase 1), keep only siblings whose name matches a fact column pattern (`xxx_id` /
   `xxx_type` ↔ table named like `xxx`) or looks like a lookup/enum table. If more than
   ~20 plausible candidates remain, fetch the best 20 and tell the user what was skipped —
   do not silently fetch a hundred tables.
3. **Fetch each remaining candidate** with `get_asset_details` and read its columns.
4. **Classify** after stripping infrastructure columns (rule 4):

   **HIGH — auto-include:**
   - Fact column `xxx_id` and a sibling table named like `xxx` (singular/plural fine) with
     an `id` column. Example: `customer_id` ↔ table `customers` (column `id`).
   - Same pattern for `xxx_type` ↔ enum/type tables.

   **MEDIUM — ask the user:**
   - Shared non-infrastructure column names between fact and dimension (e.g. `role`,
     `vocabulary`, `assigned_type`) without a clean FK pattern.
   - Sibling tables that look like status/classification lookups but lack an `<x>_id` link.

5. Present HIGH (auto-included, with the detected FK) and MEDIUM (with reasoning); wait for
   the user to confirm which MEDIUM tables to include.

## Phase 3 — Detect the semantic layer

1. `get_table_semantics` on the source table and each selected dimension.
2. Walk `semanticHierarchy[*].connectedDataAttributes` in each result.
   - Any non-empty → **semantic layer exists** → plan **two ports**: physical (SQL access)
     and semantic (`Access Method: "UI"`). Both group the same tables; see Phase 6.
   - All empty → **one port** (physical only).

## Phase 4 — Check overlap with existing Data Products and DataSets

Before Data Products existed, Collibra deployments used **Data Set** assets for the same
job, and customers were instructed to link **columns** to them. A new Data Product must not
duplicate a product or contract that already covers these tables — and should reuse the
legacy DataSet context where it exists. Run the duplicate check first: if it stops the
flow, the DataSet work is moot.

1. **Find existing Data Products / Contracts over the same tables.** In the
   `get_asset_details` responses already fetched in Phases 1–2, look for incoming `groups`
   relations whose source is a `Data Product Port`. For each such Port, resolve its Data
   Product (incoming `exposes data as`) and any Data Contract (incoming
   `governs functioning of`) via `get_asset_details` on the Port. Fallback when relations
   are inconclusive: `list_data_contract`, then `pull_data_contract_manifest` on candidates
   whose contract name resembles the table, schema, or proposed product name, and match the
   manifest's schema `physicalName` entries — and its `servers` database/schema, when
   present — against the proposed tables.
2. **Stop on duplication (rule 6).** If an existing Data Product or Data Contract covers
   substantially the same set of tables, report it — name, UUID, which tables overlap —
   tell the user they are about to create a very similar Data Product to an existing one,
   and offer to extend the existing product instead (`edit_asset` with `add_relation`, see
   `collibra/asset-edit`). Do not proceed to Phases 5–6 unless the user explicitly insists
   on a new product.
3. **Find linked DataSets.** In the same details responses, look for relations (either
   direction, commonly role `groups`) whose counterpart asset's `type.name == "Data Set"`.
   Because the links usually sit on columns, a table with no DataSet relation is not a
   clean negative: call `get_asset_details` on a sample of its non-infrastructure columns
   (3–5, prefer FK and business columns) and check their relations too. Sample only the
   source table and the selected dimensions — not every Phase 2 candidate. Collect the
   distinct DataSet UUIDs.
4. **Compare purpose.** `get_asset_details` on each found DataSet; read its `Description` /
   `Definition` attributes. Judge whether the DataSet serves the same purpose as the
   proposed Data Product — same data *and* same consumer intent, not merely overlapping
   tables.
   - **Purpose matches** → in the Phase 5 proposal, offer to copy the DataSet's
     descriptions into the Data Product attributes, each marked
     "(from Data Set `<name>`)". Copy only after the user confirms (rule 7). Also add a
     provenance line to the Data Product `Description` (e.g. "Context carried over from
     Data Set `<name>`."). **Never create a relation to the DataSet** — the mention in the
     `Description` is the only trace.
   - **Purpose differs** → mention the DataSet in the proposal for awareness; copy nothing.

## Phase 5 — Propose the structure

**Lead with a picture, then the details.** Render the proposed structure graphically
whenever the output medium allows: a Mermaid `graph TD` if the client renders Mermaid,
otherwise an ASCII box diagram in a code block — that works everywhere. Two layers in one
diagram: the asset chain on top (Data Product → Port, contract attached to the Port), the
star schema below the Port's `groups` fan-out — fact table in the middle, each included
dimension joined to it with the short join key on the edge, and warning markers (e.g.
`⚠ PII`) inside the box of any table that carries rule-5 attributes:

```
                 ┌──────────────────────────┐
                 │  DP: <Data Product name> │
                 └────────────┬─────────────┘
                              │ exposes data as
                 ┌────────────▼─────────────┐
                 │  Port: <name> (<method>) │◄─ governs ─ Contract: <name>
                 └────────────┬─────────────┘
                              │ groups
         ┌────────────────────┼────────────────────┐
         ▼                    ▼                    ▼
   ┌───────────┐  <fk = pk> ┌────────┐  <fk = pk> ┌───────────┐
   │ <dim> ⚠PII│◄───────────│ <fact> │◄───────────│ <dim>     │
   └───────────┘            └────────┘            └───────────┘
```

With two ports, draw both under the Data Product but hang the table fan-out off the
physical Port only, with a one-line footnote that the semantic Port groups the same tables
— do not duplicate the star. Edges carry only the short `fk = pk` form; compound predicates
(the multi-tenant template of rule 3) are written out in the details below, not crammed
into the diagram. Second-hop MEDIUM candidates awaiting confirmation may appear in the
diagram dashed or marked `(?)`, or be left to the details — never drawn as if confirmed.

Then the details, terse and skimmable: one block per asset (Data Product, each Port, and
the Data Contract) with its name and key attributes; under each Port, the list of tables to
link, with the join expression for each non-fact table written out in full per rule 3.
Where Phase 4 found a purpose-matching DataSet, show the attribute values to be copied,
each marked "(from Data Set `<name>`)", and ask the user to confirm or drop the copy. End
with a numbered action list that mirrors the write order in rule 1 (assets first, then
relations, then the contract).

Ask "Proceed?" and do not start writing until the user confirms.

## Phase 6 — Create

Use `create_asset` and `edit_asset` per their skills — those cover argument resolution,
status branches, partial-failure semantics, and Markdown handling. This phase only specifies
*what* to write, in this order:

1. **Create assets** with `create_asset`, `domain: "Data Products"`:
   - **Data Product** — `assetType: "Data Product"`. Attributes: `Description`,
     `Business Case`, `Business Value` (RICH_TEXT — write Markdown), `Data Product Category`
     (`"Foundational"` or `"Derived"`), optional `Target Delivery Date`.
   - **Port(s)** — `assetType: "Data Product Port"`. Attributes: `Description` (RICH_TEXT),
     `Access Method`, `Access Instructions` (RICH_TEXT), optional `Area`, `Pricing` (only
     when the user supplied cost or chargeback information — never invent a price),
     `Target Delivery Date`. One Port per output channel (Phase 3 decides if there are
     two): the physical Port's `Access Method` names the warehouse/SQL access (e.g.
     `"Redshift"`, `"JDBC"`); the semantic Port's `Access Method` is `"UI"`. `Access Method`
     is a **selection list** — the value must be one of the instance's allowed values; on
     `validation_error`, pick the closest match from the error's suggestions rather than
     inventing a new value.

2. **Wire relations** with `edit_asset add_relation`. The edited asset is always the head:
   - On the Data Product: one operation per Port, `relationType: "exposes data as"`,
     `targetAssetId: <port UUID>`.
   - On each Port: one operation per linked table, `relationType: "groups"`,
     `targetAssetId: <table UUID>`. Batch all of one Port's `groups` ops into a single
     `edit_asset` call. With two ports, both group the same tables — they differ only in
     `Access Method` / `Access Instructions`, not membership. Never add a `groups`
     relation to a legacy DataSet (Phase 4 — provenance goes in the `Description`).

3. **Push the Data Contract** with `push_data_contract_manifest` (requires
   `dgc.data-contract`). Compose an ODCS YAML manifest covering **every table the physical
   Port groups** — the Phase 1 source table plus the selected Phase 2 dimensions:

   - If the instance already has contracts (`list_data_contract`), pull one with
     `pull_data_contract_manifest` and mirror its `apiVersion` and structure — instances
     can differ in which ODCS version they accept.
   - Otherwise use this skeleton. One `schema` entry per grouped table, one `properties`
     entry per column, infrastructure columns included — the contract describes the
     physical schema. The `id` becomes the manifestId: keep it stable, and prefix it with
     the schema so it stays unique across schemas. The `servers` block comes from the
     Phase 1 schema path (`<system>><database>><schema>>`) — it makes the contract
     self-describing, so fill it; but do not add SLA/service-level sections unless the
     user supplied actual commitments.

     ```yaml
     apiVersion: v3.0.2
     kind: DataContract
     id: <kebab-case schema + product name>
     name: <Data Product name>
     version: 1.0.0
     status: active
     description:
       purpose: <one line — what the product exposes; carry forward rule-5 warnings>
     servers:
       - server: <environment, e.g. production>
         type: <warehouse type, e.g. redshift|snowflake|bigquery|postgres>
         database: <database>
         schema: <schema>
     schema:
       - name: <table name>              # repeat per grouped table
         physicalName: <schema>.<table name>
         properties:
           - name: <column name>
             logicalType: <string|integer|date|...>
     ```

   The response returns the Data Contract **asset UUID** (`id`) and `manifestId`. On
   `success: false`, surface the error verbatim, skip step 4, and still report — do not
   retry blindly.

4. **Link the contract**: one `edit_asset` call on the **Data Contract** asset (it is the
   head) with `add_relation`, `relationType: "governs functioning of"`,
   `targetAssetId: <physical Port UUID>`. With two ports, the contract governs the
   physical/SQL Port — it describes the physical schema, not the semantic layer.

5. **Report** Data Product UUID, Port UUIDs, Data Contract UUID + manifestId, count of
   linked tables. Surface any non-`success` status or per-operation failure verbatim — a
   failed relation or contract step never restarts the asset creates in step 1.

## Naming

- **Data Product** — human-readable, derived from the source table.
  `sales.customer_orders` → `"Sales Customer Orders"`.
- **Port** — `<Data Product name> — <Access Method>`.
  `"Sales Customer Orders — Redshift"`; semantic port: `"Sales Customer Orders — UI"`.

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
| Access Method (selection list)  | Port — value must match instance picklist  |
| Access Instructions (RICH_TEXT) | Port                                       |
| Area                            | Port                                       |
| Pricing                         | Port — only with user-supplied cost info   |

| Head              | Role                   | Tail                       |
|-------------------|------------------------|----------------------------|
| Data Product      | exposes data as        | Data Product Port (output) |
| Data Product      | consumes data through  | Data Product Port (input)  |
| Data Product Port | groups                 | Table / Data Set / Column  |
| Data Contract     | governs functioning of | Data Product Port          |

The metamodel allows a Port to `groups` a Data Set, but this skill never does — legacy
DataSet provenance goes in the `Description` only (Phase 4).

## When this skill does not apply

- The Data Product already exists and the user wants to add/replace its ports → `edit_asset`
  directly with `add_relation` / `remove_relation` (see `collibra/asset-edit`).
- The user wants to push or update a Data Contract manifest for an existing product, with
  no Data Product to register → `push_data_contract_manifest` directly (single-tool call,
  no skill needed).
- No source table identified yet, just an open question about what data exists →
  `collibra/discovery`.
