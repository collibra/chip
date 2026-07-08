---
description: Set up an Edge JDBC connection and capability, register a database, and run jdbc-ingestion end to end.
related: collibra/discovery, collibra/tech-lineage
---

# JDBC ingestion — connect, register, and run

Setting up jdbc-ingestion is a fixed sequence across two separate systems: Edge
(connection + capability) and DGC catalog (database registration + the actual sync).
No single tool does the whole thing — skipping a step or calling tools out of order is
the most common way this goes wrong.

## First: confirm this is the right ingestion path

Many data sources support **more than one** ingestion path — most commonly a native
**ETL integration** (`collibra/etl-integration`, e.g. Databricks Unity Catalog, Dataplex,
Purview, Sigma) *and* generic **JDBC** (this skill). The native path is usually richer
(lineage, finer metadata) and preferred where it exists.

**Whenever the user asks to create an integration, before doing anything else, determine
which ingestion type it should be** — don't assume JDBC just because this skill is loaded:

1. Identify the data source.
2. Check whether more than one path supports it (consult `collibra/etl-integration`'s
   `references/index.md` for the native-ETL data sources).
3. **If more than one path is possible, ask the user which they want** before starting.
   Only proceed with this JDBC skill once the user has chosen JDBC (or JDBC is the only
   supported path).

## The full sequence

1. **`edge_list_sites`** — get the `edgeSiteId` you'll use everywhere below.
2. **`get_data_source_setup_guide`** (pass the data source name, e.g. `"Snowflake"`) —
   before guessing any connection parameter. Returns the driver class, connection
   string format, the Marketplace URL for the driver jar, and per-auth-method
   properties. Treat a failed/empty lookup as "ask the user for the connection
   properties directly," not as a bug to retry.
3. **`upload_file`** — upload the JDBC driver jar. **Only use a file the user actually
   provides, sourced from the Collibra Marketplace listing from step 2** — never fetch
   a driver yourself from anywhere else, and never invent one. If the file is too large
   for `contentBase64` (most JDBC drivers are tens of MB) and this chip instance
   doesn't have `filePath` enabled, stop and guide the user to create the connection
   manually in the Edge UI instead — see "Large driver files" below.
4. **`edge_create_connection`** — `typeId: "Generic"` (not a vendor-specific connection
   type — jdbc-ingestion requires the Generic JDBC connection type). Fixed manifest
   parameters (`driver-class`, `connection-string`, `driver-jar` as the `upload_file`
   URI) go in `parameters`; open-ended vendor properties (Snowflake's Role, Warehouse,
   User, private key file, etc.) go in `additionalProperties`, not `parameters`.
5. **`test_connection`** — verify it actually connects before building on top of it.
   Pass `timeoutSec` for a synchronous result, or poll the returned `jobId` with
   `edge_get_job_status`.
6. **`edge_create_capability`** — `typeId: "jdbc-ingestion"`, `parameters.connection` set to
   the connection id from step 4.
7. Catalog scaffolding, only if it doesn't already exist: **`create_community`** →
   **`find_domain_types`** (look up e.g. "Physical Data Dictionary") →
   **`create_domain`**. **`find_users`** to resolve owner names to UUIDs for the next
   step.
8. **`register_database`** — discovers the database through the Edge connection and
   registers it as a Database asset. Needs `communityId`, `parentSystemId` (an existing
   System asset), and `ownerIds` (from step 7). **Never default to registering
   whichever database is discovered first if more than one exists — always confirm
   which one with the user.** The tool refuses to guess when there's more than one
   database (see "Confirm scope" below). If it returns a retryable-looking error, it
   usually means the underlying async refresh hasn't completed yet (cold-start data
   source connections can take 30–60s) — just call it again; it's idempotent.
9. **`configure_database_schemas`** — takes the `databaseConnectionId` from step 8's output,
   discovers its schemas, and sets which schemas/tables get synchronized. **Never
   default to "sync everything" — always confirm which schemas and which tables with
   the user first.** The tool refuses to guess when there's more than one schema. This
   is a separate call from step 8 specifically so that a schema-ambiguity error here
   never risks colliding with the database step 8 already registered — safe to call
   again later to change what's synced.
10. **`start_ingestion`** — triggers the actual sync using the database id from step 8.
11. **`get_job_status`** — poll the job id `start_ingestion` returned. A
    202/success from `start_ingestion` only means the job was accepted, not that
    ingestion finished — always poll to confirm completion.

Once ingestion has completed, **technical lineage** can be layered on top of the
registered database — see `collibra/tech-lineage`.

## Two job-id spaces — do not cross them

- **Edge-site jobs** (from `test_connection`) → poll with **`edge_get_job_status`**.
- **DGC jobs** (from `start_ingestion`) → poll with **`get_job_status`**.

Passing one tool's job id to the other will fail or silently poll the wrong resource.
`start_ingestion`'s own `Job.id` is a catalog job id — never poll it with
`edge_get_job_status`.

## Do not use a raw "run capability" tool for this

If this chip instance also exposes a generic "run capability" tool (e.g.
`edge_run_capability`, which wraps Edge's `POST /capabilities/{id}/run` directly):
**never use it to trigger a jdbc-ingestion run.** Always use `start_ingestion` instead.
`start_ingestion` goes through DGC's `synchronizeMetadata` endpoint, which does the
catalog database-registration bookkeeping (linking the job back to the Database asset,
updating sync state) that a raw capability run skips entirely — a direct run would
appear to succeed on the Edge side while leaving the catalog side out of sync.

## Confirm scope before registering and configuring the database

`register_database` and `configure_database_schemas` will not silently register the wrong
database or sync every table in every schema — both refuse ambiguity rather than guess,
and neither has a side effect until its own ambiguity is resolved:

- **`register_database`'s `databaseName`** is required if the data source exposes more
  than one database/catalog through the connection. The tool errors and names the
  discovered candidates if you omit it while more than one exists — no Database asset
  is registered until this is resolved.
- **`configure_database_schemas`'s `schemaNames`** is required if the database has more than
  one schema. Same behavior: omit it with multiple schemas discovered and the tool
  errors, listing them. Pass the literal `["*"]` to configure every schema — only do
  this after the user has actually said they want everything, not as a shortcut when
  you don't know yet.
- **`configure_database_schemas`'s `include`** has no default. You must always pass a table
  pattern (`"*"` for all tables, or a specific comma-separated pattern) — there is no
  silent "sync everything" fallback.

The practical flow for each tool: call it once with just the required identifiers and
let it fail on ambiguity to discover the candidates, **or** if you already know
multiple exist, ask the user up front before calling at all. Either way, do not pass
`"*"` for `schemaNames` or `include` on your own initiative — confirm with the user
first. The one exception: if discovery finds exactly one database (or one schema), the
respective tool auto-selects it — no ambiguity to resolve, so no need to ask about
*which* one, though you should still confirm the table pattern (`include`/`exclude`).

## Picking up an existing connection instead of duplicating one

If a connection with the driver already exists (most commonly because the driver file
was too large to upload and the user created it manually via the Edge UI), use
**`edge_find_connections`** (by name, optionally scoped to `edgeSiteId`) to get its id and
skip straight to step 6 — do not call `edge_create_connection` again for it.

## Large driver files

`upload_file` takes file content as base64 in a tool call, which has a hard ceiling
that isn't about encoding — the content has to be generated by the calling model
itself, so a large jar (tens of MB) simply won't fit. Some chip instances (the
standalone `chip` binary, not chip-service) also accept a `filePath` input to read
the file directly off local disk instead. If neither `filePath` nor a small-enough
file is available: guide the user to create the whole connection manually via the Edge
UI (Settings > Edge > site > Connections > New), giving them the exact field values
(driver class, connection string — from `get_data_source_setup_guide`) so they only
need to pick the file and click through. Once they confirm it's saved, use
`edge_find_connections` to pick it up and continue from step 6.

## Hard rules

1. **Never source a JDBC driver from anywhere but Collibra Marketplace.** If in doubt,
   ask the user to download the jar from the URL `get_data_source_setup_guide` returns
   and hand you the file — don't fetch or substitute one yourself.
2. **`edge_create_connection`'s `typeId` for jdbc-ingestion is `"Generic"`,** never a
   vendor-specific connection type — jdbc-ingestion cannot use those.
3. **Always poll after `start_ingestion`.** A successful call only means the job was
   accepted.
4. **Never pick a database, schema set, or table scope for the user.** When more than
   one is possible, ask; when `register_database` or `configure_database_schemas` reports
   ambiguity, relay the candidates and ask rather than picking one yourself or
   retrying with `"*"`.
