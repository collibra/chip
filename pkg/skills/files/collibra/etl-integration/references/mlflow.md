# MLflow AI (`mlflow-synchronization`)

Extracts metadata from an MLflow model registry and ingests `MlflowAIModel` assets into a
Collibra Technology Asset Domain. `MlflowAIModel` is a subtype of `AIModel` in the Collibra
AI Governance operating model; the capability creates one asset per registered model (latest
READY version only).

**Docs:** [About the MLflow AI integration via Edge](https://productresources.collibra.com/docs/collibra/latest/Content/Catalog/IntegrateAIModels/MLfLowAI/co_about-mlflow.htm)

**Connection type:** `generic` (HTTP-based). Auth methods (set on `edge_create_connection`):
- **Basic Authentication** — `username` and `password` credentials against the MLflow tracking server URL.
- **OAuth (Client Credentials)** — `tokenUrl`, `clientId`, and `clientSecret`; compatible with Databricks OAuth (M2M).

**Workflows:** single inbound workflow `main` (default — omit `workflow`). No outbound flow.

## Notable generic-config choices (fetch exact schema via `catalog_etl_get_schema`)
- `domain` (required) — UUID of the Technology Asset Domain where `MlflowAIModel` assets are created or updated.
- `customAiMetricsMappings` (optional) — array of `{ from, to }` objects mapping an MLflow metric key (e.g., `f1_score`) to a Collibra attribute type UUID. Extends the four standard auto-mapped metrics (`accuracy`, `precision_score`, `mae`, `mse`). A custom mapping entry can also override a standard metric, storing the value in both the standard attribute and the custom one.

## Gotchas
- Only the **latest READY version** of each registered model is ingested. Models with no `READY` versions are silently skipped — no asset is created or updated.
- If a model version has no associated `run_id`, or the training run was deleted (MLflow returns 404 on `/runs/get`), all metric attributes are silently omitted for that asset.
- Every attribute type referenced in `customAiMetricsMappings` must already be **assigned to the `MlflowAIModel` asset type** (`00000000-0000-0000-0000-000000031410`) in Collibra before the sync runs. Validation happens at startup; a missing assignment aborts the entire synchronization.
- The four standard metric keys (`accuracy`, `precision_score`, `mae`, `mse`) are hard-coded. If the MLflow run uses different key names (e.g., `acc` instead of `accuracy`), the standard attributes will be empty — use `customAiMetricsMappings` to remap them.
