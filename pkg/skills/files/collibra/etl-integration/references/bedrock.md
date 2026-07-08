# AWS Bedrock AI (`bedrock-synchronization`)

Extracts AI metadata from AWS Bedrock (foundation models, custom/imported model
versions, deployments, endpoints, agents, vendors, and — when data ingestion is
enabled — the S3 training datasets and invocation-log output locations) and ingests
it into DGC. `capability-type` (the `typeId` for `edge_create_capability`):
**`bedrock-synchronization`**.

**Docs:** [About the AWS Bedrock AI integration via Edge](https://productresources.collibra.com/docs/collibra/latest/Content/Catalog/IntegrateAIModels/AWSBedrock/co_about-aws-bedrock-ai.htm)

**Connection type:** AWS (`family: generic`). Auth methods (set on `edge_create_connection`
via the `authentication` property):
- **IAM static credentials** — `authentication = IAM` plus `aws_access_key_id` /
  `aws_secret_access_key`; for Edge running outside AWS.
- **Instance / Pod role** — any other `authentication` value; keys left blank, resolves
  the EC2 instance role or EKS IRSA role via the AWS default credentials chain.

**Workflows:** inbound only, `main` (default — omit `workflow`). There is no outbound
(DGC → Bedrock) flow.

## Notable generic-config choices (fetch exact schema via `catalog_etl_get_schema`)
- `domain` (required) — target domain UUID; all catalogued assets are placed here.
- `regions` — AWS regions to scan; if empty, every region where Bedrock is generally
  available is scanned (only regions actually offering Bedrock return results).
- `ingestAiInputAndOutput` (default true) — also ingest S3 File/File Container assets for
  custom-model training data and deployment invocation-log output; when false, only model
  metadata is ingested (cheaper, fewer AWS calls).
- `customAiMetricsMappings` — map a custom Bedrock training/validation metric name to a
  Collibra AI Model attribute type; without mappings no metric values are persisted.

## Gotchas
- IAM mode requires **both** keys — the run fails with `InvalidParametersException` if
  either is missing while `authentication = IAM`.
- Per-region resilience: a region returning 4xx is marked "without access" and a region
  where Bedrock isn't deployed (`UnknownHostException`) is marked "without Bedrock"; the
  run continues and reports **Completed with Errors** if at least one region returned data,
  or **Failed** only if every region was rejected.
- `ingestAiInputAndOutput` triggers extra AWS calls (`GetCustomModel`,
  `GetModelInvocationLoggingConfiguration`) — CloudWatch-only logging is skipped, and
  missing per-model permissions degrade gracefully (model imported without metrics).
- `save-input-metadata` uploads a ZIP of raw AWS responses to DGC; enable only at Collibra
  Support's request, as it snapshots all metadata visible to the AWS principal.
