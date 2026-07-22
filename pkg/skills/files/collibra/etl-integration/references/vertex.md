# Google Vertex AI (`vertex-synchronization`)

Extracts AI/ML model metadata from Google Vertex AI (Model Registry, endpoints,
deployments, monitors, reasoning-engine agents, and referenced Model Garden
foundation models) and ingests it into DGC. `capability-type` (the `typeId` for
`edge_create_capability`): **`vertex-synchronization`**.

**Docs:** [About the Google Vertex AI integration via Edge](https://productresources.collibra.com/docs/collibra/latest/Content/Catalog/IntegrateAIModels/GoogleVertexAI/co_about-vertex-ai.htm)

**Connection type:** GCP. Auth methods (set on `edge_create_connection`):
- **Service Account** — full JSON key (`gcp_service_account_credentials_json`).
- **Workload Identity Federation (WIF)** — federated identity; token/URL for file-based WIF.
- **WIF using GKE** — when the Edge site runs in GKE; uses the site's federated identity, credentials left blank.

**Workflows:** inbound only, `main` (default — omit `workflow`). There is no outbound
(DGC → Vertex) flow.

## Notable generic-config choices (fetch exact schema via `catalog_etl_get_schema`)
- `domain` (required) — target domain for the ingested assets.
- `projects` — GCP Project IDs to scan; if empty, auto-discovers every project the
  service account can access.
- `locations` — regions to search; if empty, searches all locations where Vertex AI is available.
- `ingestAiInputAndOutput` (default true) — also ingest training-data File/File Container
  assets from GCS and BigQuery prediction-logging output tables; when false, only model
  metadata is ingested.
- `systems` — Collibra System assets to search for existing BigQuery output tables before
  creating new ones (dedup); only applies when `ingestAiInputAndOutput` is enabled.
- `customLabelMappings` / `customAiMetricsMappings` — map custom Vertex labels/metrics to
  AI Model attribute types.

## Gotchas
- Vendor foundation models (Gemini, Claude, Llama, Mistral, etc.) are ingested **on demand
  only** when a customer model references one via `baseModelSource.modelGardenSource`; the
  Model Garden catalogue is never proactively scanned.
- If `getPublisherModel` fails (permission/not-found/unavailable), the vendor parent asset
  and its parent-link relation are skipped but the sync continues.
- GCS File assets and BigQuery output tables are only produced when `ingestAiInputAndOutput`
  is enabled — extra permissions (`storage.objects.*`, `bigquery.tables.get`) are then required.
