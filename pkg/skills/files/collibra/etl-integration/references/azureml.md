# Azure Machine Learning (`azureml-synchronization`)

Extracts metadata from Microsoft Azure Machine Learning workspaces and ingests it into Collibra DGC as **Azure AI Model Version** (`AzureAIModel`) assets, including performance metrics fetched from the embedded MLflow tracking server.

**Docs:** [About the Azure ML integration via Edge](https://productresources.collibra.com/docs/collibra/latest/Content/Catalog/IntegrateAIModels/AzureAIModel/co_about-azure-ai.htm)

**Connection type:** Azure (generic). Auth method (set on `edge_create_connection`):
- **Service Principal** — OAuth 2.0 Client Credentials flow; requires `clientId`, `clientSecret`, and `tenantId` for the Azure AD application.

**Workflows:** single inbound workflow `main` (omit `workflow` argument — there is no outbound flow).

## Notable generic-config choices (fetch exact schema via `catalog_etl_get_schema`)
- `domain` (required) — target Collibra Technology Asset Domain where `AzureAIModel` assets are created.
- `resourceGroupName` (required) — Azure resource group containing the workspace.
- `workspaceName` (required) — Azure ML workspace to synchronize; must be unique within the resource group.
- `customAiMetricsMappings` (optional) — maps MLflow metric keys (e.g., `f1_score_macro`) to Collibra attribute type IDs on the `AzureAIModel` asset type. The only metric auto-populated without a mapping is `accuracy` → `ModelAccuracy`; all other standard attributes (`ModelPrecision`, `MeanSquaredError`, `MeanAbsoluteError`) require an explicit mapping entry here.

## Gotchas
- **Latest version only**: one `AzureAIModel` asset is created per model container, representing only the latest registered version.
- **AutoML models silently skipped**: models whose names match the AutoML child-run pattern are excluded without warning; if a workspace appears empty, verify it contains user-registered (non-AutoML) models.
- **Custom metric validation is eager**: if a `customAiMetricsMappings` entry references an attribute type not already assigned to the `AzureAIModel` asset type in Collibra, the synchronization aborts before fetching any data. Pre-assign the attribute types first.
- **No metrics when no job run**: models that were registered without an associated MLflow job run ID produce assets with name, version, and description only — no metric attributes are populated.
- **`InitiatingUserInSource` requires DGP 2026.07+**: on earlier platform versions this attribute is silently omitted even when present in the source.
- **Azure permissions**: the service principal needs `Microsoft.MachineLearningServices/workspaces/models/versions/read` and `workspaces/jobs/read`. The built-in **AzureML Data Scientist** role covers both; a plain **Reader** role retrieves models but no MLflow metrics.
