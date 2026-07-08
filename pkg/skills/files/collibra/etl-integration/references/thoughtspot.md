# ThoughtSpot Synchronization (`thoughtspot-synchronization`)

Extracts metadata from a ThoughtSpot cloud tenant — Tenant, Orgs, Connections, Liveboards, Answers, Answer Visualizations, Data Models (Worksheets), Data Entities (Tables/Views), and Data Attributes (columns) — and ingests it into a Collibra DGC domain. A separate lineage workflow additionally submits column-level lineage edges to Collibra TechLin. The `capability-type` (the `typeId` for `edge_create_capability`) is **`thoughtspot-synchronization`** (confirmed in `manifest-template.yaml`; the published `manifest.yaml` uses the `<CAPABILITY_TYPE>` build-time placeholder, so the resolved value is the same string).

**Docs:** [About ThoughtSpot integration via Edge](https://productresources.collibra.com/docs/collibra/latest/Content/CollibraDataLineage/DataSources/ThoughtSpot/co_about-thoughtspot-integration.htm)

**Connection type:** Generic. Auth method (set on `edge_create_connection`):
- **Username / Password** — ThoughtSpot username (`userName`), password (`password`), and tenant base URL (`url`, e.g. `https://customer.thoughtspot.cloud`). The capability authenticates via the ThoughtSpot REST API v2 full-token endpoint and re-authenticates per org to obtain an org-scoped bearer token.

**Workflows:** The integration exposes two runnable tabs in the catalog Integration Configuration page:
- **Metadata Inbound** (`default` workflow, `-DstartMode=sync`) — inbound; fetches all ThoughtSpot entity types listed above and creates Catalog assets in the configured DGC domain.
- **ThoughtSpot Lineage** (`lineage` workflow, `-DstartMode=lineage`, Preview) — submits column-level and asset-level lineage relations to Collibra TechLin. Requires the **Metadata Inbound** tab to be configured first.

## Notable generic-config choices (fetch exact schema via `catalog_etl_get_schema`)

**Metadata Inbound tab:**
- `domain` (required) — UUID of the DGC domain where ThoughtSpot assets will be created.
- `orgs` (optional) — multi-select list of ThoughtSpot org IDs or names to include. Leave empty to sync all orgs the credentials can see.

**ThoughtSpot Lineage tab:**
- `domain` (required) — UUID of the DGC domain that owns the lineage source.
- `sourceId` (required) — stable string identifier used to construct the lin-sdk source name (`ThoughtSpot-<sourceId>-<dgcHost>`). Changing this across runs creates a new parallel lineage source instead of updating the existing one.
- `orgs` (optional) — same org-filter semantics as the sync tab; should be aligned with the sync configuration to avoid partial graphs.

The lineage capability also requires these **install-time** run-parameters (not in the generic config): `targetDgc` (DGC URL), `username` and `password` (Collibra DGC credentials for lin-sdk — distinct from the ThoughtSpot connection credentials). Optionally `customParameters.techlinHost` + `customParameters.techlinKey` if bypassing DGC-routed lineage submission.

## Gotchas

- **Run sync before lineage.** Lineage edges stitch onto assets the sync flow has already created. Running lineage against an empty domain produces a disconnected gray graph in TechLin with no errors.
- **Do not set `org-ids` at the capability level and `orgs` in the generic config at the same time** — the capability throws `InvalidParametersException` at startup. Use one or the other.
- **`sourceId` must be stable.** Changing it between lineage runs creates a new lin-sdk source instead of overwriting the previous one, resulting in duplicate lineage graphs.
- **Lineage is all-or-nothing per run.** If any org's metadata fetch fails during the lineage workflow, the entire job aborts before submitting any edges to TechLin. The sync workflow does not share this property.
- **Data Entities are silently dropped** if the capability cannot resolve their ThoughtSpot Connection (by `dataSourceId` or by name). Their columns are dropped too. This happens when the Connection is not visible to the service user or when multiple connections share a name and the fallback name-match is ambiguous.
- **Private / embed ThoughtSpot deployments** (non-`*.thoughtspot.cloud` hosts) get an incorrect `Url` attribute on the Tenant asset — the synchronization itself still works because API calls use the user-supplied connection URL.
- **Only TABLE-type visualizations** produce per-column Data Attribute assets. CHART, HEADLINE, and other viz types are not expanded into attributes, and the lineage flow cannot draw column-level edges into them.
- **The `defaultStatus` and `cloudIngestionJobId` run-parameters** are declared in the manifest but are not read by the current code — setting them has no effect.
- **`save-input-metadata=true`** (install-time flag) records all source metadata in a downloadable DGC file. Use only when requested by Collibra Support — the file can be large and contains source-system content.
