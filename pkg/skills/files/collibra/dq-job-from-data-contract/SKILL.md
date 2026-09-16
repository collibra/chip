---
description: Translate a Collibra Data Contract into a live Data Quality job plus the contract-enforcing rules (monitors) that check its manifest clauses against the real data. Use this whenever a user wants to "enforce", "monitor", "check", or "test" a data contract, stand up DQ / data-quality monitoring for a data product or contract, turn a contract's rules (not-null, unique, freshness, allowed values) into actual checks, or verify that the table behind a contract actually meets its promises. Trigger even when the user only names the data product, contract, or governed table and asks to "make sure the data is good" or "add quality checks" — the contract manifest is the source of truth for what to enforce. Do NOT use this to author the data contract or data product itself (that is collibra/data-product-create), or for free-standing DQ rules with no contract behind them (that is collibra/dq-rule-workbench / collibra/dq-rules).
related: collibra/dq-rules, collibra/dq-rule-workbench, collibra/data-product-create
---

# DQ job from a data contract

Turn a Collibra **Data Contract** into an operational **Data Quality job** and the
custom **rules** (Collibra calls them "monitors") that enforce the contract's
promises against the actual data. The contract's *manifest* is the specification;
this skill compiles that spec into a running check on the source table.

The flow is: read the contract → resolve its table and connection → reuse or create
the job on that connection → add tolerance-0 rules mapped one-to-one from the
manifest clauses → run and read results. Phases 1–2 are read-only discovery; the
job and rules are writes, each gated behind a confirm checkpoint.

This skill sits on top of two others and defers to them for mechanics:
`collibra/dq-rules` (the validate → create → read lifecycle of a single rule) and
`collibra/dq-rule-workbench` (authoring rules at scale). Load `collibra/dq-rules`
before creating rules if you have not already.

## Hard rules

1. **The manifest is the source of truth for what to enforce.** Every *contract*
   rule must trace back to a clause in the contract manifest (a property's
   `required`, `unique`, `primaryKey`, `classification`, an allowed-values list, or
   an SLA). Never silently invent a constraint the contract does not state — the
   consumers who trust the contract would be misled by a check enforcing something
   they never agreed to. It is fine — encouraged — to *offer* extra coverage beyond
   the contract (see Phase 3), but the user must opt in, and such rules are labelled
   supplementary, not contract-derived.
2. **Match the job's connection to the manifest's `servers` block.** `create_data_quality_job`
   auto-resolves a connection from the catalog table, and that resolved connection
   often does **not** match the platform the contract actually describes (a
   contract whose manifest server is Snowflake can auto-resolve to an Athena or
   other connection pointing at a copy). Read the manifest `servers` entry
   (type/host/database/schema) and steer discovery to the connection that matches
   it, so the checks run against the data the contract governs.
3. **Prefer reusing an existing job on the table.** Before creating anything, check
   whether a DQ job already scans this table (Phase 2). If one does and its schedule
   already matches the contract's cadence, adding the contract's rules to that job is
   more efficient — one execution covers both — so offer that instead of a second
   job. Only create a new job when none exists or the schedules genuinely conflict.
4. **Confirm before each write.** Both `create_data_quality_job` and
   `create_data_quality_rule` default to a preview (`confirm` omitted/false) that
   writes nothing. Do discovery and mapping silently; surface one clear proposal,
   take one approval, then execute with `confirm: true`.
5. **The auto-monitors are not the contract.** Creating a job auto-generates adaptive
   monitors (NULL / EMPTY / UNIQUENESS / DATA_TYPE per column) that **observe what is
   normal for the data and flag drift from that baseline** — they learn, they don't
   assert. They must never stand in for the contract's clauses, which are *explicit*
   promises. Enforce each clause with its own `FREEFORM_SQL` rule at `tolerance: 0`
   (zero breaking records tolerated), and tell the user why the explicit rule is not
   redundant with the auto-monitor that appears to cover the same column.
6. **Descriptive statistics is opt-in.** The `descriptiveStatistics` monitor computes
   min/mean/max/etc. but sets `maskSensitive=false`, so it **exposes raw values** —
   dangerous on PII. Do not enable it by default. Ask the user whether they want it;
   only if they explicitly say yes, pass `acknowledgeDescriptiveStatistics: true`.
   If the manifest classifies any column as PII, spell out that risk when you ask.
7. **Validate rule SQL when the platform allows; otherwise lean on the preview.** Per
   `collibra/dq-rules`, `validate_data_quality_rule` needs `edgeSiteId` +
   `connectionId` from `prepare_create_data_quality_job` (not exposed on every
   instance). When available, validate before creating. When not, use
   `create_data_quality_rule`'s own `confirm=false` preview as the checkpoint, say so,
   and keep the SQL simple enough to reason about. **On PULLUP jobs, see the PULLUP
   note in Phase 3 — rule preview/validate may not be possible through these tools at
   all.**
8. **Cross-table rules are not supported yet.** A rule's SQL must reference only the
   job's own dataset (`@<jobName>`) — never a `JOIN` to another table, another job's
   dataset, or a lookup table, and never a subquery against a second table. This rules
   out referential-integrity checks (a foreign key against a parent table) and any
   other clause that requires comparing rows across tables, whether the other table
   is outside the contract or is a sibling table governed by the same multi-table
   contract. If a manifest clause needs that, tell the user it cannot be enforced as a
   DQ rule through this skill today and skip it — do not approximate it with a
   single-table rule that silently drops the cross-table check it was meant to
   perform.

## Phase 1 — Read the contract

Start from whatever the user gives (a Data Contract, a Data Product, or a governed
table) and end with the manifest plus the table(s) to check.

- If you have the Data Product or Port, walk to the contract: the Data Contract
  `governs functioning of` the Port, and the Port `is implemented as` the table(s).
  `get_asset_details` bridges these relations.
- Pull the manifest with `pull_data_contract_manifest` (the manifest `id` equals the
  Data Contract UUID). This is an ODCS document — read three things from it:
  - **`servers`** — the platform/connection the job must target (rule 2).
  - **`schema[].properties[]`** — per-column clauses to enforce (see the mapping
    table below).
  - **`slaProperties`** — especially `processingFrequency` (→ the job's run cadence)
    and `recency` / `latency` (freshness expectations).
- Capture the qualified table location (`database.schema.table`) and the column
  list. If the manifest covers several tables, one DQ job is created per table (a
  job scans exactly one table).

## Phase 2 — Reuse or create the DQ job

**First, look for an existing job on this table (rule 3).** A table that already has
DQ coverage shows a non-zero monitor count on its catalog Table asset
(`get_asset_details`), and `find_data_quality_rules` filtered by the table's columns
returns the owning `jobName`. `create_data_quality_job` discovery also reveals it —
it auto-names the next free job for a table (`…_1`, `…_2`), which increments when a
job already exists.

- **If a job exists and its schedule matches the contract cadence:** skip creation
  and go straight to Phase 3, adding the contract rules to that job. Confirm the
  match with the user first (name the existing job and its schedule).
- **If a job exists but the schedule differs:** tell the user, and let them choose —
  accept the existing cadence, or create a separate job on the contract's cadence.
- **If no job exists:** create one with `create_data_quality_job`.

**Precondition — the connection must be DQ-enabled and you must have DQ access; fail
gracefully if not.** A DQ job can only run on a connection whose Collibra **Edge**
capability includes **Data Quality** — a plain catalog/ingestion connection won't do.
If the manifest's server has no matching **DQ-enabled** connection (it doesn't appear
among the connection options, or discovery/creation is rejected for that reason), stop
and tell the user plainly: a **Data Quality capability must be added to that connection
in Edge** via the Collibra UI (an admin action this skill cannot perform) before a job
can be created. Distinguish that from an **access** problem: a `401`/`403`, a "no DQ
license", or a missing-role error means the *user* lacks a **DQ license or the required
DQ roles/permissions** — say so explicitly and route them to their Collibra
administrator. In either case do not silently fall back to a non-DQ connection or keep
retrying.

When creating:

- **Steer the connection (rule 2).** Pass `connection` = the connection matching the
  manifest's server, then answer `dataSourceName` / `schemaName` / `tableName` as it
  walks discovery. Don't accept an auto-resolved connection on a different platform.
- **Schedule from the SLA.** Map `processingFrequency` to `scheduleRepeat`
  (e.g. `daily` → `DAILY`). A contract promising daily freshness should be checked
  daily.
- **Scan only the latest data with a time slice.** By default every run rescans the
  *whole* table — wasteful, and rarely what the user wants: on a table that grows
  over time they care about the rows that just landed, not the entire history. Set
  `timeSliceColumn` to a real date/timestamp column so each run checks only that
  window's new rows, and size the window with `timeSliceSize` / `timeSliceUnit` to
  match the contract's cadence (a daily `processingFrequency` → a 1-day slice). Pair
  it with `scheduleRepeat` so scheduled runs are incremental. Two constraints: (a)
  the slice column must be a **genuine date/timestamp column that records when the row
  was created or loaded** — an ingest / `created_at` / `loaded_at` / effective-load
  timestamp. A **business** date is the wrong choice even when it's a real timestamp:
  date of birth, hire date, order date, or patient-checkout time describe the *event*,
  not when the row landed, so slicing on one would scan the wrong rows and silently
  miss newly-loaded records. And a cast on a non-date column is not supported, so a
  table with no genuine load timestamp cannot be time-sliced at all (the EMPLOYEES
  example had none — its only date field, `DATE_OF_HIRE`, is both a business date and
  stored as VARCHAR); (b) **offer it, don't impose it** — a first baseline run, a full
  reconciliation, or a small static table may legitimately want a full scan, so
  propose the slice and let the user confirm the window.
- **Monitors.** The defaults (`rowCount`, `nullValues`, `emptyFields`, `uniqueness`)
  are a fine baseline, but remember rule 5 — they don't enforce the contract.
  Descriptive statistics stays off unless the user opts in (rule 6).
- **jobType** is detected from the connection: **PUSHDOWN** (checks run inside the
  source DB) or **PULLUP** (data pulled into Spark). This choice governs whether you
  can add rules through these tools — see the PULLUP note in Phase 3.
- Preview (`confirm` omitted), show the resolved location + schedule + monitors, get
  approval, then `confirm: true`. Leave `jobName` empty for an auto-assigned name and
  note it for Phase 3.

## Phase 3 — Add contract-enforcing rules

Add one explicit rule per enforceable manifest clause.

> **PULLUP note (check this before authoring rules).** Custom rule create/validate
> through these tools is oriented to PUSHDOWN jobs; on a non-PUSHDOWN dataset
> `create_data_quality_rule` returns HTTP 422 (`collibra/dq-rules`). A PULLUP job
> needs an active interactive Spark session to preview/validate a rule, and **these
> DQ tools expose no way to open or attach that session** (there is no session
> parameter, and `validate_data_quality_rule` only takes connection IDs). **So on a
> PULLUP job the agent cannot author the contract rules — the *user* must write them
> themselves in the Collibra DQ UI** (which establishes the session). Do not attempt
> the create or retry the 422. Instead, hand the user the exact rules to add — names,
> SQL, tolerance, business-friendly descriptions, and dimensions — so they can paste
> them in directly. This skill can still create the job and read results; only rule
> authoring is the user's to do on a PULLUP job.

1. **Check for duplicates.** `find_data_quality_rules` with `jobName` (+ `columnName`)
   so you don't re-create a rule the job already has (especially important when
   reusing an existing job under rule 3).
2. **Map each clause to SQL.** A rule is a query that *selects the breaking records* —
   the rule fails when that query returns rows beyond `tolerance`. Reference the
   dataset in `FROM` as `@<jobName>`.

   **These rows are representative patterns, not the full set of what you can build.**
   The rule space is wide but bounded to a single table (rule 8): `FREEFORM_SQL`
   accepts any query that returns the breaking rows *from the job's own dataset*, so
   you can express far more than the table below — cross-column consistency, regex,
   conditional/required-if logic, and aggregate thresholds all reduce to "write the
   query that finds the violations." You also have `SIMPLE_SQL` (a single-column
   predicate), rule **templates** (`deploy_data_quality_rule_template`, parameterized
   patterns applied in bulk), and `generate_data_quality_rule_sql` (plain-language →
   SQL) when you'd rather not hand-write it. What you cannot express is anything
   requiring a second table (rule 8) — most notably referential integrity. Pick
   whichever expresses the clause most clearly; the mappings below just cover the
   common ODCS clauses.

   | Manifest clause | Rule intent | `FREEFORM_SQL` (`monitorValue`) | dimension (map to one already configured on the instance) |
   |---|---|---|---|
   | `required: true` (not-null) | column never null | `SELECT * FROM @<job> WHERE <col> IS NULL` | Completeness |
   | `unique: true` / `primaryKey: true` | no duplicate values | `SELECT <col> FROM @<job> GROUP BY <col> HAVING COUNT(*) > 1` | Uniqueness |
   | allowed-values / enum in manifest | value must be in set | `SELECT * FROM @<job> WHERE <col> NOT IN ('a','b','c')` | Validity |
   | documented format/pattern | value matches shape | `SELECT * FROM @<job> WHERE <col> IS NOT NULL AND <col> NOT LIKE '<pattern>'` (or `NOT RLIKE '<regex>'`) | Validity |
   | numeric/date range or bound | value within limits | `SELECT * FROM @<job> WHERE <col> < <min> OR <col> > <max>` | Validity |
   | cross-column consistency | fields agree | `SELECT * FROM @<job> WHERE <end_col> < <start_col>` | Consistency |
   | conditional / required-if | dependent field present when applicable | `SELECT * FROM @<job> WHERE <when_col> = '<x>' AND <then_col> IS NULL` | Completeness |
   | referential integrity | value exists in parent | **Not supported (rule 8) — cross-table rules aren't available.** Do not author a `JOIN` to a parent table. Tell the user this clause can't be enforced as a DQ rule today and skip it. | — |
   | **schema conformance** (expected columns + types the contract declares) | live schema matches the contract | `SELECT column_name, data_type FROM INFORMATION_SCHEMA.COLUMNS WHERE table_schema = '<schema>' AND table_name = '<table>' AND (column_name, data_type) NOT IN ( ('<COL1>','<TYPE1>'), ('<COL2>','<TYPE2>'), … )` — flags any column whose name/type isn't what the manifest's `schema[].properties[]` declares | Conformity |
   | `recency` / `latency` freshness SLA (needs a genuine load timestamp) | newest row is within the SLA window | `SELECT MAX(<ts_col>) AS latest FROM @<job> HAVING TIMESTAMPDIFF('hour', MAX(<ts_col>), CURRENT_TIMESTAMP()) > <sla_hours>` — returns a row (breaks) only when the freshest record is older than the SLA allows | Timeliness |

   Only write allowed-values / format / range rules when the manifest (or the user)
   actually supplies the values — never guess them from a column name (rule 1).

   **Schema conformance vs. schema-change monitors.** The schema-conformance rule
   compares the *live* columns and types against the **contract's declared**
   `schema[].properties[]` — it breaks when reality drifts from the promise. This is
   deliberately different from a job's built-in schema-change / `DATA_TYPE` monitors,
   which only flag a change *from the previous run* and know nothing about what the
   contract expects. Enforcing the contract needs the conformance rule; keep the
   change monitors too if you also want drift alerts. (Build the expected-columns list
   straight from the manifest's declared column names and physical types.)

   **Freshness / SLA enforcement — be explicit about which SLA fields become data
   rules.** Most ODCS `slaProperties` are *operational* commitments, not table checks:
   `uptimePercentage`, `retentionPeriod`, `supportAvailability`,
   `recoveryTime`/`recoveryPoint`, `responseTime`, and `backupFrequency` are honored
   through the platform, the job schedule, and notifications — **do not** try to encode
   them as a SQL rule, and tell the user they remain operational. The SLA fields that
   *do* translate into data rules are the freshness ones:
   - `recency` / `latency` → the max-timestamp freshness rule above. It does **not**
     mean "every row's timestamp is before now" (that's trivially true). It means:
     take the **newest** timestamp in the table and check that *now minus that value*
     is within the SLA. Convert the SLA to one unit (`24 hours`, `2 days` → hours) and
     use it as `<sla_hours>`. The check needs a genuine **load** timestamp (a business
     date won't do — see the Phase 2 time-slice note). If the table has no load
     timestamp, freshness cannot be enforced as data — say so and fall back to the job
     `scheduleRepeat` from `processingFrequency`, which controls *how often you look*,
     not *how fresh the data is*.
   - `processingFrequency` → the job **schedule** (Phase 2), not a rule.
   State plainly which SLA promises you turned into rules and which stay operational,
   so no one assumes a `retentionPeriod` or `uptime` SLA is being enforced when it
   structurally can't be.

   **Watch how a time slice (Phase 2) interacts with whole-table clauses.** If the
   job is time-sliced, each run only sees the latest window — fine for *per-row*
   clauses (not-null, format, allowed-values: every row is judged on its own), but a
   problem for clauses that must hold across the **entire** table. The clearest case
   is `unique` / `primaryKey`: a within-slice duplicate check can pass while a true
   duplicate sits in an earlier window it never scanned. For such clauses, either run
   them on a full (unsliced) pass, or tell the user plainly that uniqueness is being
   verified within the slice only. Don't let a time slice silently weaken a
   whole-table contract promise.
3. **Give every rule a meaningful name and a business-friendly description.** The
   `monitorName` (`<col>_not_null`, `<col>_unique`, `<col>_format_valid`) is how
   results are read later. The `description` should be **plain-language and
   business-friendly** — written for the person reading the findings, not the SQL
   author, so they instantly understand what is being evaluated and why it matters.
   Prefer "Every employee record must have an email address on file" over "NOT NULL
   check on EMAIL". Set `tolerance: 0`.

   **Use only the data-quality dimensions already configured on the instance — never
   invent one.** The dimension labels in the table above (Completeness, Uniqueness,
   Validity, Timeliness, Conformity, …) are *indicative*; the real valid set is
   whatever the instance defines in its DQ settings. Discover it from what's already in
   use — `list_data_quality_rule_templates` and existing rules via
   `find_data_quality_rules` both report the dimensions in play — and pick the closest
   existing match. If none fits, leave `dimensions` unset rather than passing a new
   label; adding a dimension is a settings change that is the user's/admin's call, not
   the agent's.
4. **Offer supplemental coverage (opt-in).** Beyond the strict contract clauses, it is
   good to *suggest* additional checks the data plausibly warrants — an email-format
   sanity check, a range check on an amount, a cross-column consistency check — and
   let the user opt in. Present them clearly as **supplementary to the contract**, not
   derived from it (rule 1), and only create the ones the user accepts. Keep
   suggestions single-table (rule 8) — don't offer a referential/cross-table check as
   supplemental coverage either, since it can't be created regardless of who asked
   for it.
5. **Validate if possible, else preview** (rule 7 and the PULLUP note). Then preview
   each rule (`confirm` omitted) and, once approved, create with `confirm: true`.
   Batch the creates; apply partial-success handling — a failure on one rule does not
   abort the rest — and report created / skipped / failed with reasons.

## Phase 4 — Run and read results

- **There is no on-demand "run job" tool.** A newly created job runs once *at
  creation* — which happens **before** the Phase-3 rules exist, so those rules have
  no results yet. Rules are evaluated on the job's **next** run: the scheduled run, or
  a run the user triggers manually in the DQ UI. Tell the user this and, if they want
  results now, ask them to hit Run in the UI (or offer a scheduled follow-up that
  reads results after the next run).
- **Read with `get_data_quality_rule_results`** (`jobName` + `ruleName`). Each entry is
  one run: `passFail`, `breakMsg` (PASSING / BREAKING), `breakingRecords`,
  `passingRecords`, `totalCount`, `score`. Report pass/fail per rule and flag any
  BREAKING rule as a contract violation.
- **Results give counts, not rows.** These tools return how many records broke, not
  the offending values. Where to find the actual rows depends on the connection:
  - **If the connection archives break records** (a DQ connection-level setting; not
    configurable or readable through these tools), the breaking rows are persisted to
    the configured break-records store and are viewable/downloadable from the run's
    findings in the DQ UI (`/data-quality/jobs?jobName=<job>`). Point the user there
    and, if you know it, name the break-records location.
  - **If archiving is off,** only the counts exist after the fact; suggest the user
    enable break-record archiving on the connection if they need row-level detail on
    future runs.

## Guardrails and stop conditions

These protect against the failure modes that matter most when writing governance
objects against a live instance. Check them as you go; several are natural stopping
points where you pause and ask rather than push ahead.

- **DQ-enablement & access (precondition).** If the target connection has no Data
  Quality Edge capability, or the user lacks a DQ license / the required DQ roles, stop
  and route them to the Collibra UI or their admin (Phase 2) — don't fall back to a
  non-DQ connection or retry.
- **Dimensions are instance-defined.** Only use DQ dimensions already configured on the
  instance; never pass a new dimension label (Phase 3 step 3).
- **Environment guard.** Never create or modify a job/rule against a **production**
  connection without the user explicitly confirming the environment. If the resolved
  connection or manifest server looks like prod (naming, host), say so and get a clear
  go-ahead first.
- **Idempotency — don't duplicate what's already enforced.** In the reuse case
  (rule 3), a job may already carry the contract's rules. Always run the duplicate
  check (Phase 3 step 1); if a clause is already covered by an equivalent active rule,
  report it and **add nothing** rather than creating a near-identical monitor. "The
  contract is already fully enforced" is a valid, good outcome.
- **Contract status.** A manifest can be `candidate`/draft rather than active. Surface
  the status and confirm this is the version the user wants to enforce before writing
  rules from it — enforcing a draft contract can be premature.
- **Schedule verification.** These tools cannot read an existing job's schedule, so
  when reusing a job (rule 3) you cannot prove its cadence matches the contract SLA.
  Don't assert that it does — ask the user to confirm the schedule in the DQ UI, and
  treat a mismatch as a decision point (accept, adjust, or separate job).
- **Permissions pre-flight.** Rule/job creation needs permission on the target job;
  a `403`/`422` is a hard stop for that object, not something to retry. Report it
  plainly and continue with whatever else is permitted (partial success, Phase 3
  step 5).
- **PII exposure.** Beyond descriptive statistics (rule 6), be conscious that any
  breaking-record sample or preview could surface personal data; keep sensitive
  values out of chat and point to the DQ UI for row-level detail.
- **No invented enforcement.** Restating rule 1 as a stop condition: if you cannot
  trace a proposed check to a manifest clause or an explicit user opt-in, do not
  create it.
- **Cross-table rules.** Restating rule 8 as a stop condition: never author a rule
  whose SQL joins or subqueries a second table (referential integrity being the most
  common case) — not even when the "parent" is another table in the same multi-table
  contract. Tell the user that clause isn't enforceable through this skill today and
  move on; don't retry with a workaround.

## When this skill does not apply

- Authoring the Data Contract / Data Product itself → `collibra/data-product-create`.
- DQ rules with no contract behind them, or bulk rule authoring across many columns →
  `collibra/dq-rule-workbench` / `collibra/dq-rules`.
- Editing an existing job's rules or reading results only → the individual DQ tools
  directly (`find_data_quality_rules`, `get_data_quality_rule`,
  `get_data_quality_rule_results`).

## Reference material

- `references/examples.md` — worked prompt → ideal-response transcripts (full flow,
  the connection-mismatch trap, an already-enforced contract, and the no-timestamp
  time-slice case). Read it when you want a concrete picture of what "good" looks
  like end to end; it is illustrative, not a substitute for the phases above.
