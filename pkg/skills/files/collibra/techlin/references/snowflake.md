# Technical Lineage for Snowflake (`edgeharvester-snowflake`)

Harvests lineage from Snowflake — view/procedure SQL or `ACCESS_HISTORY` — and loads it
into the Collibra lineage graph, stitched to the assets ingested by jdbc-ingestion.

- **Capability typeId** (for `edge_create_capability`): **`edgeharvester-snowflake`** —
  the manifest displays **"Technical Lineage for Snowflake"**. Confirm the live id with
  `edge_list_capability_types(query: "snowflake")` — the query also matches other
  Snowflake-named types on the site (e.g. the ETL-owned `snowflake-synchronization`,
  Collibra Protect); only the technical-lineage one is correct. The SQL-files /
  cloud-storage lineage variants (manifests asking for file or bucket locations) are
  not supported via chip.
- **Connection:** the Generic JDBC connection (`typeId: "Generic"`) created for
  jdbc-ingestion — resolve it (and the edge site for the capability) from the Database
  asset with `get_registered_database`. Credentials, role, and warehouse live in the
  connection — the capability never carries them.
- **Trigger:** `start_technical_lineage(assetId)` with the **Database asset** UUID.

**Docs:** [Technical lineage for Snowflake](https://productresources.collibra.com/docs/collibra/latest/Content/CollibraDataLineage/DataSources/Snowflake/to_snowflake.htm)

## Snowflake access requirements

The connection's role needs access to `SNOWFLAKE.ACCOUNT_USAGE` views (`TABLES`,
`COLUMNS`, `VIEWS`, `PROCEDURES`, `OBJECT_DEPENDENCIES`, `ACCESS_HISTORY`,
`QUERY_HISTORY`, …) — typically granted with `IMPORTED PRIVILEGES` on the shared
`SNOWFLAKE` database. Without it the harvest queries fail even though `test_connection`
succeeds.

## The conversation — ask in this order

This is the question script for the parameter checkpoint (step 6 of the parent
`SKILL.md` sequence). Every question is asked; nothing is filled in silently:

1. **`snowflakeMode`** — "Should lineage come from parsing view/procedure SQL
   (`SQL`), or from what actually ran, via `ACCESS_HISTORY` (`SQL-API`)?" The mode
   decides which of the remaining questions apply — the gating is documented here,
   in this reference; the manifest does not encode it.
2. **Scope** — **`databaseNames` (required in BOTH modes — see below)** and
   `schemaNames` (optional). In `SQL-API` mode additionally: `snowflakeDays`
   (default 365) and `extraDatabaseDefinitions`.
3. **`id` (Source ID)** — propose a value, verify uniqueness (see below).
4. **`collibraSystemName`** — must match the system jdbc-ingestion registered the
   database's assets under.
5. **Queries** — ask only whether the user wants to *override* any harvest query
   (almost nobody does; defaults apply automatically — see "Queries").
6. **Techlin server (Custom Properties)** — ask whether the user wants to configure
   the Collibra Data Lineage (techlin) server for this capability. If yes: collect
   **`techlinHost` only** — never the key. `techlinKey` is a secret; the user adds
   it after creation, outside this conversation (see "Custom Properties" below).
7. **Advanced Properties** — only when the user asks for them.
8. **Present the complete parameter set in one table and get an explicit "create
   it"** before calling `edge_create_capability`.

## Capability parameters — Main Properties

**Ask `snowflakeMode` first** — it decides which of the remaining questions apply.
Requiredness below is the harvest-runtime truth and comes from this reference; the
manifest's `required` flags describe the Edge UI form and disagree with it in both
directions (see Gotchas). The live manifest stays authoritative for exact parameter
names and options on the installed capability version.

- `snowflakeMode` (required, **first question**) — how lineage is derived:
  - **`SQL`** — parses the SQL of views and stored procedures. Gives lineage for
    defined objects.
  - **`SQL-API`** — reads `ACCESS_HISTORY` + `OBJECT_DEPENDENCIES`, i.e. lineage from
    what actually ran, including ad-hoc DML. Supports `snowflakeDays` and
    `extraDatabaseDefinitions`.

Then the rest:

- `id` (required) — **Source ID**: a unique identifier for this lineage source. It keys
  the source on the lineage server: re-running with the same `id` replaces that source's
  lineage; other sources can reference it via `dependentSourceIds`. Pick a stable,
  descriptive value (e.g. `snowflake-prod`) and **verify it is unique** — it must not
  collide with any other lineage source's `id` (check existing lineage capabilities'
  `id` parameter via `edge_list_capabilities`), nor with the `collibraSystemName`, nor
  with a database or schema name. A collision overwrites or cross-links another
  source's lineage.
- `connection` (required) — the Generic JDBC connection id (from
  `get_registered_database`).
- `collibraSystemName` (required) — the System asset name used to stitch harvested
  lineage to catalog assets. Must match the system under which jdbc-ingestion registered
  the database's assets.
- `databaseNames` (list; **required in BOTH modes**, even though the manifest marks it
  optional) — the databases to harvest. The harvester fails with "No database names
  provided" when it is empty, and Edge accepts the capability without it — the mistake
  only surfaces later, in the failed harvest job. Always collect the names from the
  user directly; leave the manifest's `databaseNamesJson` file-upload variant unset
  (there are no files in this flow).
- `schemaNames` (list, optional) — restrict harvesting to these schemas.
- `snowflakeDays` (default `365`; `SQL-API` mode) — how many days of `ACCESS_HISTORY`
  to read. Lower it for faster harvests on busy accounts.
- `extraDatabaseDefinitions` (list, `SQL-API` only) — databases to load metadata for
  without harvesting lineage from them (targets of cross-database references).

## Queries — defaults are automatic; only overrides are a conversation

Each query parameter is an editable SQL statement with a built-in default. **Never
collect query SQL from the user unprompted, and never copy the manifest's default SQL
into `parameters`.** "Keep the default" is expressed one of two ways:

- **omit the parameter** — Edge materializes the manifest default at save time (the
  create response echoes the filled queries back; that is expected, not an error), or
- pass the literal string **`"use-default"`** — the sentinel the Edge UI stores; the
  harvester substitutes its built-in query at run time.

Only when the user explicitly wants to customize a harvest query, offer the chosen
mode's query types and set that one parameter to the user's SQL:

- **`SQL` mode:** `columns`, `views`, `procedures`; `external_stages` (optional).
- **`SQL-API` mode:** `access_history` (the core lineage query of this mode),
  `object_dependencies`, `columns_joined`.
- **Either mode (optional):** `dynamic_tables`, `semantic_views`.

Placeholders like `##DBNAME##`, `##DBNAMES##`, `##SCHEMANAMES##`, `##DAYS##` are
substituted at run time and must be preserved in any override.

## Custom Properties — the techlin server; the key never goes through the chat

The Custom Properties group is a **single parameter, `customParameters`** — a list of
key/value objects the harvester reads at run time. `techlinHost` and `techlinKey` are
**not top-level parameters**: set at the top level of `parameters` they are silently
ignored.

- `techlinHost` — host URL of the Collibra Data Lineage (techlin) server the harvested
  batches are uploaded to.
- `techlinKey` — the API key for that server. **A secret: never collect, store, or
  echo its value in the conversation** — a value passed through
  `edge_create_capability` is stored plaintext and readable back through the API.

The values are tenant-specific (the lineage server assigned to the customer's
environment) and optional — always ask explicitly whether the user wants to configure
them; declining is the **user's** decision, never a silent default. When the user
wants them, the flow is split:

1. Ask for **`techlinHost` only** and include it in the capability:

   ```json
   {"customParameters": [
     {"name": "techlinHost", "value": "https://techlin-<env>.example.com",
      "type": "string", "secret": false, "encrypted": false, "fromVault": false}
   ]}
   ```

2. Create the capability with the confirmed parameter set — **without `techlinKey`**.
3. Tell the user to add `techlinKey` themselves in the Collibra/Edge UI — the
   capability's **Custom Properties** group → add property `techlinKey`, **marked
   secret** so it is masked and encrypted — and to say when it is done.
4. **Trigger the harvest only after the user confirms the key is in place.**

`techlinHost` alone can also be added or changed later via the upsert (see "Updating
an existing capability" in the parent `SKILL.md`); `techlinKey` always goes in
through the Edge UI.

## Advanced Properties

- `processingLevel` — `Load` (harvest only, raw metadata inspectable), `Analyze`
  (harvest + analyze), `Sync` (default: also synchronize lineage into the catalog).
- `dependentSourceIds` — Source IDs of other lineage sources this one references, so
  cross-source lineage resolves.
- `databaseSystemMapping` — map database names to system names when one capability
  covers databases that belong to different systems.
- `deleteRawMetadataAfterProcessing` — remove harvested raw metadata after processing.
- `active` — set to `false` and re-run to **remove** this source's lineage from the
  lineage server. Edge fills `active: true` at creation; there is nothing to ask.
- `techlinAdminConnection` (beta) — optional connection to the lineage (TechLin) server
  admin API. Leave unset unless the user asks for it.

## Gotchas

- **Stitching needs ingested assets.** Lineage attaches to the Schema/Table/Column
  assets created by jdbc-ingestion under `collibraSystemName`; if ingestion never ran
  (or ran under a different system name), the harvest succeeds but lineage doesn't link
  to catalog assets.
- **The trigger is asset-keyed.** `start_technical_lineage` takes the Database asset
  UUID; DGC finds the matching Technical Lineage capability itself. Passing the
  capability id fails.
- **Edge fills required defaults at save.** The `edge_create_capability` response
  comes back with the required query parameters and `active: "true"` materialized —
  expected behavior, not something to re-collect or re-send.
- **One capability per (type, connection).** `edge_create_capability` fails with
  `400 — "Connection(s) […] already used in the capability of the same type"` when a
  Technical Lineage capability already exists for that connection. Treat it as
  "already exists": go back to `edge_find_capabilities` and the update or trigger
  path — never retry with a different connection.
- **Requiredness comes from this reference, not the manifest.** The manifest marks
  `databaseNames` optional (fatal at runtime when missing) and marks both modes'
  query parameters required (harmless — Edge auto-fills them). If the live manifest
  lists a parameter this reference doesn't mention, surface it to the user instead of
  guessing.