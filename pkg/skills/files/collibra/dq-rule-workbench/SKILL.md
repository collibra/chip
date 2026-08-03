---
description: Author data quality rules at scale against catalog columns — target columns, check for duplicates, define rules via templates or plain-language SQL, assign or create a job, and deploy in bulk with a partial-success model.
related: collibra/dq-rules, collibra/discovery, collibra/asset-edit
---

# Data quality rule workbench

A multi-turn flow for creating DQ rules against one or more **catalog columns** at
scale — from templates (bulk) or plain-language intent (Text2SQL) — without the
user writing SQL by hand. This skill orchestrates existing tools; it does not add
new API surface.

Relationship to the other DQ skills:
- **`collibra/dq-rules`** — the mechanics of a single rule on an existing job
  (validate → create → inspect → read results). This workbench reuses those
  rules and defers to that skill for the per-rule detail.
- **Job creation** — when a column has no suitable job, this flow calls
  `prepare_create_data_quality_job` + `create_data_quality_job` (see their tool descriptions).

## Tools this flow orchestrates

- **Target columns**: `search_catalog_columns` (metadata filters — description,
  data type, data-steward role, and relations to a business term / business rule /
  data element / data attribute; needs the Knowledge Graph API), plus
  `search_asset_keyword` (domain/community/asset-type + free-text),
  `discover_data_assets` (natural-language), `get_asset_details` (by UUID).
- **Resolve DQ location / job + detect PUSHDOWN**: `prepare_create_data_quality_job`
  (resolves a catalog Table asset → connection / edge / job and reports the job
  type). `create_data_quality_job` when none exists.
- **Duplicate detection**: `find_data_quality_rules` (filter by `jobName` + `columnName`).
- **Define rules** — two paths:
  - Templates: `list_data_quality_rule_templates` / `get_data_quality_rule_template` →
    `deploy_data_quality_rule_template` (bulk).
  - Plain-language / AI: `generate_data_quality_rule_sql` (Text2SQL) → `create_data_quality_rule`.
- **Review SQL**: `validate_data_quality_rule`.
- **Read results**: `get_data_quality_rule_results` (per-run rule outcomes, once the job has run).
- **Catalog associations**: `edit_asset` (`add_relation`) to Business Rule, Data
  Element, Data Attribute, or a catalog Data Quality Rule asset.

## Hard rules

1. **Confirm at every checkpoint before any write.** The user must explicitly
   approve (a) the confirmed column list, (b) the job assignment per group, and
   (c) the final rule set. Never create, deploy, or run without that confirmation.
2. **Cap at 25 columns per invocation.** If targeting yields more, present the
   top 25, state how many were excluded, and ask the user to refine.
3. **Rules require a PUSHDOWN, DQ-connected table.** Resolve each column's table
   with `prepare_create_data_quality_job` and confirm the job type is PUSHDOWN. Columns
   whose table is not connected to a DQ Pushdown source are excluded — silently
   for search results (report the count + reason), but a **hard error** if the
   user named such a column explicitly.
4. **Check for duplicates before creating.** For each targeted column call
   `find_data_quality_rules` with `jobName` + `columnName`. Show any existing rule (name,
   job, SQL) and have the user confirm-to-proceed or skip that column.
5. **Validate every rule's SQL before saving.** Run `validate_data_quality_rule` on each
   generated or edited SQL. Do not create a rule whose SQL is invalid. See
   `collibra/dq-rules`.
6. **A newly created job must complete at least one run before rules can be
   added.** When the job-assignment step creates a job via the sub-flow, tell the
   user about this dependency and confirm before proceeding; the new job runs
   once (create auto-runs) before its rules are added.
7. **Partial success.** A failure on one rule does not abort the batch. Continue,
   then report counts of created / skipped (with reasons) / failed (with reasons)
   and offer retry, modify, or skip for the failures.
8. **Permissions are per job.** If a `403` comes back for a job, exclude that
   job's columns with a clear message and continue with the rest.
9. **Metadata search via `search_catalog_columns` (needs the Knowledge Graph
   API).** It filters columns by description, data type, a data-steward role, and
   relations to a business term / business rule / data element / data attribute
   (by name), AND-combined — plus domain/community. Two caveats: **Classification
   Tags is not supported** (no KG predicate), and the tool errors if the KG API
   isn't enabled on the instance. When KG is unavailable, or for a
   classification-tag filter, fall back to `search_asset_keyword`
   (domain/community + free-text) or explicit column naming, and say so. A broad
   lone substring filter can hit the KG query timeout — combine filters.

## Conversational flow

Progress through these states; the user may revise within a state before moving on.

**1. Column targeting.** Two modes:
   - *Search*: use `search_catalog_columns` for metadata filters (description,
     data type, data-steward role, business term / rule / data element / data
     attribute), or `search_asset_keyword` for domain/community + free-text, or
     `discover_data_assets` for a natural-language ask. Apply rule 9 (KG
     availability, classification tags).
   - *Explicit*: the user names columns by qualified path (`schema.table.column`)
     or catalog asset name; resolve each with `search_asset_keyword` /
     `get_asset_details`.
   Enforce the 25-cap (rule 2) and the DQ/PUSHDOWN exclusion (rule 3).

**2. Column confirmation.** Present a summary per column: qualified path, parent
   domain/community, data type, existing-rule count (from `find_data_quality_rules`), and any
   duplicate flag. The user confirms or edits the list (remove columns / restart
   targeting) before advancing.

**3. Rule definition.** Pick one path for the whole confirmed list:
   - *Template*: `list_data_quality_rule_templates` (filter by dimension / OOTB), show the
     choices, let the user pick one. There is **no** template/data-type
     compatibility API, so incompatible combinations can't be pre-flagged — they
     surface as errors at deploy (step 7); warn the user of this.
   - *Plain-language / AI*: the user gives one intent; call `generate_data_quality_rule_sql`
     per column (it needs `edgeSiteId`/`connectionId` from step 5's resolution and
     the column list). Note Text2SQL returns a single SQL string — there is no
     separate filter/WHERE clause; if the intent implies scoping, author a
     `filterQuery` yourself and confirm it.

**4. SQL review & revision.** For every rule, show the SQL and let the user
   approve, edit SQL, revise the intent and regenerate, or skip the column. Run
   `validate_data_quality_rule` on each version before presenting (rule 5). Loop until the
   user approves or skips.

**5. Job assignment.** For each column resolve a job in priority order:
   1. *Exact match* — an existing job covering the column's table + connection.
   2. *Connection match* — a job on the same connection but not yet the table.
   3. *No match* — create one via `prepare_create_data_quality_job` + `create_data_quality_job`
      (rule 6 applies).
   Group columns by connection, resolve one job per group, and show the full
   grouping. The user confirms all assignments before any rule is saved (rule 1).

**6. Final confirmation.** Summarize what will be written, and present it by path —
   do not dump a flat list of template×column pairs:
   - *Template path*: show the chosen template **once** (its name and what it
     checks), then, grouped by job, the list of columns it will be applied to and
     the resulting rule name per column (`{template}_{column}`). One template
     header, then its columns — not a row per template/column combination.
   - *Plain-language path*: list one entry per rule — column, job, rule name, and
     the approved SQL (+ any filter) — since each rule's SQL can differ.
   Then get explicit approval.

**7. Execution.** Create the rules. Both write tools have a **confirm checkpoint**:
   call with `confirm` omitted to get a `preview` (nothing is written), then call
   again with `confirm: true` only after the step-6 approval. The tools enforce
   this — a `confirm=false` call never writes.
   - Template path: `deploy_data_quality_rule_template` with `targets` = the confirmed
     `{jobName, columnName}` list (bulk; rules named `{template}_{column}`).
   - Plain-language path: `create_data_quality_rule` per column (validated SQL as
     `monitorValue`; a meaningful `monitorName` — see `collibra/dq-rules`).
   Apply the partial-success model (rule 7), and report outcomes with job links.
   The rules are evaluated on each job's next (scheduled) run — this flow does not
   trigger runs. Once a job has run, read outcomes with `get_data_quality_rule_results`.

## Rule settings & catalog associations

Which settings you can set depends on the path:

- **Plain-language path (`create_data_quality_rule`)**: set `dimensions` (default none),
  `tolerance` (default 0), and `description` (default auto-generated from the
  intent) per rule, bulk-uniform unless the user asks for per-column values.
  **`tolerance` is a count of breaking records allowed before the rule fails, not
  a percentage** — despite the ticket's "Tolerance %", the API takes an integer
  record count. Say it that way to the user.
- **Template path (`deploy_data_quality_rule_template`)**: rules inherit the template's
  `dimensions`, `tolerance` and `description`. The deploy call takes only
  `{jobName, columnName}` targets, so these are **not** set per deploy — choose
  (or create) a template that already carries the settings you want.

**Notifications are job-level, not per-rule.** `create_data_quality_rule` cannot attach
notifications to a rule; notification recipients/triggers are configured when the
job is created (`create_data_quality_job` `notify*` fields) and apply to the whole job. Do
not tell the user a rule carries its own notification settings.

To link created rules to catalog assets — Business Rule, Data Element, Data
Attribute, or a Data Quality Rule asset — use `edit_asset` `add_relation` by role
name. If a Business Rule asset was used as a targeting filter, pre-populate that
association and confirm it with the user.

## Known limitations (state these when relevant)

- Column metadata search (`search_catalog_columns`) needs the Knowledge Graph API
  enabled, and does not support Classification Tags — see rule 9.
- Template/data-type compatibility is not pre-flagged (no API); incompatible
  deploys fail at execution.
- Text2SQL returns one SQL string, not a primary-SQL + filter split.
- Per-rule notifications are not supported — notifications are job-level only
  (`create_data_quality_job`). The ticket lists notifications as a rule setting; the tools
  do not.
- `tolerance` is a breaking-record count, not the ticket's "Tolerance %".
