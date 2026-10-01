---
description: Set up, create, configure, and run technical lineage (techlin) harvesting for a registered database (Snowflake, …) — resolve the connection, create and configure the Technical Lineage capability, trigger and track the harvest. Use for any "create/set up/enable/run technical lineage" request; the whole flow, including creating the capability when it is missing, lives here. For querying existing lineage, load collibra/lineage instead.
related: collibra/jdbc-ingestion, collibra/etl-integration, collibra/lineage, collibra/discovery
---

# Technical lineage (techlin) — prerequisites, capability, harvest

Technical lineage — **techlin** for short; the lineage server and the `techlinHost`/
`techlinKey` properties carry the same name — extracts data-flow lineage (views,
procedures, query history) from a data source and loads it into Collibra's lineage
graph. On Edge it is a **capability** (display name "Technical Lineage for <Source>")
that reads the source through the same **Generic JDBC connection** used for
jdbc-ingestion, and is triggered through a dedicated catalog endpoint keyed by the
**Database asset** — not by the capability.

Whatever the user asked for — "set up", "run", "harvest" — the flow is the same and
**starts by checking whether the capability exists, never by triggering**. If it
exists, trigger it. If it doesn't, creating and configuring it is part of **this
skill**: load the per-source reference, run the parameter conversation, and create it
here. Never hand the creation off to another skill, and never call
`start_technical_lineage` before the capability exists.

This skill is about *producing* technical lineage. For *querying* the lineage graph
(upstream/downstream, transformations), see `collibra/lineage`.

## The catalog-etl parallel — same lifecycle, different tools

Configuring a techlin capability follows the same approach as an ETL integration
(`collibra/etl-integration`), only spelled with edge tools. There is no separate
"generic config" layer — **the capability's parameters are the entire configuration**:

| Step | catalog-etl | techlin |
| --- | --- | --- |
| See which parameters exist | `catalog_etl_get_schema` | `edge_list_capability_types` (manifest — an inventory, see below) |
| Save the configuration | `catalog_etl_save_generic_config` | `edge_create_capability` (upserts by `capabilityId`) |
| Read the config back | `catalog_etl_get_config` | `edge_get_capability` |
| Run | `catalog_etl_start_job` / schedules | `start_technical_lineage(assetId)` — no schedules |
| Track | `get_job_status` | `jobs_find` → `get_job_status` |

One important difference: `catalog_etl_get_schema` returns a JSON Schema the server
enforces when the config is saved; the Edge manifest is **not** enforced as a
contract. At save time Edge materializes defaults for required manifest parameters
(e.g. the harvest queries, `active`) and rejects a second capability of the same type
on one connection — but it accepts any parameter *values* and accepts missing
runtime-required ones (e.g. Snowflake's `databaseNames`). So the dangerous failure
mode is wrong or missing values, and the parameter conversation (step 6 below) is
never optional — a capability created from guesses harvests the wrong thing, or fails
asynchronously in the harvest job.

## Not this skill: ETL-owned lineage capabilities

Some sources have lineage capabilities managed as **ETL integrations** by another team
— Databricks Unity Catalog (`databricks-lineage`) and GCP/Dataplex
(`dataplex-lineage-synchronization`). Those use their own connection types (not the
Generic JDBC connection), have no Database-asset prerequisite, and are configured and
scheduled through `catalog_etl_*` — route them to `collibra/etl-integration`.
Conversely, techlin (`edgeharvester-*`) capabilities are never created or run from the
ETL skill.

## Prerequisites — check these first, before any setup

Technical lineage builds on top of a completed jdbc-ingestion:

1. A **Database asset** must exist for the data source, registered via
   `register_database` (`collibra/jdbc-ingestion`). Its UUID is the key that triggers
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
   input of step 2 and the `assetId` for step 7.
2. **`get_registered_database`** with that UUID → the **Edge connection** backing the
   ingestion and its **`edgeSiteId`**. Both are derived, not chosen: the connection id
   feeds the capability's `connection` parameter, and the capability **must be created
   on that same edge site**. A failure here means the asset is not a registered
   database — treat it as a missing prerequisite (see above). Only if this tool is
   unavailable, fall back to `edge_list_sites` + `edge_find_connections` and confirm
   the picks with the user.
3. **`edge_find_capabilities`** on that edge site → does a **Technical Lineage
   capability for this source** already exist (one whose `connection` parameter is the
   step-2 connection)? If yes, skip to step 7. If no, tell the user the capability has
   to be created and configured first, and continue with steps 4–6 — the creation
   happens **here, in this skill**; do not load another skill for it and do not
   trigger anything yet.
4. **Load the per-source reference** — `references/index.md` lists the supported
   sources and their capability typeIds; load the source's file (e.g.
   `references/snowflake.md`) **before asking the user anything**. It defines which
   questions to ask and in what order. If the source has no reference: when it is one
   of the ETL-owned lineage capabilities, route to `collibra/etl-integration`;
   otherwise walk the step-5 manifest through with the user parameter by parameter —
   never guess.
5. **`edge_list_capability_types`** (the `edgeSiteId` from step 2; pass a `query`, e.g.
   `"snowflake"`) → find the **Technical Lineage** capability type and read its
   parameter manifest — an **inventory**: authoritative for which parameters exist on
   the installed version, their exact names, and their options. Requiredness and mode
   behavior come from the per-source reference (step 4), not from the manifest's
   `required` flags — those describe the Edge UI form. If the manifest lists a
   parameter the reference doesn't mention, surface it to the user; never guess.
   **Several capability types can match the data source name — only the
   technical-lineage one is correct.** See "Exactly one right capability type".
6. **The parameter conversation — a hard checkpoint.** Collect every parameter value
   with the user in the order the per-source reference prescribes (mode-gating
   question first when the source has one), then present the complete parameter set
   and get an explicit go-ahead. Only after the user confirms:
   **`edge_create_capability`** → `typeId` from step 5, `edgeSiteId` and
   `parameters.connection` from step 2, remaining `parameters` exactly as confirmed.
   See "Capability parameters".
7. **`start_technical_lineage`** with the Database asset UUID from step 1 — **not** the
   capability id. Only reached once step 3 (or step 6) confirmed the capability exists.
   If the user opted into the techlin server configuration in step 6, wait for their
   confirmation that `techlinKey` was added in the Edge UI before triggering.
   Success means *submitted*, nothing more.
8. **Track:** the trigger returns no job id. Find the spawned DGC job with **`jobs_find`**
   (jobs started at/after the trigger), then poll it with **`get_job_status`** until it
   reaches a terminal state. Never poll with `edge_get_job_status` — the Edge-side
   harvest is tracked through the DGC job.

## Exactly one right capability type

An Edge site lists many capability types, and more than one can contain the data-source
name (e.g. jdbc-ingestion profiles, sync capabilities, *and* technical lineage for
Snowflake). The technical-lineage capability is recognizable by its identity, not by the
name substring alone — for Snowflake it is the type whose manifest displays
**"Technical Lineage for Snowflake"** (`references/index.md` lists the supported
sources and their expected typeIds). If the expected type is not present on the site,
list what `edge_list_capability_types` returned and ask the user — never fall back to
another `*snowflake*` type.

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

Read the parameter names and options from the **live manifest**
(`edge_list_capability_types` with a `query`) — never from memory; manifests evolve with
the capability version. The per-source reference (via `references/index.md`) is
authoritative for what the fields *mean*, which are required at run time, how the
ingestion mode gates them, and which questions to ask in which order; the manifest's
own `required`/`default` flags describe the Edge UI form, not what the harvest needs.

**This conversation is the checkpoint between "the capability is missing" and
`edge_create_capability` — it always happens, in full, even when the user has already
said "yes, create it".** That yes authorizes the creation, not skipping the questions.
Never ask for database credentials — they live in the Edge connection, not in
capability parameters.

Ordering and rules for the conversation:

1. **Mode first.** If the source has an ingestion-mode parameter (Snowflake:
   `snowflakeMode`), settle it before anything else — the mode decides which of the
   remaining questions apply, so asking anything else before it wastes the user's
   time. The per-source reference documents the gating; the manifest does not encode
   it.
2. **Collect every runtime-required parameter the reference lists**, regardless of
   the manifest's `required` flags. For Snowflake, `databaseNames` is required in
   **both** modes even though the manifest marks it optional — Edge accepts a
   capability without it, and the harvest then fails asynchronously with "No database
   names provided".
3. **Source ID must be unique.** Propose a value, but never one that collides with an
   existing lineage source id (check the `id` parameter of existing lineage
   capabilities via `edge_list_capabilities`/`edge_find_capabilities`), with the
   `collibraSystemName`, or with a database/schema name. A colliding Source ID silently
   overwrites or cross-links another source's lineage.
4. **Proactively offer the techlin server configuration** (`techlinHost` /
   `techlinKey` custom properties) — they point the capability at the Collibra Data
   Lineage (techlin) server and are passed **inside the `customParameters` list
   parameter** (the per-source reference shows the exact encoding), never as
   top-level parameters. They are optional, and the two are handled differently:
   when the user wants them, collect **`techlinHost` only** and create the
   capability without the key — **`techlinKey` is a secret and is never collected,
   stored, or echoed in the conversation**. After creation, the user adds
   `techlinKey` in the Collibra/Edge UI (Custom Properties, marked secret);
   **trigger the harvest only after the user confirms the key is in place**.
   Declining both is the user's decision, never a silent default.
5. **Confirm the complete set.** Present every parameter — chosen values, applied
   defaults, custom properties — in one table and get an explicit confirmation before
   calling `edge_create_capability`.

## Updating an existing capability

Read the current configuration back with `edge_get_capability`; change it by calling
`edge_create_capability` with the same `capabilityId` (it upserts). Start from the
read-back `parameters`, apply only the agreed changes, and send the full set — that
preserves the server-materialized values (harvest queries, `active`) without
re-collecting them. Parameter changes go through the same conversation-and-confirm
rule as creation — show what will change and get a go-ahead first.

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
2. **Capability before trigger.** Never call `start_technical_lineage` until
   `edge_find_capabilities` has confirmed a Technical Lineage capability exists for the
   source's connection. Triggering without one sets nothing up — it fails or spawns a
   failed job. When it's missing, say so and switch to the creation conversation.
3. **Creating the capability is this skill's job.** A techlin capability is not an ETL
   integration — never load `collibra/etl-integration` (or any other skill) to create
   it, and never use `catalog_etl_*` tools on it. The ETL-owned lineage capabilities
   (`databricks-lineage`, `dataplex-lineage-synchronization`) are the one exception —
   route those to `collibra/etl-integration`.
4. **Load the per-source reference before the parameter conversation**
   (`references/index.md` → `references/<source>.md`) and **read the live manifest**
   (`edge_list_capability_types`) — the reference for meaning, question order, and
   requiredness; the manifest for exact names and options; never memory. Manifest
   parameters the reference doesn't mention are surfaced to the user, not guessed.
5. **Ask the user for parameter values and confirm the full set** before
   `edge_create_capability` — even when the user already approved creating the
   capability; approval covers the creation, not skipping the questions. Never ask
   for credentials, and never collect the `techlinKey` value in the conversation —
   the user adds it in the Edge UI after creation (see the per-source reference).
   The `connection` value and the `edgeSiteId` are not user choices — take both from
   `get_registered_database`; the capability lives on the same edge site as the
   connection.
6. **Only `start_technical_lineage` triggers a harvest** — never `edge_run_capability`
   and never `catalog_etl_start_job`. Pass the **Database asset** UUID, not the
   capability id.
7. **Always locate and poll the DGC job after triggering** (`jobs_find` →
   `get_job_status`). A 202/success alone is not a completed harvest.
8. **When several capability types match the source name, never pick one silently** —
   match the technical-lineage identity or ask.
9. **Databases only.** This flow harvests from the database over the JDBC connection —
   never ask where files are stored. Lineage for SQL files in cloud storage is not
   supported via chip; route those users to the Collibra/Edge UI.
10. **One capability per (type, connection).** When `edge_create_capability` fails
    with `400 — "Connection(s) […] already used in the capability of the same type"`,
    the capability already exists — go back to `edge_find_capabilities` and the
    update or trigger path. Never retry with a different connection.