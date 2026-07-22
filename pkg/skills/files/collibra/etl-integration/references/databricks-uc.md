# Databricks Unity Catalog (`databricks-edge-capability`)

Extracts metadata from Databricks Unity Catalog (catalogs, schemas, tables, columns, volumes, AI models/deployments/endpoints) and ingests it into DGC. The `typeId` for `edge_create_capability` is **`databricks-edge-capability`**.

**Docs:** [About the Databricks Unity Catalog integration via Edge](https://productresources.collibra.com/docs/collibra/latest/Content/Catalog/Databricks/co_about-databricks-integration.htm)

**Connection type:** Databricks. Auth methods (set on `edge_create_connection`):
- **Personal Access Token** — `accessToken` field.
- **OAuth** — `clientId` + `clientSecret`.
- **Microsoft Entra ID** — `clientId` + `clientSecret` + `tenantId`.

**Workflows:**
- `main` (default) — combined inbound; flow type is chosen by `ingestionType` in the generic config.
- `ai` — dedicated AI asset inbound (models, deployments, endpoints, agents, monitors; DGC 2026.03+).
- `ai-monitoring` — reads MLflow trace metrics via JDBC and POSTs them to the Collibra AI Governance API; does **not** create or modify DGC assets.
- `outbound` — writes DGC attribute changes back to Databricks Unity Catalog as tags.

## Ingestion modes (selected in the generic config, NOT via the workflow arg)

Applies when using the `main` (default) workflow via the `ingestionType` config field:

- **Metadata ingestion** — standard catalog/schema/table/column structure, JDBC-compatible with other ingestion capabilities. Requires `system`.
- **AI model ingestion** (Deprecated) — ingests AI base models, versions, deployments, endpoints, agents, monitors. Requires `domain`.
- **Metadata and AI model ingestion** (Deprecated) — combined; requires both `system` and `domain`.

When using the dedicated `ai` workflow, `ingestionType` is not used — the workflow name itself determines the flow.

## Notable generic-config choices (fetch exact schema via `catalog_etl_get_schema`)

- `system` — required for metadata flows; the System asset in DGC that becomes the root for ingested catalog/schema/table assets.
- `domain` — required for AI model flows; the DGC domain where AI assets land.
- `domainIncludeMappings` / `domainExcludeMappings` — scope ingestion to specific catalogs, schemas, or tables using `catalog > schema > table` patterns (wildcards `*` and `?` supported). Empty = ingest everything the connection can see.
- `version` — domain mapping resolution version. `V0` (default): unmatched schemas inherit the catalog domain. `V1`: unmatched schemas are excluded. New configurations should use `V1`.
- `ingestVolumes` — when enabled, also ingests Databricks Volume assets (DatabricksVolume, Directory, File) for each schema. Requires the Databricks Files API to be enabled on the workspace.
- `tagAttributeMappings` — routes Unity Catalog tag values to specific DGC attributes in addition to (or instead of) the default `Source Tags` attribute.
- `extensiblePropertiesMapping` — maps Databricks system properties (e.g. `table_type`, `created_at`, `metastore_id`) and custom table parameters to DGC attributes.
- `ingestInputDatasetsAndDeploymentOutput` (AI flow) — enables ingestion of MLflow training datasets and inference table output linked to model versions/deployments.

## Gotchas

- **JDBC connection is required for tag fetching.** A separate `jdbc-connection` capability parameter must reference a JDBC-type Edge connection. Without it, Unity Catalog tags are not fetched and `Source Tags` will be empty. The same JDBC connection also enables Profiling and Classification.
- **Domain cannot be changed for existing assets.** Once a catalog (Database) or schema asset has been ingested, its domain is locked — the engine looks up existing assets by name and reuses their domain regardless of the current mapping. Get domain mappings right before the first run.
- **Do not configure AI ingestion in both tabs.** Providing AI parameters in both the "Metadata Inbound" tab and the dedicated "AI ingestion" tab causes synchronization to fail on DGC < 2026.04.
- **Governed tags in Databricks only accept predefined values.** The outbound flow silently skips tag writes whose value is not permitted by the tag policy; the DGC attribute is updated but the Databricks tag stays unchanged. Check outbound logs for errors when mapping governed tags.
- **Volumes ingestion requires a Databricks beta flag.** The Files API (`enable_experimental_files_api_client = True`) must be enabled on the workspace before `ingestVolumes` will work.
- **`ai-monitoring` produces no DGC catalog assets.** It writes only to the AI Governance API (`/rest/aiGovernance/v1/metrics/bulk`). There is no synchronization report entry; success/failure is reported as an Edge job status only.
- **`stopComputeResource` shuts down the compute cluster after tag extraction.** Only relevant when `httpPath` is set. Enable with caution in shared-cluster environments.
