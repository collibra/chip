# Technical Lineage for Databricks Unity Catalog (`databricks-lineage`)

Extracts data lineage from the Databricks Unity Catalog lineage system tables and produces column-level technical lineage in DGC, stitched to existing Databricks assets.
`capability-type` (the `typeId` for `edge_create_capability`): **`databricks-lineage`**.

**Docs:** [Add the technical lineage for Databricks Unity Catalog capability](https://productresources.collibra.com/docs/collibra/latest/Content/CollibraDataLineage/DataSources/DatabricksUnityCatalog/ta_databricks-add-capability.htm)

**Connection type:** Databricks (requires `workspaceUrl`). Auth methods (set via `authType` on `edge_create_connection`):
- **Personal Access Token** — `accessToken`.
- **OAuth** — `clientId` + `clientSecret`.
- **Microsoft Entra ID** — `clientId` + `clientSecret` + `tenantId`.

**Workflows:** inbound is `main` (default — omit `workflow`). No outbound flow; instead, running with the `active` capability parameter set to `false` for a `sourceId` removes that source's lineage from the Techlin server.

## Capability parameters (set on `edge_create_capability`)
- `httpPath` (required) — HTTP path of the SQL warehouse / compute resource (must start with `sql`).
- `sourceId` (required) — unique lineage source id; also the key for the exclusion flow.
- `processingLevel` — `Analyze` (load + analyze) or `Sync` (also synchronize to DGC).
- `timeFrame` — lookback window in days (default 365).
- **`customParameters` (mandatory)** — a user-defined list that must include the Techlin server coordinates:
  - `techlinHost` — the Techlin/Data Lineage server host URL for this environment (e.g. `https://techlin-<env>.cp.collibra-ops.com`).
  - `techlinKey` — the Techlin API key.
## Notable generic-config choices (fetch exact schema via `catalog_etl_get_schema`)
- `system` (required) — target System asset the ingested objects are stitched to.
- `includeFilters` / `excludeFilters` — scope by `Workspace > Catalog > Schema` (wildcards `*`/`?`; exclude wins).
- `catalog` — read lineage from views in a custom catalog instead of the `system` catalog.
- `sqlSourcesLimit` / `ingestSqlSources` — include SQL transformation texts, capped per relation.
- `ingestExternalLocations`, `ingestVolumes`, `ingestEntities` — extend coverage to external locations, volumes, and notebook/entity transformations.

## Gotchas
- This capability does **not** create the Databricks assets — they must already exist in DGC (ingested by the separate `databricks-edge-capability`) for stitching to work; stitching matches on the exact full path `(System >) Database > Schema > Table > Column`.
- Set `useCollibrasystemName` when the same paths exist across environments (e.g. PROD/DEV), otherwise stitching is non-deterministic; changing it requires re-analyzing all sources.
- Requires read permissions on the Unity Catalog lineage system tables (`system.access.column_lineage`, `system.access.table_lineage`, `system.query.history`) plus compute attach rights.
- Lineage retention is fixed at 365 days by Databricks; large datasets rely on the default `scalable` query strategy (tune via `jvm-args`).
- **`techlinHost`/`techlinKey` custom parameters are mandatory** — set them before the first run. See the `customParameters` entry under "Capability parameters".
