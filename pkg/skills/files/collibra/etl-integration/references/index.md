# ETL integration catalog

Every integration below is an Edge capability configured through the same lifecycle (see
the parent `SKILL.md`): `edge_create_connection` → `edge_create_capability` →
`catalog_etl_get_schema` → `catalog_etl_save_generic_config` → `catalog_etl_start_job` /
`catalog_etl_add_schedule`.

The **capability typeId** is what you pass to `edge_create_capability` (`typeId`). Always
confirm the exact connection/capability parameters via `edge_list_capability_types` and the
generic-config fields via `catalog_etl_get_schema` — the per-integration notes here are
conceptual, not field-level.

| Integration | capability typeId | Syncs |
|---|---|---|
| Google Knowledge Catalog / Dataplex | `dataplex-synchronization` | GCP Dataplex / Knowledge Catalog metadata (BigQuery, CloudSQL) — see `dataplex.md` |
| Google Vertex AI | `vertex-synchronization` | Vertex AI models/datasets (GCP) |
| GCP technical lineage | `dataplex-lineage-synchronization` | Technical lineage for GCP |
| Databricks Unity Catalog | `databricks-edge-capability` | Databricks Unity Catalog metadata |
| Databricks lineage | `databricks-lineage` | Technical lineage for Databricks Unity Catalog |
| AWS Bedrock | `bedrock-synchronization` | AWS Bedrock AI models |
| AWS SageMaker | `sagemaker-synchronization` | SageMaker AI (models, etc.) |
| AWS DataZone / SageMaker Unified Studio | `datazone-synchronization` | SageMaker Unified Studio data catalog (Preview) |
| Azure ML | `azureml-synchronization` | Azure Machine Learning |
| Azure AI Foundry | `azure-foundry-synchronization` | Azure AI Foundry |
| Microsoft Fabric | `fabric-synchronization` | Microsoft Fabric |
| Microsoft Purview | `purview-synchronization` | Purview catalog metadata |
| MLflow | `mlflow-synchronization` | MLflow AI experiments/models |
| Sigma | `sigma-synchronization` | Sigma dashboards + lineage |
| ThoughtSpot | `thoughtspot-synchronization` | ThoughtSpot objects + lineage (Preview) |

**Not listed here / not controllable via chip:** the object-storage crawlers **S3**
(`s3-synchronization`), **ADLS** (`adls-synchronization`), and **GCS**
(`gcs-synchronization`) use a different endpoint/config model and cannot be configured
through the `catalog_etl_*` tools — see "Not controllable via chip (yet)" in `SKILL.md`.

Notes:
- ThoughtSpot's manifest uses a generated typeId; confirm the live value with
  `edge_list_capability_types` if `edge_create_capability` rejects the id above.
- SAP capabilities (AI Core, Datasphere, Data Products, S/4HANA) are out of scope for this
  skill — they're owned by the JDBC Ingestion team.
