---
description: Author, validate, run and observe data quality rules (monitors) on an existing DQ job, using the create/get/edit/delete/validate/preview/run/status/log DQ tools.
related: collibra/discovery
---

# Data quality rules

A data quality **rule** (a "monitor") is a check attached to an existing DQ **job**
(a dataset). This skill covers the rule lifecycle — validate, create, read, edit,
delete — plus running the job and observing the outcome. It does **not** cover creating
the job itself.

Rule tools: `validate_dq_rule`, `preview_dq_rule_sql`, `create_dq_rule`, `get_dq_rule`,
`edit_dq_rule`, `delete_dq_rule`, `get_dq_rule_results`. Job tools: `run_dq_job`,
`get_dq_job_status`, `get_dq_job_log`.

## Hard rules

1. **Validate before you create.** Always call `validate_dq_rule` on the rule SQL *before*
   `create_dq_rule`. It checks the SQL against the source and returns `valid: true/false`
   plus a message, so a malformed rule (e.g. a bad `SIMPLE_SQL`/SQLG predicate) is caught up
   front instead of failing at run time. `validate_dq_rule` takes the raw SQL — the rule does
   **not** need to exist yet. If `valid` is `false`, fix the SQL and re-validate; do **not**
   create the rule.
2. **`validate_dq_rule` and `preview_dq_rule_sql` need discovery IDs.** Both require
   `edgeSiteId`, `connectionId` and `schemaName`. Get them from `prepare_create_dq_job` for
   the target job — do not guess them. (`create_dq_rule`, `get_dq_rule`, `edit_dq_rule`,
   `delete_dq_rule`, `get_dq_rule_results` and the job tools take only names/ids and need no
   discovery step.)
3. **`monitorType` is `FREEFORM_SQL` or `SIMPLE_SQL`.** `FREEFORM_SQL` is a full SQL query;
   `SIMPLE_SQL` is a single-column predicate (set `columnName`). Nothing else is valid.
4. **Rules require a PUSHDOWN job.** If a rule call returns an error mentioning the dataset is
   not PUSHDOWN (HTTP 422), rule creation/editing is not allowed on that job — tell the user
   rather than retrying.
5. **Read the `status` field in every response.** Branch on `success`, `validation_error`,
   or `error`. For `validate_dq_rule`, `status: success` means validation *ran* — the verdict
   is the separate `valid` field.
6. **`delete_dq_rule` is destructive.** Confirm with the user before deleting a rule.
7. **Creating a rule does not run it.** A new rule is only evaluated on the next run. Call
   `run_dq_job` to execute it, then observe the outcome.

## Workflow: create and run a rule

1. **Discover** — call `prepare_create_dq_job` for the job to get `edgeSiteId`,
   `connectionId` and `schemaName`. (Skip if you already have them.)
2. **Validate** — call `validate_dq_rule` with those IDs, the `jobName`, and `previewRule`
   (the SQL you intend to use as `monitorValue`). If `valid` is `false`, fix and re-validate.
   Optionally call `preview_dq_rule_sql` (same inputs) to see the sample rows the SQL returns.
3. **Create** — only once valid, call `create_dq_rule` with `jobName`, `monitorName`,
   `monitorType`, `monitorValue` (and optional `filterQuery`, `columnName`, `dimensions`,
   `tolerance`, `active`, `suppressed`).
4. **Run** — call `run_dq_job` with the `jobName`. It returns a `jobRunId`.
5. **Observe** — poll `get_dq_job_status` with the `jobRunId` until it is terminal
   (`FINISHED`, `FAILED`, `CANCELLED`). On failure, read `exception` and call `get_dq_job_log`
   for the run stages. Then call `get_dq_rule_results` for the rule to see its score and
   breaking/passing record counts.

## Managing an existing rule

- **Inspect** — `get_dq_rule` returns a rule's current definition (type, SQL, filter,
  tolerance, active/suppressed).
- **Edit** — `edit_dq_rule` is a full replace, not a patch: send the complete definition, not
  just the changed field. Set `newMonitorName` to rename. Re-validate the new SQL first
  (rule 1). A safe pattern is `get_dq_rule` → modify the returned fields → `validate_dq_rule`
  → `edit_dq_rule`.
- **Delete** — `delete_dq_rule` after user confirmation (rule 6).

## Reading results

`get_dq_rule_results` is paginated (`offset`/`limit`, newest first by default). Each entry is
one job run: `ruleStatus` (PASSING / BREAKING / EXCEPTION), `passFail`, `score`,
`totalCount`, `breakingRecords`, `passingRecords`, and `exception` when the run errored. Use
it to confirm a rule behaved as intended after `run_dq_job`.
