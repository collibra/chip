# Azure AI Foundry (`azure-foundry-synchronization`)

Extracts AI asset metadata from Microsoft Azure AI Foundry (Azure Machine Learning
Workspaces, Azure Cognitive Services, Azure OpenAI) and ingests it into DGC — covering
AI projects, model deployments, AI agents, fine-tuned models, training files, AI
endpoints, and AI monitors. `capability-type` (the `typeId` for
`edge_create_capability`): **`azure-foundry-synchronization`**.

**Docs:** [Steps overview: Integrate Azure AI Foundry via Edge](https://productresources.collibra.com/docs/collibra/latest/Content/Catalog/IntegrateAIModels/AzureAIFoundryModel/co_edge-integrate-azure-ai-foundry-steps.htm)

**Connection type:** Azure. Auth methods (set on `edge_create_connection`):
- **Azure Service Principal (Client Credentials Flow)** — `clientId`, `clientSecret`,
  `tenantId`. This is the only supported authentication method.

The Service Principal needs `Reader` on the subscription plus `AzureML Data Scientist`
(or `Reader`) on each ML Workspace and `Cognitive Services Contributor` (or `Reader`) on
each Cognitive Services account.

**Workflows:** inbound only, `main` (default — omit `workflow`). There is no outbound
(DGC → Azure) flow.

## Notable generic-config choices (fetch exact schema via `catalog_etl_get_schema`)
- `domain` (required) — UUID of the existing DGC domain where all assets will be created.
  All workspaces share one domain; per-workspace domain routing is not supported.
- `workspaceNames` (required) — array of workspace identifiers in
  `"resourceGroup/workspaceName"` format for ML workspaces, or
  `"resourceGroup/accountName/projectName"` for Azure AI Foundry projects. At least one
  entry is required.
- `associateAIUseCasesWithAIModels` (default `true`) — when true, creates `implementedBy`
  relations between existing Collibra AI Use Case assets and ingested AI Model Version /
  Base Model assets.
- `createFileAssets` (default `true`) — when true, ingests file metadata linked to AI
  agents (training files, vector store files) and fine-tuning jobs as `File` and
  `FileContainer` assets.
- `customAiMetricsMappings` (optional) — maps Azure Monitor metric names (e.g.
  `TokenTransaction`, `RequestsPerMinute`) to Collibra attribute type UUIDs on
  `AIModelDeployment` assets. Each entry: `{"from": "MetricName", "to": "<uuid>"}`. The
  UUID must exist as an attribute type in DGC or the job fails at validation before any
  Azure data is fetched.
- `assetDeletionMode` (Edge capability parameter, default `Soft`) — `Soft` archives
  missing assets; `Hard` permanently deletes them.
- `subscriptionId` (Edge capability parameter, required) — Azure Subscription ID
  containing the AI Foundry workspaces.

## Gotchas
- **Hub workspaces are expanded automatically.** If a workspace listed in `workspaceNames`
  has `kind` containing `"hub"`, all child project workspaces are discovered and ingested
  automatically — you do not need to list child projects individually.
- **All assets land in one domain.** There is no per-workspace or per-resource-group
  domain routing. Choose the target domain carefully before the first run.
- **Each run is a full sync.** There is no incremental/delta mode. Every run fetches all
  listed workspaces from scratch.
- **DGC version gates features.** Several asset types only appear on newer DGC versions:
  `AIBaseModel`, `AIEndpoint`, `AIMonitor`, and `Vendor` assets require DGC 2026-03+
  (or 2026-04+ for Vendor). Fine-tuned model assets require DGC 2025-09+. The capability
  enforces these limits at startup and skips unsupported assets rather than failing.
- **Fine-tuned model deployments auto-delete in Azure.** Azure removes inactive fine-tuned
  model deployments after 15 days. The next sync will archive (Soft) or delete (Hard) the
  corresponding `AIModelDeployment` asset in DGC. Schedule runs at least every 15 days to
  keep DGC in sync.
- **`customAiMetricsMappings` UUIDs are validated before any Azure fetch.** If any `to`
  UUID is not a valid attribute type in DGC, the job fails immediately with
  `InvalidParametersException` before contacting Azure.
- **Metric attributes are not populated if Azure Monitor has no data.** If a deployment
  has never received traffic, mapped metric attributes will remain unset.
- **Workspace names must match exactly.** An unrecognised workspace name in `workspaceNames`
  throws `InvalidParametersException` before any data is fetched.
