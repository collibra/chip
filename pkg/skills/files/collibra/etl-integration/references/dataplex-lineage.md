# Technical Lineage for GCP (`dataplex-lineage-synchronization`)

Extracts technical lineage from Google Dataplex (Data Lineage API) plus BigQuery / GCS and pushes it to Collibra Data Lineage (Techlin), producing table- and column-level technical lineage in DGP. This is a lineage capability — it does **not** ingest table/column assets; those must already exist in DGP for stitching to work.
`capability-type` (the `typeId` for `edge_create_capability`): **`dataplex-lineage-synchronization`**.

**Docs:** [Synchronize technical lineage for Google Dataplex](https://productresources.collibra.com/docs/collibra/latest/Content/CollibraDataLineage/DataSources/GoogleDataplex/ta_dataplex-sync-techlin.htm)

**Connection type:** GCP. Auth methods (set on `edge_create_connection`):
- **Service Account** — full JSON key (`gcp_service_account_credentials_json`).
- **Workload Identity Federation (WIF)** — federated identity; JSON only for file-based WIF.
- **WIF using GKE** — when the Edge site runs in GKE; uses the site's federated identity.

**Workflows:** inbound only (default — omit `workflow`). No outbound flow. Setting the capability param `active=false` for a `sourceId` triggers removal of that source's lineage from Techlin instead of generating any.

## Ingestion modes (selected in the generic config, NOT via the workflow arg)
- **Table lineage** (`lineageType` default) — table-level lineage; supports `includeFilters` / `excludeFilters`.
- **Column lineage** — column-level lineage; requires `gcsBucket` and the (preview) Export Metadata API. Note: stitching is not supported at table level, only column level.

## Notable generic-config choices (fetch exact schema via `catalog_etl_get_schema`)
- `system` (required) — target System asset the ingested objects are stitched to.
- `projects` — GCP project IDs where Dataplex lineage is enabled (required for WIF; else derived from the SA key).
- `locations` — GCP regions to harvest from; defaults to the supported Dataplex regions.
- `lineageType` — Table vs Column lineage (see modes above).
- `gcsBucket` — required for Column lineage; path to the GCS bucket holding exported lineage.
- `includeFilters` / `excludeFilters` — Table-lineage only; patterns `project > location > @bigquery > dataset > table` with `*`/`?` wildcards; exclude wins over include.
- `sqlExtractionMethod` — `api` (default, one Get Job call per process) vs `query` (batched `INFORMATION_SCHEMA.JOBS_BY_PROJECT`).

## Gotchas
- Stitching matches on the **exact full path** `(System >) Database > Schema > Table > Column`; assets must be pre-ingested (e.g. via BigQuery JDBC) or nothing stitches. GCS-file stitching is not supported.
- Enable `useCollibrasystemName` (capability param) when identical paths exist across environments (PROD/DEV) — otherwise stitching is non-deterministic. Changing it requires re-analyzing all sources.
- Dataplex retains lineage for only 30 days; older lineage is permanently gone, so runs must be scheduled recurringly.
- Column lineage depends on the preview `exportMetadata` API, which must be enabled by Google on request.
