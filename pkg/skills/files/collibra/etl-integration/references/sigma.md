# Sigma Computing (`sigma-synchronization`)

Extracts metadata from Sigma Computing (workspaces, workbooks, datasets, data models, connections, and their columns/elements) and ingests it into DGC. `capability-type` (the `typeId` for `edge_create_capability`): **`sigma-synchronization`**.

**Docs:** [About Sigma integration](https://productresources.collibra.com/docs/collibra/latest/Content/Catalog/Sigma/co_about-sigma-integration.htm)

**Connection type:** Sigma (OAuth 2.0 client credentials). Auth parameters (set on `edge_create_connection`):
- `clientId` — OAuth client ID from Sigma Admin Portal → Developer Access.
- `clientSecret` — OAuth client secret (shown only once at creation; store it safely).
- `url` — Region-specific Sigma REST API base URL (e.g. `https://aws-api.sigmacomputing.com`; also `gcp-api`, `azure-api` variants).
- `tenantUrl` — Your Sigma tenant URL (e.g. `https://app.sigmacomputing.com/<tenant-name>`); used to build asset `Url` attributes.

**Workflows:** inbound is `main` (default — omit `workflow`). No outbound (DGC → Sigma) flow.

## Notable generic-config choices (fetch exact schema via `catalog_etl_get_schema`)
- `domain` (required) — UUID of the DGC domain where all ingested assets will land.
- `workspaces` (optional) — array of workspace IDs or case-insensitive workspace names to limit scope. Empty = all workspaces the OAuth client can see. The catalog UI pre-populates this field as a dropdown via an automatic lightweight pre-flight call.

## Gotchas
- The OAuth client must have **Admin** or **Creator** account type in Sigma. A Member-level client cannot enumerate org-wide content reliably.
- Admin account type alone lets the capability list a workspace and create the workspace asset, but **not** walk its contents (folders, workbooks, datasets, data models). Those workspaces appear in the sync report as "could not be synchronized." Fix: add the OAuth client as a workspace member with at least Viewer permissions.
- Setting `workspace-ids` on the capability **and** `workspaces` in the catalog config simultaneously throws `InvalidParametersException` at startup. Use only one.
- Workspace filter entries that match neither a workspace ID nor a workspace name are tracked as failures. If all configured entries fail, the job status becomes `FAILED`; partial failures yield `COMPLETED_WITH_ERRORS`.
