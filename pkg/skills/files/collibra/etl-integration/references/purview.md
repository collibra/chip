# Microsoft Purview (`purview-synchronization`)

Extracts schema metadata from Azure SQL Server databases cataloged in Microsoft Purview (servers,
databases, schemas, tables, and columns) and ingests it into DGC. `capability-type` (the `typeId`
for `edge_create_capability`): **`purview-synchronization`**.

**Docs:** No standalone Collibra product page documents this `purview-synchronization` (Azure SQL Server via Purview) capability. In the public docs, "Purview" appears only as a *synchronization source within the ADLS integration* — a different capability — so do not treat the ADLS docs as authoritative for this integration.

**Connection type:** Azure. Auth method (set on `edge_create_connection`):
- **Service Principal** — `clientId`, `clientSecret`, and `tenantId` from an Azure AD app
  registration.

**Workflows:** single inbound workflow `main` (default — omit `workflow`). No outbound flow.

## Notable generic-config choices (fetch exact schema via `catalog_etl_get_schema`)
- `dataSource` (required) — currently only `MSSQL` (Azure SQL Server); defaults to `MSSQL`.
- `system` (required) — target System asset in DGC to which databases are linked.
- `community` (required) — default Collibra community; sub-domains for databases and schemas are
  auto-created here when no explicit domain mapping matches.
- `domainIncludeMappings` (optional) — array of `{from, to}` objects. `from` is a
  `Server > Database > Schema` glob path (supports `*` and `?` wildcards); `to` is the target
  domain. Omitting `to` auto-creates domains inside the default community.
- `domainExcludeMappings` (optional) — array of strings in `Server > Database > Schema` format;
  matched paths are skipped even if they match an include rule.

## Gotchas
- This integration reads from Purview's catalog — it does not scan Azure SQL directly. The Azure
  SQL data sources must have already been registered and scanned inside Purview before running the
  capability, otherwise no assets will be found.
- The service principal used in the Azure connection needs **Data Reader** and **Data Source Admin**
  roles assigned in the Purview account (set in Purview's data-map collections permissions, not in
  Azure IAM alone).
- The `purview-account` capability parameter (the short Purview account name, not a URL) is set at
  capability creation time and is separate from the generic config.
- Domain mapping paths must be a triplet `Server > Database > Schema`. Paths with fewer or more
  levels fail validation at run time with a descriptive error.
- Exclude mappings take priority over include mappings.
