---
description: Create a Collibra Data Product from an existing physical table (name, UUID, or URL). Discovers related tables, checks for overlap, collects governance metadata, proposes the full set of assets, and after one explicit confirmation creates the Data Product, a Port, and a Data Contract.
related: collibra/asset-create, collibra/asset-edit, collibra/discovery
---

# Creating a Data Product from a table

Packages an existing physical table — given as a name, DGC UUID, or Collibra URL — into a
Collibra Data Product with one Port and a Data Contract. Phases 1–5 are read-only discovery and a
proposal; Phase 6 is the single confirmation; Phase 7 writes. Generic tool mechanics — how to set
attributes, write Markdown, handle each tool's success/error responses, resolve relation names,
and recover from partial failures — live in `collibra/asset-create` and `collibra/asset-edit`;
this skill is the DP-specific layer on top.

## Hard rules

1. **Confirm once.** Do discovery silently, gather every required choice (included tables, domain,
   owner, SLAs) in as few turns as possible and **before** the overview, present one complete
   overview (Phase 5), take a single explicit approval (Phase 6), then execute all writes
   (Phase 7) with no further prompts. Never ask field-by-field; never re-confirm individual writes.
2. **Nothing is auto-included or auto-created without that approval.** Adding a table is an access
   decision: suggested tables default to *not included* and the user opts them in. Creating assets
   is significant and **not auto-rolled-back** — wrong assets must be deleted manually.
3. **One Port by default;** more only if the user explicitly asks.
4. **Fill every applicable attribute — the reference tables are the source of truth for which
   attributes each asset has.** Draft a proposed value for every attribute listed there; leave
   blank only the SLAs the user declined and `Pricing` without supplied cost. **SLA values are
   discrete Data Contract attributes** — never written into `Description` or the manifest.
5. **Existing coverage is a stop condition:** if a Data Product/contract already covers
   substantially the same tables, stop and offer to extend it instead.
6. **Say where inferred values come from.** When a value is drawn from another asset or otherwise
   inferred, tell the user in chat where it came from so they can judge it — the written attribute
   holds only the clean value, not provenance markers.
7. **Attribute writes** (per `collibra/asset-edit`): set a not-yet-valued attribute with
   `add_attribute`, not `update_attribute` (which only changes an existing value). If any name
   doesn't resolve on this instance, follow the error's suggestion.

## Phase 1 — Locate the table to package

Resolve the input to a UUID (UUID → use directly; URL → the UUID after `/asset/`; name →
`search_asset_keyword`, `resourceTypeFilters=["Asset"]`, let the user pick if multiple).
`get_asset_details` on the resolved UUID; if it is not table-like, say what it is and stop. From
that one response capture:

- **name** and existing description / sensitivity / refresh / acceptable-use attributes;
- the **parent schema** (the incoming `Schema` relation) — needed in Phase 3 to find related
  tables in the same schema;
- the **column list** (names + UUIDs, from the outgoing `Column` relations) — needed in Phase 2
  to check whether any column already belongs to a Data Set via the `includes data element`
  relation, and used throughout as **context to understand what the product is about**, so the
  generated attributes (Description, Business Case, Business Value, and the rest) are higher
  quality across every asset.

Capturing schema and columns here is free (same call). Defer the deeper per-column fetches —
reading each column's `includes data element` relation — to Phase 2, where the Data Set check runs.

## Phase 2 — Check existing coverage

Confirm the table to package is not already published as a data product, or included in a data set,
so we do not create a near-duplicate.

**Is it already in a Data Product?** Check whether the table is linked to a `Data Product Port`
(via `is implemented as`). If it is, look at how that Port relates to a Data Product: a Port connected to
the product through the `exposes data as` relation is an **output port** — which means a Data
Product already publishes this table.

**Is it in a Data Set?** Data Sets contain data elements — usually columns. Take a few of the
table's columns (from Phase 1; no need to check them all) and see whether any are linked as data
elements to a Data Set via the `includes data element` relation. If a Data Set matches, mention it
to the user and suggest proceeding — but let them decide.

**Report the result either way.** If the table is already published as a Data Product, stop
(rule 5): report its name and UUID and offer to extend the existing product instead of creating a
near-duplicate. If nothing covers it, say so explicitly ("No existing Data Product covers this
table"). Don't skip the all-clear.

## Phase 3 — Suggest related tables (optional)

Sometimes nearby tables in the same schema add value to the data product — several related tables
are often better packaged together as one product. This phase *suggests* such tables; it never adds
them.

**First, ask** whether the user wants the skill to look for additional tables to include. If they
only want that one table, skip this phase — a single-table product is perfectly valid.

**How to look.** If yes, search the same schema with `search_asset_keyword`, `query=<schema name>`
(the schema captured in Phase 1), pre-filtered by name and cost-capped. You're looking for other
tables in that schema that would add value for the product's consumers — for example a lookup or
reference table the packaged table points to, or a broader table it feeds into.

**What to report.** Present each candidate with plain-language reasoning for why it might belong,
and **note whether it is already part of another Data Product** (from its Port relations in the
fetched details) — e.g. "`dim_customer` is already in the *Customer 360* product." Make the
include/exclude choice **explicit**: do not pre-select, and do not move to the overview until the
user has chosen. If the packaged table plus the tables the user includes substantially match an
existing product, stop (rule 5). Carry only confirmed tables forward.

## Phase 4 — Collect governance metadata

Gather these in one batched turn, with sensible pre-filled proposals so the user can accept or
adjust (rule 1):

- **Target domain** — offer only domains that accept a Data Product (resolve via
  `prepare_create_asset`, `assetType: "Data Product"`); the user picks. Never default silently.
- **Owner / steward — always ask.** Explicitly ask the user who should own the product (a user or
  group). You may offer the requester as the default, but **present the question** for the user to
  confirm or change — never assign an owner silently. Set it in Phase 7 via `set_responsibility`
  (`role: "Owner"`/`"Steward"`, `userId` = email/username/UUID).
- **Access Method (Port)** — a single-select whose allowed values are instance-specific. Fetch
  them with `prepare_create_asset` (`assetType: "Data Product Port"`, chosen domain), propose the
  closest fit (for a physical table, typically `Table`), and let the user confirm — never assume a
  fixed value.
- **Category (Foundational vs Derived)** — ask the user: **Foundational** = exposes raw input data;
  **Derived** = built on other data products. Default to *Foundational* if unsure.
- **Sensitivity & freshness** — note classifications/sensitivity on the table and its columns, and
  carry the refresh/freshness attribute into the contract.
- **SLAs** — attributes on the **Data Contract** asset (full list in the reference tables below).
  Ask how to handle them: (a) **the skill proposes values** as a suggestion, shown in the overview
  for approval; (b) **define them now** with the user's values; or (c) **leave for later** (none
  set). Write only values the user provided or approved.

## Phase 5 — Propose

**Prerequisite:** do not present the overview until the table choice (Phase 3) and the
domain/owner/SLA choices (Phase 4) are made. The overview reflects decisions already taken.

State the **proposed Data Product name first, on its own line**, and invite a rename.

**Structure** — lead with the picture, marking any table carrying sensitivity (e.g. `⚠ PII`):

```
        ┌───────────────────────────┐
        │  DP: <Data Product name>   │   domain: <chosen domain>
        └────────────┬──────────────┘
                     │ exposes data as
        ┌────────────▼──────────────┐
        │  Port: <name>             │◄─ governs ─ Contract: <name>
        └────────────┬──────────────┘
                     │ is implemented as
      ┌──────────────┼───────────────┐
      ▼              ▼               ▼
 ┌───────────┐  ┌──────────────┐  ┌───────────┐
 │ <related> │  │ <the table>  │  │ <related>⚠│
 └───────────┘  └──────────────┘  └───────────┘
```

**Attributes** — for each asset (Data Product, Port, Data Contract), show the **proposed value**
for every applicable attribute; the reference tables are the source of truth for which attributes
each asset has, and this part supplies the values. For the contract, show the SLA values to be set
and note which are left unset.

**Manifest improvements** — list each specific improvement you intend to add to the generated
manifest so the user can approve or reject each one: generic data-quality rules (not-null on keys,
freshness/recency tied to the refresh attribute) and column classification/PII. **Only propose
`validValues`/accepted-value/range rules when you have a real source** (a Collibra classification,
column semantics, declared types, an allowed-values attribute) or the user supplies the values —
never guess valid values from a column name, **because the agent can't see the actual data and a
wrong constraint would mislead the consumers who rely on the contract.** State each rule's source.

## Phase 6 — Confirm (single gate)

The one deliberate stop (rules 1–2). Name every object and warn it persists and must be deleted
manually if wrong. **Put each item on its own line** for readability, and **link the tables** the
Port will include (`https://<instance>/asset/<uuid>`). For example:

> Creating in the **`<domain>`** domain:
> - 1 Data Product: **`<name>`** (rename now if you like)
> - 1 Port: **`<name>`**, including: [`<table>`](url), [`<table>`](url)
> - 1 Data Contract: **`<name>`** governing the Port
> - owner: **`<owner>`**
>
> These are live assets and must be deleted manually if wrong. Create them all now?

Only on a clear yes continue to Phase 7, and then ask for no further confirmation.

## Phase 7 — Create and verify

Execute in order without pausing (rule 1). No single API creates everything atomically; batch what
each call allows.

1. **Create the Data Product and Port** (`create_asset`, `domain` from Phase 4). **Leave `status`
   unset** so each asset takes the instance's default out-of-the-box status — do not set a status.
   Set Port attributes inline (`Access Method` is a selection list — match the picklist; on error
   pick the suggested value). Create only these two now; the Data Contract asset comes in step 3.
2. **Wire the Data Product and Port relations** (`edit_asset`): Data Product `exposes data as`
   Port; owner via `set_responsibility`; Port `is implemented as` each included table, batched. The
   tables must be linked to the Port before init (step 4) so the generated manifest covers them.
3. **Create the Data Contract asset and set its attributes** (`create_asset`,
   `assetType: "Data Contract"`, `domain` from Phase 4; leave `status` unset). Set `Description`
   plus the chosen or approved SLA attributes, each as its own attribute — using the correct value
   format (`<number> <unit>` for duration fields, e.g. `4 hours`; see the reference). Setting the
   SLAs now means the manifest generated at init (step 4) already includes them in the correct
   format. Set only the intended attributes; do not populate unrelated ones.
4. **Link the contract to the Port, then initialize it.** On the Data Contract, `edit_asset`
   `add_relation` `governs functioning of` → Port. Then `init_data_contract` with
   `governedAssetId: <Port UUID>` (`manifest` omitted). Because the contract already governs the
   Port, init **initializes that existing contract** (idempotent by port — no duplicate) and returns
   the base manifest, already reflecting the contract's SLA attributes and the Port's tables. The
   manifest `id` equals the Data Contract UUID. Do not hand-author the YAML — init produces the base.
5. **Improve the returned manifest and push as `0.0.2`** (init creates `0.0.1`). Add the
   Phase-5-approved improvements (data-quality rules, classification/PII) — the SLAs are already in
   the base manifest from step 3–4, so don't re-add them. **Never delete or rewrite init's sections**
   (`id`, `apiVersion`, `servers`, `schema`). Every added rule must be sourced (cite it) or
   user-approved — no invented constraints. `push_data_contract_manifest` (`version: "0.0.2"`,
   `active: true`).
6. **Verify and report — with links.** Re-read the Data Product and Port (`get_asset_details`) to
   confirm owner, `exposes data as`, the Port's `is implemented as` links, and the contract's
   `governs functioning of` landed. Report each created asset as a **clickable link**
   (`https://<instance>/asset/<uuid>`, instance host from the Phase 1 input) — for the Data
   Product, Port, and Data Contract — never bare UUIDs. Also report owner, domain, manifest
   version, and table count. Surface any failure verbatim; never restart earlier creates — tell the
   user exactly what exists and what is missing.

## Naming

- **Data Product** — human-readable from the table, e.g. `sales.customer_orders` →
  `"Sales Customer Orders"`.
- **Port** — name by whatever *distinguishes* it, never by a fixed/env-specific term:
  - **Single port (default):** use a neutral name that implies no direction — a port can also be
    consumed as an *input* by another product — e.g. `<Data Product name> — Port`.
  - **Multiple ports (only if the user asks):** distinguish by the chosen `Access Method`
    (`… — Table`, `… — API`, `… — UI`); if two share an access method, fall back to the platform
    or DB.schema (`… — Snowflake COLLIBRA.ODSS`).

Collibra allows duplicate asset names, but they cause problems downstream, so avoid them — choose a
clear, distinct name rather than reusing an existing one.

## Reference: DP-specific attributes and relations

Generic mechanics are in `collibra/asset-create` / `collibra/asset-edit`. One row per attribute;
set only values the user provided or approved (Data Contract SLAs especially).

| Attribute | Asset type | Type | Notes |
|---|---|---|---|
| Description | Data Product, Port, Data Contract | Rich text (Markdown) | narrative |
| Business Case | Data Product | Rich text | |
| Business Value | Data Product | Rich text | |
| Data Product Category | Data Product | Single-select | `Foundational` / `Derived` |
| Target Delivery Date | Data Product, Port | Date | |
| Access Method | Port | Single-select | instance-specific values — query them via `prepare_create_asset` (Data Product Port, chosen domain); do not hardcode |
| Access Instructions | Port | Rich text | |
| Area | Port | Plain text | |
| Pricing | Port | Plain text | only with supplied cost info |
| Uptime Percentage | Data Contract | Number | e.g. `99.9` |
| Unlimited Retention | Data Contract | Boolean | `true` / `false` |
| Retention Period | Data Contract | Plain text | e.g. `7 years` |
| Latency | Data Contract | Plain text | `<number> <unit>` |
| Recency | Data Contract | Plain text | `<number> <unit>` |
| Most Recent Record Date | Data Contract | Date | ISO date |
| Processing Frequency | Data Contract | Plain text | e.g. `daily`, `hourly` |
| Processing Method | Data Contract | Plain text | e.g. `batch`, `streaming` |
| Backup Frequency | Data Contract | Plain text | e.g. `daily` |
| Recovery Time | Data Contract | Plain text | `<number> <unit>` |
| Recovery Point | Data Contract | Plain text | `<number> <unit>` |
| Support Availability | Data Contract | Plain text | e.g. `Mon–Fri 9–5 CET` |
| Response Time | Data Contract | Plain text | `<number> <unit>`, e.g. `1 day` |

Duration/value SLA fields (`Latency`, `Recency`, `Retention Period`, `Response Time`,
`Recovery Time`, `Recovery Point`) are parsed into ODCS as `<number> <unit>` — first token is the
value, second word is the unit. Write `1 day`, not `1 business day`; `4 hours`, not `about 4 hrs`.

| Head | Role | Tail |
|---|---|---|
| Data Product | exposes data as | Data Product Port (output) |
| Data Product | consumes data through | Data Product Port (input) |
| Data Product | responsibility (Owner/Steward) | User / Group — via `set_responsibility` |
| Data Product Port | is implemented as | Table / Data Set / Column |
| Data Contract | governs functioning of | Data Product Port |

Whole **tables** only (column masking is handled later by access tools); glossary terms surface
automatically via derived relations — no explicit relation created here.

## When this skill does not apply

- Adding/replacing ports on an existing product → `edit_asset` directly (`collibra/asset-edit`).
- Updating a contract manifest for an existing product, no product to register →
  `push_data_contract_manifest` directly.
- Just an open question about what data exists → `collibra/discovery`.
