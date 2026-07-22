# AWS SageMaker AI (`sagemaker-synchronization`)

Extracts ML model metadata from Amazon SageMaker AI and ingests it into DGC as a structured AI asset hierarchy: AI Base Models → AI Model Versions → AI Model Deployments → AI Endpoints + AI Monitors. The `typeId` for `edge_create_capability` is **`sagemaker-synchronization`**.

**Docs:** [About the AWS SageMaker AI integration via Edge](https://productresources.collibra.com/docs/collibra/latest/Content/Catalog/IntegrateAIModels/AWSSageMaker/co_about-aws-sagemaker-ai.htm)

**Connection type:** AWS. Auth methods (set on `edge_create_connection`):
- **IAM** — programmatic user; requires `aws_access_key_id` and `aws_secret_access_key`.
- **EC2** — instance role; no explicit credentials needed (suitable when the Edge site runs on an EC2 instance with an attached IAM role).

**Workflows:** single inbound workflow `main` (omit the `workflow` argument). No outbound flow.

## Notable generic-config choices (fetch exact schema via `catalog_etl_get_schema`)
- `domain` (required) — target Collibra Domain where all AI assets are created (AI Base Models, AI Model Versions, AI Model Deployments, AI Endpoints, AI Monitors).
- `regions` (optional) — list of AWS region identifiers to scope ingestion; omit to scan all SageMaker-enabled regions automatically.
- `ingestDeploymentOutput` (optional, default `false`) — when enabled, endpoint output paths from each endpoint configuration are ingested as `FileContainer` and `File` assets linked to the corresponding `AIModelDeployment`.
- `customAiMetricsMappings` (optional) — maps SageMaker metric key names (supports nested JSON lookup) to DGC attribute type UUIDs on the AI model asset. Useful for ingesting non-standard or user-defined model metrics beyond the built-in accuracy/precision/MSE/MAE fields.

## Gotchas
- Only model packages with `ModelApprovalStatus = Approved` are ingested. Non-approved versions are silently skipped — if model versions are missing, check their approval status in SageMaker.
- The ETL path is selected automatically at runtime based on DGC version: **≥ 2026.03** produces the full AI asset hierarchy (Base Model → Version → Deployment → Endpoint + Monitor); **< 2026.03** produces a single legacy `AWSSagemakerAIModel` asset per model package group (latest approved version only). No configuration is required or possible.
- The IAM user (or role) needs separate permission sets for model ingestion (`sagemaker:ListModelPackageGroups`, `DescribeModelPackageGroup`, `ListModelPackages`, `DescribeModelPackage`), endpoint/deployment ingestion (`sagemaker:ListEndpoints`, `DescribeEndpoint`, `DescribeEndpointConfig`, `DescribeModel`), and monitoring ingestion (`sagemaker:ListMonitoringSchedules`, `DescribeMonitoringSchedule`, `ListMonitoringAlerts`). Missing any group silently drops that category of assets.
- If a deployment variant cannot be linked to an AI Model Version (by package ARN or model data URL), it is still ingested as a standalone deployment with no version relation. The sync report records the failure, but no error is raised.
- An AI Endpoint asset is only created if at least one of its deployment variants was successfully resolved; endpoints with zero resolved variants are dropped.
- `customAiMetricsMappings.from` performs a recursive key search across the metrics JSON. If the found value is a complex object containing a `"value"` key, that inner value is extracted; otherwise the entire object is serialized to a string. Map leaf metric keys (e.g., `recall`, `accuracy`) rather than parent objects to get clean scalar values.
- The `AIMonitor.Url` attribute is only populated on DGC ≥ 2026.04. The `InitiatingUserInSource` attribute on AI assets requires DGC ≥ 2026.07.
