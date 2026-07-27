---
description: Author and validate custom data quality rules (monitors) on an existing DQ job, inspect them, and read their per-run results, using the validate/create/get/results DQ tools.
related: collibra/discovery, collibra/dq-rule-workbench
---

# Data quality rules

A data quality **rule** (a "monitor") is a check attached to an existing DQ **job**
(a dataset — a saved data-quality check on one database table). This skill covers
authoring a custom rule (validate → create), inspecting it, and reading its per-run
results. It does **not** cover creating the job itself, editing or deleting rules,
or triggering job runs — a new rule is evaluated on the job's next (scheduled) run.

Rule tools: `validate_data_quality_rule`, `create_data_quality_rule`, `get_data_quality_rule`, `get_data_quality_rule_results`.

## Hard rules

1. **Validate before you create.** Always call `validate_data_quality_rule` on the rule SQL *before*
   `create_data_quality_rule`. It checks the SQL against the source and returns `valid: true/false`
   plus a message, so a malformed rule (e.g. a bad `SIMPLE_SQL`/SQLG predicate) is caught up
   front. `validate_data_quality_rule` takes the raw SQL — the rule does **not** need to exist yet. If
   `valid` is `false`, fix the SQL and re-validate; do **not** create the rule.
   - **`create_data_quality_rule` has a confirm checkpoint.** Call it first with `confirm` omitted/false:
     it returns a `preview` (the composed rule and its SQL) and creates nothing. Show that
     preview to the user, then call again with `confirm: true` to actually create. The tool
     enforces this — it will not write on a `confirm=false` call.
2. **`validate_data_quality_rule` needs discovery IDs.** It requires `edgeSiteId`, `connectionId` and
   `schemaName`. Get them from `prepare_create_data_quality_job` for the target job — do not guess them.
   (`create_data_quality_rule`, `get_data_quality_rule` and `get_data_quality_rule_results` take only names/ids and need no
   discovery step.)
3. **`monitorType` is `FREEFORM_SQL` or `SIMPLE_SQL`.** `FREEFORM_SQL` is a full SQL query;
   `SIMPLE_SQL` is a single-column predicate. Nothing else is valid.
4. **Always give the rule a meaningful name.** `monitorName` is required and is how the rule is
   found and reported on later. Ask the user for a name; if they don't supply one, propose a
   clear, descriptive name (e.g. `orders_amount_not_null`) and confirm it before creating — do
   not invent an opaque name. Names allow only letters, digits, `-` and `_`.
5. **For `SIMPLE_SQL`, ask which column the check targets** and pass it as `columnName`. For
   `FREEFORM_SQL` the column(s) live inside the SQL, so `columnName` is not needed.
6. **Rules require a PUSHDOWN job.** If `create_data_quality_rule` returns an error mentioning the dataset
   is not PUSHDOWN (HTTP 422), rule creation is not allowed on that job — tell the user rather
   than retrying.
7. **Read the `status` field in every response.** Branch on `success`, `validation_error`, or
   `error`. For `validate_data_quality_rule`, `status: success` means validation *ran* — the verdict is
   the separate `valid` field.
8. **Creating a rule does not run it.** A new rule is only evaluated on the job's next run
   (runs happen via the job's schedule — this skill does not trigger them). Once a run has
   happened, use `get_data_quality_rule_results` to see how the rule did.

## Workflow: author a rule

1. **Discover** — call `prepare_create_data_quality_job` for the job to get `edgeSiteId`, `connectionId`
   and `schemaName`. (Skip if you already have them.)
2. **Validate** — call `validate_data_quality_rule` with those IDs, the `jobName`, and `previewRule`
   (the SQL you intend to use as `monitorValue`). If `valid` is `false`, fix and re-validate.
3. **Create** — only once valid, call `create_data_quality_rule`. First make sure you have a meaningful
   `monitorName` from the user (rule 4) and, for a `SIMPLE_SQL` rule, the target `columnName`
   (rule 5). Call with `confirm` omitted to preview, then `confirm: true` to create. Pass
   `jobName`, `monitorName`, `monitorType`, `monitorValue` (and optional `filterQuery`,
   `columnName`, `dimensions`, `tolerance`, `active`, `suppressed`).

## Inspecting a rule

`get_data_quality_rule` returns a rule's current definition (type, SQL, filter, tolerance,
active/suppressed) by `jobName` + `monitorName`.

## Reading results

`get_data_quality_rule_results` is paginated (`offset`/`limit`, newest first by default). Each entry is
one job run: `ruleStatus` (PASSING / BREAKING / EXCEPTION), `passFail`, `score`, `totalCount`,
`breakingRecords`, `passingRecords`, and `exception` when the run errored. Use it to confirm a
rule behaved as intended after the job has run. Results appear only once the job has executed
at least once since the rule was created.
