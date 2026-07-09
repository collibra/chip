# Technical Lineage for Snowflake (`edgeharvester-snowflake`)

Harvests lineage from Snowflake — view/procedure SQL or `ACCESS_HISTORY` — and loads it
into the Collibra lineage graph, stitched to the assets ingested by jdbc-ingestion.

- **Capability typeId** (for `edge_create_capability`): **`edgeharvester-snowflake`** —
  the manifest displays **"Technical Lineage for Snowflake"**. Confirm the live id with
  `edge_list_capability_types(query: "snowflake")`; do not confuse it with other
  Snowflake-named capability types on the site — especially not with the SQL-files /
  cloud-storage lineage variants, whose manifests ask for file or bucket locations
  (that flow is not supported via chip).
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

## Capability parameters — Main Properties

**Ask `snowflakeMode` first** — it decides which of the remaining parameters exist and
which are required, so nothing else should be collected before it:

- `snowflakeMode` (required, **first question**) — how lineage is derived:
  - **`SQL`** — parses the SQL of views and stored procedures. Gives lineage for
    defined objects; requires `databaseNames`.
  - **`SQL-API`** — reads `ACCESS_HISTORY` + `OBJECT_DEPENDENCIES`, i.e. lineage from
    what actually ran, including ad-hoc DML. Supports `extraDatabaseDefinitions` and
    `snowflakeDays`.

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
- `databaseNames` (list; required in `SQL` mode) — the databases to harvest. Collect the
  names from the user directly; leave the manifest's `databaseNamesJson` file-upload
  variant unset (there are no files in this flow).
- `schemaNames` (list, optional) — restrict harvesting to these schemas.
- `snowflakeDays` (default `365`) — how many days of `ACCESS_HISTORY` to read
  (`SQL-API` mode). Lower it for faster harvests on busy accounts.
- `extraDatabaseDefinitions` (list, `SQL-API` only) — databases to load metadata for
  without harvesting lineage from them (targets of cross-database references).

## Queries group

One editable SQL parameter per query type (`columns`, `views`, `procedures`, and in
`SQL-API` mode `object_dependencies`, `access_history`, …), pre-filled with the default
`ACCOUNT_USAGE` queries. Leave the defaults unless the user explicitly needs to override
them (e.g. custom filtering); placeholders like `##DBNAMES##`, `##SCHEMANAMES##`,
`##DAYS##` are substituted at run time.

## Custom Properties — suggest these proactively

The Custom Properties group takes free-form key/value pairs the harvester reads at run
time. **Suggest adding these two** when creating the capability:

- `techlinHost` — host URL of the Collibra Data Lineage (techlin) server the harvested
  batches are uploaded to.
- `techlinKey` — the API key for that server.

The values are tenant-specific (the lineage server assigned to the customer's
environment) — ask the user for them; if they don't have them at hand, the capability
can be created without and updated later (`edge_create_capability` upserts by
`capabilityId`).

## Advanced Properties

- `processingLevel` — `Load` (harvest only, raw metadata inspectable), `Analyze`
  (harvest + analyze), `Sync` (default: also synchronize lineage into the catalog).
- `dependentSourceIds` — Source IDs of other lineage sources this one references, so
  cross-source lineage resolves.
- `databaseSystemMapping` — map database names to system names when one capability
  covers databases that belong to different systems.
- `deleteRawMetadataAfterProcessing` — remove harvested raw metadata after processing.
- `active` — set to `false` and re-run to **remove** this source's lineage from the
  lineage server.
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
- **`SQL` vs `SQL-API` changes the parameter set** (which query params exist, whether
  `databaseNames` is required) — re-read the manifest after the user picks the mode.
- The live manifest from `edge_list_capability_types` is authoritative for exact
  parameter names, requiredness, and defaults on the customer's capability version;
  this reference explains meaning and choice.