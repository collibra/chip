---
description: Set up, create, configure, and run technical lineage harvesting for a registered database (Snowflake, …) — resolve the connection, create the Technical Lineage capability, trigger and track the harvest. Use for any "create/set up/enable/run technical lineage" request; for querying existing lineage, load collibra/lineage instead.
related: collibra/jdbc-ingestion, collibra/etl-integration, collibra/lineage, collibra/discovery
---

# Technical lineage — prerequisites, capability, harvest

Technical lineage extracts data-flow lineage (views, procedures, query history) from a
data source and loads it into Collibra's lineage graph. On Edge it is a **capability**
(display name "Technical Lineage for <Source>") that reads the source through the same
**Generic JDBC connection** used for jdbc-ingestion, and is triggered through a dedicated
catalog endpoint keyed by the **Database asset** — not by the capability.

This skill is about *producing* technical lineage. For *querying* the lineage graph
(upstream/downstream, transformations), see `collibra/lineage`. Some sources have their
own lineage sync capabilities managed as ETL integrations instead (Databricks, GCP/Dataplex)
— those go through `collibra/etl-integration`, not this skill.

## Prerequisites — check these first, before any setup

Technical lineage builds on top of a completed jdbc-ingestion:

1. A **Database asset** must exist for the data source, registered via
   `configure_database` (`collibra/jdbc-ingestion`). Its UUID is the key that triggers
   the harvest.
2. **JDBC ingestion must have run** (`start_ingestion`) — harvested lineage stitches to
   the ingested Schema/Table/Column assets; without them the lineage has nothing to
   attach to.

Verify before proceeding: resolve the Database asset with `search_asset_keyword`
(filter by asset type `"Database"`; `collibra/discovery` has the name→UUID rules). If no
Database asset exists, or ingestion never ran: **stop and tell the user** that a
registered database and a completed JDBC ingestion are prerequisites for technical
lineage, and offer to set them up first via `collibra/jdbc-ingestion`. Do not try to
create the Database asset any other way.

## The full sequence

1. **Resolve the Database asset** (see Prerequisites) → keep its UUID; it is both the
   input of step 2 and the `assetId` for step 5.
2. **`get_registered_database`** with that UUID → the **Edge connection** backing the
   ingestion and its **`edgeSiteId`**. Both are derived, not chosen: the connection id
   feeds the capability's `connection` parameter, and the capability **must be created
   on that same edge site**. A failure here means the asset is not a registered
   database — treat it as a missing prerequisite (see above). Only if this tool is
   unavailable, fall back to `edge_list_sites` + `edge_find_connections` and confirm
   the picks with the user.
3. **`edge_list_capability_types`** (the `edgeSiteId` from step 2; pass a `query`, e.g.
   `"snowflake"`) → find the **Technical Lineage** capability type and read its
   parameter manifest. **Several capability types can match the data source name — only
   the technical-lineage one is correct.** See "Exactly one right capability type".
4. **`edge_create_capability`** → `typeId` from step 3, `edgeSiteId` and
   `parameters.connection` from step 2, remaining `parameters` from the manifest — ask
   the user for the values and confirm the full set first. See "Capability parameters".
   Skip if a Technical Lineage capability for this source already exists
   (`edge_find_capabilities`).
5. **`start_technical_lineage`** with the Database asset UUID from step 1 — **not** the
   capability id. Success means *submitted*, nothing more.
6. **Track:** the trigger returns no job id. Find the spawned DGC job with **`jobs_find`**
   (jobs started at/after the trigger), then poll it with **`get_job_status`** until it
   reaches a terminal state. Never poll with `edge_get_job_status` — the Edge-side
   harvest is tracked through the DGC job.

## Exactly one right capability type

An Edge site lists many capability types, and more than one can contain the data-source
name (e.g. jdbc-ingestion profiles, sync capabilities, *and* technical lineage for
Snowflake). The technical-lineage capability is recognizable by its identity, not by the
name substring alone — for Snowflake it is the type whose manifest displays
**"Technical Lineage for Snowflake"** (see `references/snowflake.md` for the expected
`typeId`). If the expected type is not present on the site, list what
`edge_list_capability_types` returned and ask the user — never fall back to another
`*snowflake*` type.

In particular, **capability types for SQL files in cloud storage** (lineage harvested
from `.sql` files in an S3/ADLS/GCS bucket) can also carry the dialect name and their
manifests ask for file/storage locations. Those belong to the separate "lineage for
cloud storage" flow — see "Not supported" below. This skill's flow harvests **directly
from the database** over the JDBC connection; there are no files to locate, so never
ask the user where files are stored.

## Not supported: lineage for cloud storage

Technical lineage from **SQL files in cloud storage** (S3, ADLS, GCS buckets) is a
different flow with different capability types and parameters, and is **not supported
via chip yet**. If the user's lineage source is SQL files in a bucket rather than a
registered database, tell them it must be configured in the Collibra/Edge UI — do not
attempt it with this skill's tools or pick a cloud-storage capability type.

## Capability parameters

Read the parameter names, requiredness, defaults, and options from the **live manifest**
(`edge_list_capability_types` with a `query`) — never from memory; manifests evolve with
the capability version. The per-source reference (`references/snowflake.md`) explains
what the fields *mean* and how to choose values; the manifest stays authoritative for
exact names and defaults.

To fill them in: ask the user for the required values, propose defaults for the
optional ones, and **confirm the complete parameter set with the user before calling
`edge_create_capability`**. Never ask for database credentials — they live in the Edge
connection, not in capability parameters.

## Trigger and track — 202 means submitted, not finished

`start_technical_lineage` wraps a bare POST that returns `202 Accepted` with no body:

- A success response **only** means DGC accepted the harvest request.
- No job id is returned. Locate the spawned DGC job with `jobs_find` right after the
  call (filter to jobs started at/after the trigger) and poll it with `get_job_status`
  to a terminal state before reporting the harvest as done. If the job ends in failure,
  surface its message/status log to the user.

## Hard rules

1. **Verify the prerequisites first** (Database asset + completed ingestion). When
   missing, stop and route to `collibra/jdbc-ingestion` — don't improvise.
2. **Read capability parameters from `edge_list_capability_types`,** not memory; use
   `references/<source>.md` for meaning, the manifest for names/defaults.
3. **Ask the user for parameter values and confirm the full set** before
   `edge_create_capability`. Never ask for credentials. The `connection` value and the
   `edgeSiteId` are not user choices — take both from `get_registered_database`; the
   capability lives on the same edge site as the connection.
4. **Only `start_technical_lineage` triggers a harvest** — never `edge_run_capability`
   and never `catalog_etl_start_job`. Pass the **Database asset** UUID, not the
   capability id.
5. **Always locate and poll the DGC job after triggering** (`jobs_find` →
   `get_job_status`). A 202/success alone is not a completed harvest.
6. **When several capability types match the source name, never pick one silently** —
   match the technical-lineage identity or ask.
7. **Databases only.** This flow harvests from the database over the JDBC connection —
   never ask where files are stored. Lineage for SQL files in cloud storage is not
   supported via chip; route those users to the Collibra/Edge UI.