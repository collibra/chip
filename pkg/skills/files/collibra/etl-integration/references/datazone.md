# AWS DataZone / SageMaker Unified Studio (`datazone-synchronization`)

Extracts Redshift or Glue table/view metadata published in AWS SageMaker Unified Studio and ingests it into DGC as a System → Database → Schema → Table/View → Column hierarchy. The `typeId` for `edge_create_capability` is **`datazone-synchronization`**.

**Docs:** [About the Amazon SageMaker Unified Studio integration](https://productresources.collibra.com/docs/collibra/latest/Content/Catalog/AwsSagemakeUnifiedStudioDataZone/co_about-aws-sagemaker-unified-studio.htm)

**Connection type:** AWS (generic family). Auth methods (set on `edge_create_connection`):
- **IAM** — programmatic user credentials (`aws_access_key_id` + `aws_secret_access_key`). This is the only supported method.

**Workflows:** inbound is `main` (default — omit `workflow`). Also has an outbound flow (DGC → SageMaker Unified Studio).

## Ingestion modes (selected in the generic config, NOT via the workflow arg)
- **datasource**: `Amazon Redshift` (default) or `Amazon Glue`. Controls which asset type is pulled from the SageMaker Search API and determines the `DataSourceType` attribute on ingested Database assets.

## Notable generic-config choices (fetch exact schema via `catalog_etl_get_schema`)
- `systemDomain` (required) — domain where System assets are created.
- `community` (required) — community under which per-database and per-schema sub-domains are auto-created.
- `datasource` (required) — `Amazon Redshift` or `Amazon Glue`.
- `regions` (required, array) — one or more AWS regions to scan; at least one must be provided.
- `datazoneDomains` (optional, array) — SageMaker Unified Studio domain IDs to include; empty means all domains visible to the connection.
- `jdbcConnectionsMapping` (optional, Redshift only) — array of `{ databaseName, connectionId }` pairs that link a Collibra Database full name to a JDBC connection, enabling profiling, classification, and sampling on that database.

## Gotchas
- The IAM user must have `datazone:ListDomains`, `datazone:ListProjects`, and `datazone:SearchListings` permissions in every region you scan.
- `jdbcConnectionsMapping` is only relevant for Redshift. Glue tables are S3-backed; JDBC profiling is not supported in v1, so the field is hidden in the UI when `Amazon Glue` is selected.
- For Glue, all Glue databases within a catalog are mapped as schemas under a single synthetic `AwsDataCatalog` Database — there is no real per-catalog database concept in DataZone listings.
- The capability-level parameter `jdbc-capabilities-auto-create-mode` (set on `edge_create_capability`, not in the generic config) controls whether JDBC ingestion, profiling, and classification capabilities are created automatically when the integration runs. Defaults to auto-creating ingestion + profiling + classification.
