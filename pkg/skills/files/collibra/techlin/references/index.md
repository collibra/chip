# Technical lineage (techlin) source catalog

Every source below is harvested through the same lifecycle (see the parent
`SKILL.md`): `get_registered_database` → `edge_find_capabilities` →
`edge_list_capability_types` → the parameter conversation → `edge_create_capability`
→ `start_technical_lineage` → `jobs_find`/`get_job_status`.

The **capability typeId** is what you pass to `edge_create_capability` (`typeId`).
Always confirm the live id and the exact parameter names/options via
`edge_list_capability_types`; the per-source reference is authoritative for
requiredness, mode gating, and question order — the manifest's `required` flags
describe the Edge UI form, not what the harvest needs.

| Source | capability typeId | Reference | Notes |
| --- | --- | --- | --- |
| Snowflake | `edgeharvester-snowflake` | `snowflake.md` | two modes (`snowflakeMode`: `SQL` / `SQL-API`) — the mode gates the questions; the gating is documented in `snowflake.md`, not encoded in the manifest |

More sources (Oracle, SAP HANA, …) will be added here as their capabilities ship —
one reference file per source, same lifecycle, no `SKILL.md` change needed.

**Not in this catalog:**

- **Databricks Unity Catalog lineage (`databricks-lineage`) and GCP/Dataplex lineage
  (`dataplex-lineage-synchronization`)** are ETL integrations owned by another team:
  own connection types, no Database-asset prerequisite, configured and scheduled via
  `catalog_etl_*`. Route them to `collibra/etl-integration`.
- **SQL files in cloud storage** (S3/ADLS/GCS buckets) — a different flow with
  different capability types and parameters, not supported via chip; route users to
  the Collibra/Edge UI.