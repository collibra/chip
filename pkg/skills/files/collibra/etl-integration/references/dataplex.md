# Dataplex / Google Knowledge Catalog (`dataplex-synchronization`)

Extracts metadata from Google Dataplex / Knowledge Catalog (formerly Dataplex Universal
Catalog) and ingests it into DGC. `capability-type` (the `typeId` for
`edge_create_capability`): **`dataplex-synchronization`**.

**Docs:** [About Google Dataplex ingestion via Edge](https://productresources.collibra.com/docs/collibra/latest/Content/Catalog/GCS/Dataplex/co_about-dataplex.htm)

**Connection type:** GCP. Auth methods (set on `edge_create_connection`):
- **Service Account** — full JSON key (`gcp_service_account_credentials_json`).
- **Workload Identity Federation (WIF)** — federated identity; JSON only for file-based WIF.
- **WIF using GKE** — when the Edge site runs in GKE; uses the site's federated identity.

**Workflows:** inbound is `main` (default — omit `workflow`). Also has an outbound flow
(DGC → Dataplex).

## Ingestion modes (selected in the generic config, NOT via the workflow arg)
- **DataSourceType**: `BigQuery` (default) or `CloudSQL` (MySQL / PostgreSQL / SQL Server).
- For BigQuery, **ingestionType** picks the ETL flow: **Knowledge Catalog** ingestion
  (JDBC-compatible db/schema/table/column structure) or legacy **Dataplex** ingestion
  (older non-JDBC APIs, still supported).

## Notable generic-config choices (fetch exact schema via `catalog_etl_get_schema`)
- `system` (required) — target System asset; `community` required for CloudSQL.
- `projects` / `locations` (optional) — scope ingestion; empty = all the connection can see.
- `domainIncludeMappings` / `domainExcludeMappings` + `domainMappingVersion` (V0 keeps an
  existing asset's domain; V1 lets the mapping override it).
- `aspectMappings` (Knowledge Catalog) / `customLabelMappings` (legacy Dataplex) — map
  source aspects/labels to DGC attributes.
- `columnsIngestionMode` — parent-only / parent+nested / flattened column structures.

## Gotchas
- The BigQuery-vs-CloudSQL and Knowledge-Catalog-vs-legacy choice lives in the **config**
  (`DataSourceType`/`ingestionType`), not the Edge `workflow` argument.
- An optional `jdbc-connection` capability parameter only enables profiling/classification
  of ingested data — it is not the ingestion path itself.
