---
description: Set up, configure, schedule, and run Edge ETL integrations (metadata sync capabilities like Dataplex, Databricks Unity Catalog, Purview, Sigma) end to end.
related: collibra/jdbc-ingestion, collibra/discovery
---

# ETL integrations — connect, configure, schedule, run

An ETL integration is an Edge **capability**: a small app running in the Edge Kubernetes
cluster that extracts metadata from a third-party system and loads it into the Collibra
Data Governance Platform (DGC). Setup spans two systems — **Edge** (connection +
capability) and **DGC catalog** (the generic config + schedule/run). No single tool does
the whole thing; calling them out of order is the most common way this fails.

## First: confirm this is the right ingestion path

Many data sources support **more than one** ingestion path — most commonly a native
**ETL integration** (this skill) *and* generic **JDBC** (`collibra/jdbc-ingestion`). Which
one to use is a real decision, not a default.

**Whenever the user asks to create an integration, before doing anything else, determine
which ingestion type it should be** — don't assume ETL just because this skill is loaded:

1. Identify the data source.
2. Check whether more than one path supports it (`references/index.md` lists the data
   sources with a native ETL integration; anything reachable over a JDBC driver can also go
   through `collibra/jdbc-ingestion`).
3. **If more than one path is possible, ask the user which they want** before starting.
   Only proceed with this ETL skill once the user has chosen it (or it's the only supported
   path).

## The three configuration layers (merged at run time)

When a run starts, DGC's catalog module merges three parameter sets into one and sends
them to the Edge site to start the capability's workflow; it also creates a **Job** in
DGC to track progress and, on completion, records status metrics and a report.

1. **Edge connection** — credentials/endpoint for the third-party system.
   `edge_create_connection`.
2. **Edge capability** — the integration instance and its capability parameters (most are
   shared across capabilities, some are specific). `edge_create_capability`. **Its id is
   the `ingestibleId`** you use for every catalog step below.
3. **Generic (ETL) config** — the sync configuration specific to the integration.
   `catalog_etl_save_generic_config`.

## The full sequence

1. **`edge_list_sites`** → the `edgeSiteId` you'll reference below.
2. **`edge_list_capability_types`** (pass a `query`) → find the connection type and
   capability type for your integration and read their parameter manifests. Don't guess
   parameter names — read them here.
3. **`edge_create_connection`** → the credentials for the third-party system.
4. **`edge_create_capability`** → `typeId` = the capability type from step 2;
   `parameters.connection` = the connection id from step 3. **The returned capability id
   is your `ingestibleId`.**
5. **`catalog_etl_get_schema(ingestibleId)`** → the JSON schema for the generic config.
   **Always fetch this and shape the config to match it — never hardcode or guess field
   names.** (See "Never hardcode the config schema".)
5a. **Resolve DGC prerequisites** — nearly every integration's generic config requires at
   least a `domain` UUID; many also require a `community` or a `system` (System asset) UUID.
   **Before building the config, ask the user** whether they want to use an existing
   domain/System or create new ones. (See "DGC prerequisites".)
6. **`catalog_etl_save_generic_config(ingestibleId, configuration)`** → the config JSON,
   conforming to the schema from step 5. Read it back any time with
   **`catalog_etl_get_config`**.
7. Run or schedule:
   - **`catalog_etl_start_job(ingestibleId[, workflow])`** — run immediately.
   - **`catalog_etl_add_schedule(ingestibleId, cronExpression, cronTimeZone[, workflow])`**
     — schedule for later. Inspect/update/remove with
     `catalog_etl_get_schedule` / `catalog_etl_get_all_schedules` /
     `catalog_etl_update_schedule` / `catalog_etl_delete_schedule`.
8. **Track:** poll the DGC job with **`get_job_status`** (find recent runs with
   `jobs_find`); **`list_integrations`** gives the schedule + last/next run overview and is
   the entry point for "show me all integrations" questions. Stop a running sync with
   `catalog_etl_cancel_job`.

## Workflows

A capability has one or more workflows. **Inbound is the default** — omit the `workflow`
argument to target it. Some integrations expose multiple inbound workflows (e.g. different
entry types) and some an outbound workflow (DGC → third party). `catalog_etl_start_job`,
`catalog_etl_add_schedule`, `catalog_etl_update_schedule`, `catalog_etl_delete_schedule`,
and `catalog_etl_cancel_job` all take an optional `workflow` — omit it for the default
inbound, or set it to target a specific one. The **generic config is per-integration**
(one config via `get_schema`/`save_generic_config`/`get_config`), not per-workflow.

## DGC prerequisites: domain, community, and System asset

Most integrations require one or more of these DGC objects to already exist before
`catalog_etl_save_generic_config` can succeed. The exact required fields are in the schema
from `catalog_etl_get_schema` — but the pattern is consistent across integrations:

| Field in generic config | DGC object | Notes |
|---|---|---|
| `domain` | Domain | Where ingested assets land. Almost universal. |
| `community` | Community | Required by some integrations (e.g. Dataplex CloudSQL, Purview). |
| `system` | System asset | Target System asset for the ingested data. Required by several integrations. |

**Always ask the user before building the config:**

> "Do you already have a domain/System you want to use, or should I create new ones?"

### Using existing objects

- Resolve a domain or System UUID with **`search_asset_keyword`** (filter by asset type
  `"Domain"` or `"System"`) or **`prepare_create_asset`** (lists available domains and their
  communities).

### Creating new objects

Follow this order — each step is a prerequisite for the next:

1. **`create_community`** — if no suitable community exists yet.
2. **`create_domain`** — within the community. Use **`find_domain_types`** to resolve the
   domain type name (e.g. `"Technology Asset Domain"`) to a `typeId` UUID first.
3. **`create_asset`** — to create a System asset, if required. Use **`prepare_create_asset`**
   to resolve the `"System"` asset type UUID and pick the target domain before calling.

## Never hardcode the config schema

Each integration's generic-config schema lives in that integration's own repo (as a
react-json-schema-form data-schema) and evolves with the integration's version. This skill
deliberately **does not embed those schemas** — always call `catalog_etl_get_schema` at run
time. That endpoint returns the very schema DGC uses to validate
`catalog_etl_save_generic_config`, so it can never be stale or wrong for the customer's
version. Likewise, read connection/capability parameter names from
`edge_list_capability_types`, not from memory. The per-integration references capture the
stable conceptual detail (purpose, auth options, flows, gotchas) — not exact field lists.

## Per-integration references

For a specific integration, load its reference for auth types, flows, the capability/
connection `typeId`s, and notable config nuances: `references/<integration>.md` (e.g.
`references/dataplex.md`). `references/index.md` lists them all. These **complement, not
replace,** the live schema from `catalog_etl_get_schema`.

## Distinct from jdbc-ingestion

`collibra/jdbc-ingestion` is a specialized flow: it registers a Database asset via
`configure_database` and runs via `start_ingestion` (DGC's `synchronizeMetadata`), **not**
`catalog_etl_start_job`. Use that skill for JDBC database ingestion; use **this** skill for
the generic/ETL sync capabilities (Dataplex, Databricks Unity Catalog, Purview, Sigma,
ThoughtSpot, …).

## Not controllable via chip (yet)

The cloud **object-storage** crawlers are **not** ETL integrations in the sense above and
**cannot be configured through chip** — they use different endpoints and a different
configuration model than the `catalog_etl_*` / edge tools here:

- **AWS S3** (`s3-synchronization`)
- **Azure Data Lake Storage / ADLS** (`adls-synchronization`)
- **Google Cloud Storage / GCS** (`gcs-synchronization`)

If a user asks to set up, configure, or run one of these, tell them it isn't supported via
chip yet and must be configured in the Collibra/Edge UI — do not attempt it with the tools
in this skill.

## Hard rules

1. **Fetch the config schema (`catalog_etl_get_schema`) before building the generic
   config** — never guess field names or reuse a stale schema.
2. **Read connection/capability parameters from `edge_list_capability_types`,** not memory.
3. **Confirm scope with the user** (which projects/datasets/filters) — don't default to
   "ingest everything."
4. **Two job-id spaces:** Edge jobs every started edge capability produce one → `edge_get_job_status`;
   DGC jobs (from `catalog_etl_start_job`) → `get_job_status`. Never cross them. ETL capabilities utilize DGC jobs. Don't check Edge jobs unless directly asked
5. **Inbound only** unless the user explicitly asks to configure an outbound flow.
6. **Before building the generic config, ask the user** whether to use existing DGC
   domain/community/System assets or create new ones — never assume they exist. See
   "DGC prerequisites".
7. To fill in parameters ask user questions and before saving them confirm with the user.
8. Before creating capability confirm capability parameters with the user