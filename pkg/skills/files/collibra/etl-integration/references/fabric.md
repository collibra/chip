# Microsoft Fabric (`fabric-synchronization`)

Extracts metadata from Microsoft Fabric — traversing Tenant → Capacity → Workspace →
Lakehouses (schemas, tables, columns, files), Warehouses (schemas, tables, views, columns),
SQL Databases, and Mirrored Azure Databricks Catalogs — and ingests it into DGC.
`capability-type` (the `typeId` for `edge_create_capability`): **`fabric-synchronization`**.

**Docs:** [About the Microsoft Fabric integration](https://productresources.collibra.com/docs/collibra/latest/Content/Catalog/MicrosoftFabric/co_about-fabric.htm)

**Connection type:** Azure. Auth method (set on `edge_create_connection`):
- **Service Principal** — `client-id`, `client-secret`, and `tenant-id` of an Azure Entra
  App registration (OAuth 2.0 client credentials flow).

**Workflows:** inbound is `main` (default — omit `workflow`). No outbound flow.

## Notable generic-config choices (fetch exact schema via `catalog_etl_get_schema`)
- `community` (required) — Collibra community where per-workspace Technology Assets
  subdomains are auto-created.
- `mainDomain` (required) — domain where `MicrosoftTenant` and `FabricCapacity` assets land.
- `workspaceNames` (optional) — limit ingestion to specific workspace names; empty = all
  accessible to the service principal.
- `maxLakehouseFiles` (optional, default `100`) — cap on files ingested per Lakehouse; set
  to `-1` to ingest all files.
- `jdbcConnectionsMapping` (optional) — pairs `databaseName` (full Collibra asset name, e.g.
  `Tenant>Capacity>Workspace>my-db`) with a `connectionId` (JDBC connection) to unlock
  schema/table/column ingestion, profiling, classification, and sampling for SQL Databases.
  Without this, the capability creates only the top-level `Database` asset with no internal
  structure.
- `domainIncludeMappings` / `domainExcludeMappings` — route or exclude assets by a
  `>`-separated hierarchical path (1–4 segments: Workspace → Artifact → Schema → Table;
  wildcards `*` and `?` supported within each segment). Excludes always win over includes.
- `tenantNameOverride` / `capacityNameOverrides` — static name overrides; once set, the
  capability stops updating those names from the Microsoft API.

## Gotchas
- **SQL Database internal structure is not ingested by this capability.** Only the top-level
  `Database` asset is created. Configure a `jdbcConnectionsMapping` entry so the
  auto-created JDBC Ingestion capability handles schemas, tables, and columns.
- **Warehouse metadata requires `ReadData` permission**, not just Workspace Viewer. The Viewer
  role grants only `CONNECT`; `ReadData` (*Read all data using SQL*) must be explicitly granted
  in the Fabric portal to allow JDBC `INFORMATION_SCHEMA` queries.
- **Lakehouse table and file access is governed by OneLake security roles**, separate from the
  workspace Viewer role. Grant the Service Principal **Read on Tables** (schema/column
  discovery via OneLake Table API) and **Read on Files** (file listing via OneLake DFS API) in
  the Lakehouse's OneLake security settings — or grant item-level `ReadAll` to bypass role
  checks entirely.
- **Mirrored Azure Databricks Catalog tables require two Databricks-side controls**: (1)
  `EXTERNAL USE SCHEMA` privilege on each schema in Unity Catalog, and (2) **External data
  access enabled on the metastore** (a metastore-level toggle, off by default, requiring
  metastore/account admin). If either control is missing, table listing silently returns HTTP
  200 with no tables and column fetches return 403 — ingestion appears to succeed but produces
  empty schemas.
- **No admin-consent Power BI/Fabric application permissions** may be registered in Entra for
  the Service Principal, even unused ones. Their mere presence breaks service principal
  authentication against the Power BI Admin API (used for capacity discovery).
- When both `workspaceNames` and `domainIncludeMappings` are configured, a workspace must
  pass **both** filters to be ingested.
