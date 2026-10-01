---
description: Triage a data quality finding (failing monitor, dropped DQ score, failed job run) — decide whether data or the check is at fault, walk impact to data products and contracts, name the owner, prioritise, and recommend the narrowest fix.
related: collibra/dq-rules, collibra/dq-root-cause, collibra/lineage, collibra/discovery
requires: data-quality
---

# DQ Incident Triage

Triage answers four questions, in this order:

1. **Is this real?** — did data break, did a rule break, or did the job fail to run?
2. **What is broken?** — which monitors, which columns, which dimension, how severe.
3. **What does it impact?** — which tables, ports, data products, contracts, consumers.
4. **Who owns it and what now?** — the responsible party, the priority, the recommended action.

Finding where an issue originated upstream is root cause analysis — hand off to
`collibra/dq-root-cause`.

Triage stops at a recommendation. It does not fix data, edit rules, or run jobs unless
the user explicitly asks — and even then, see *Guardrails*.

**On the "Observed" notes below:** each hard rule cites concrete behaviour seen in the
`dq-benchmark` environment on 2026-09-17. They are evidence for *why* the rule exists, not
current state. Specific job names, scores, and monitor states will have drifted — re-verify
before quoting a figure to anyone.

---

## Hard rules

Each of these exists because the real environment violated a reasonable assumption.

1. **`FAILED` on a job run means the job did not execute. It does not mean the data is bad.**
   A run status of `FAILED` is an *operational* incident (Edge site upgrading, connection
   refused, driver error). Read the `exception` field and report it as an infrastructure
   problem. Do not describe the data as low quality when no data was ever read.
   Observed: a run failed with `"Upgrade of Edge site ... in progress. Try again later."`
   and `rowCount: 0`. There were no data findings at all.

2. **Never trust the headline `score` on its own.** A run can report `score: 100` while
   carrying a breaking monitor and a monitor in `EXCEPTION`. Always enumerate monitor
   states before making a health claim.
   Observed: `HARVESTER.DQ_TEST_WIDE_TABLE` — `score: 100`, `breakingMonitors: 1`, and a
   custom monitor in `EXCEPTION`.

3. **Distinguish the four monitor states. They mean different things and route differently.**
   - `BREAKING` — the check ran and the data violated it. This is a data incident.
   - `EXCEPTION` — the check itself errored. This is a *rule or platform* defect, not a
     data defect. Report it separately and never count it as a data finding.
   - `PASSING` — fine.
   - `LEARNING` — an adaptive monitor still establishing a baseline. **Not yet enforcing.**
     Reporting a `LEARNING` monitor as an issue is a false positive.

4. **Prefer `find_data_quality_rules` to enumerate findings — but a zero result is not
   evidence of no findings.**
   `find_data_quality_rules(jobName=...)` returns one compact row per monitor including
   `monitorStatus`, `columnName`, `dimensions`, and `ruleQuery`. `dq_get_job_run` returns
   every adaptive monitor in full and is enormous — a 231-monitor job produced tens of
   thousands of tokens, nearly all of it `LEARNING` noise.
   **The trap:** `find_data_quality_rules` is *rule*-scoped. On a job carrying only adaptive
   monitors it returns `0 of 0` even when that job has breaking monitors.
   Observed: `HARVESTER.DQ_TEST_WIDE_TABLE` returns 0 rules, while its run reports
   `activeMonitors: 231`, `breakingMonitors: 1`, and a custom monitor in `EXCEPTION`.
   So: call `find_data_quality_rules` first; if it returns 0, do **not** conclude "healthy" —
   fetch the latest run and read `activeMonitors`, `breakingMonitors`, and `customMonitors`
   before making any health claim. Also use `dq_get_job_run` whenever you need run-level
   metadata (`exception`, `executedQuery`, `rowCount`, `runDate`, `executionTimeSeconds`).

5. **A breaking monitor may be a bad rule, not bad data. Read the `ruleQuery` before you
   believe it.** Check whether the predicate actually expresses the intended constraint.
   If it does not, the finding is a false positive and the recommendation is *fix the rule*.
   Observed: a monitor named `CUSTOM_RULE` on column `sign` was `BREAKING` with
   `SELECT * FROM @PUBLIC.FOUR_DAY_RUN WHERE "SIGN" IS NOT NULL` — which flags every
   non-null row. That is a misauthored rule, not a quality problem.

6. **The catalog's DQ score aggregates every DQ job on that table, not the one you are
   looking at.** Enumerate jobs with **`dq_search_jobs(table=...)`** — that is the reliable
   path. The catalog-side `represents` relation from `Data Quality Job` assets is a useful
   cross-check but is **often absent**, so never treat it as the enumeration method.
   Observed: table `FOUR_DAY_RUN` reported 142 total / 131 passing monitors while the job
   `PUBLIC.FOUR_DAY_RUN` had only 25. `dq_search_jobs` found **six** jobs on that table —
   yet the table asset had **zero** `represents` relations and no `Data Quality Job` assets
   in the catalog at all. By contrast table `ACCOUNTS` had four DQ jobs all
   `represents`-linked. Coverage of that relation is inconsistent.
   Job-to-table mapping is many-to-one; never assume one job.

7. **Technical lineage may be unavailable. Fail over, don't fail.** Impact analysis via
   `search_lineage_entities` → `get_lineage_downstream` is the richest path but is not
   dependable. See `collibra/lineage` for the ID bridge. Fall back to catalog relations (rule 8), and say plainly that lineage was
   unavailable so the impact picture is business-level only.
   Observed: `search_lineage_entities` returned HTTP 500 on both `dgcId` and `nameContains`.
   It was not a bad parameter — the endpoint was down.

8. **The catalog relation path is the dependable impact route.** It works without lineage:

   ```
   Data Product Port  ──is implemented as──▶  Table
   Data Product       ──exposes data as────▶  Data Product Port
   Data Contract      ──governs functioning of──▶  Data Product Port
   Column             ──is part of─────────▶  Table
   Data Quality Job   ──represents─────────▶  Table   (often absent — see rule 6)
   ```

   Mind the direction. Starting from a **table**, every one of these is an
   *incoming* relation — a table asset may have no `outgoingRelations` at all. So from a
   table you reach the port via `incomingRelations`, then read the port's
   `incomingRelations` to reach the data product and the contract.
   Walk it with `get_asset_details`, reading `incomingRelations` and `outgoingRelations`.
   Verified end to end: table → port → data product (category `Foundational`), and
   port ← contract.

9. **Always check for a governing contract, and check whether it is actually enforced.**
   A contract turns a soft finding into a breach of a stated promise — the single biggest
   priority multiplier. Pull it with `pull_data_contract_manifest(dataContractId=...)` and
   compare its `properties` against the breaking monitors.
   Constraints are not all at the same level: numeric bounds sit under
   **`logicalTypeOptions`** (e.g. `logicalTypeOptions.maximum: 0`), while `required` and
   `unique` sit directly on the property. Reading only top-level property keys will miss the
   bounds.
   Observed: a contract promised `ALL_NEGATIVE_NUMBERS` `logicalTypeOptions.maximum: 0`
   and `DATA_DATE` `required: true, unique: true`. Monitor `ALL_NEGATIVE_NUMBERS__MAX VALUE`
   was `BREAKING` — a direct, explicit contract violation.
   Also observed: another job ran against the same contract-governed table with **zero**
   rules. A contract can exist and be entirely unenforced on a given job. Say so.

10. **Report ownership honestly, including when there is none.** Read `responsibilities` on
    each asset in the impact chain. `inherited: true` against an admin account is not a real
    steward, and `responsibilitiesStatus: "No responsibilities assigned"` means nobody is
    accountable. Both are triage findings in their own right — escalate them rather than
    inventing an owner.
    Observed: a data product and its port both had no responsibilities assigned; two tables
    had only an inherited `Admin` owner.

11. **Surface permission errors verbatim.** If a call fails for a missing scope
    (`dgc.ai-copilot`, `dgc.catalog`, `dgc.data-contract`, …), name the scope. Do not
    silently substitute a weaker tool — semantic and keyword search return qualitatively
    different result sets and a silent downgrade produces a misleading triage.

---

## Phase 1 — Establish what broke

Resolve the user's starting point. They will hand you one of: a job name, a job run, a table
or report name, a data product, or just "the DQ score dropped."

| Starting point | First call |
|---|---|
| Job name | `find_data_quality_rules(jobName=...)` — then rule 4's cross-check |
| "What's failing right now?" | `dq_search_job_runs(status=...)`, then per-job as above |
| Table / report / product name | `search_asset_keyword(query=..., assetTypeFilter=["Table"])` → `get_asset_details` |
| Table, and you want its DQ jobs | `dq_search_jobs(table=...)` |
| Vague / conceptual | `discover_data_assets(...)` with the user's own wording |
| Specific run id | `dq_get_job_run(run_id=...)` |

Note the filter parameter on `search_asset_keyword` is **`assetTypeFilter`** (an array), not
`assetType`. An unrecognised key is silently ignored and you will get unfiltered results
back — including `DataQualityJob` resources mixed in with tables.

Parameter naming is inconsistent across these tools (`run_id`, `name`, `jobName`,
`dataContractId`). Read each schema; do not pattern-match from a sibling tool.

Then classify before going further:

- Run status `FAILED` → **operational incident.** Read `exception`. Report the platform
  fault, note that no data was assessed, stop. Do not proceed to impact analysis.
- Run status `FINISHED` → enumerate monitors and bucket them by state (rule 3).
- Monitors `BREAKING` → read each `ruleQuery` and sanity-check it (rule 5).
- Monitors `EXCEPTION` → report as rule/platform defects in a separate section.

State the finding in one sentence before elaborating: which column, which dimension,
which check, and whether data or the check itself is at fault.

## Phase 2 — Assess impact

1. Resolve the job's table: `search_asset_keyword(query=..., assetTypeFilter=["Table"])`.
2. `get_asset_details(assetId=<table>)` — read `numericAttributes` for
   `Data Quality Score`, `Total Active Quality Monitors`, `Passing Quality Monitors`, and
   read `incomingRelations`.
3. Enumerate *all* DQ jobs on the table with `dq_search_jobs(table=...)` (rule 6) so score
   arithmetic reconciles.
4. Walk the relation chain in rule 8 to reach ports, data products, and contracts.
5. Attempt lineage for technical downstream consumers. If it errors, say so and continue
   with the business-level picture (rule 7). Column-level lineage is unreliable by design —
   go through the parent table.
6. For a column-scoped finding, confirm the column belongs to the table via the
   `is part of` relation, and check whether that specific column is named in the contract.

Report impact concretely: name the data products and contracts, and say what a consumer of
them would actually get wrong. "One data product affected" is weaker than "this foundational
data product exposes the table through a SQL port, so any consumer reading
`ALL_NEGATIVE_NUMBERS` may receive positive values the contract forbids."

## Phase 3 — Prioritise

Weigh, roughly in this order:

1. **Contract breach** — an explicit promise is broken. Highest.
2. **Data product exposure** — especially `Foundational` category, which other products build on.
3. **Dimension** — `Accuracy` and `Completeness` failures on a key or measure column
   generally outrank `Duplication` on a descriptive one.
4. **Blast radius** — count of downstream products, ports, reports.
5. **Persistence** — is this new, or has it been breaking across runs?
   Use `get_data_quality_rule_results(...)` (paginated, newest first) for the monitor's history.
6. **Confidence** — a `BREAKING` monitor whose `ruleQuery` is suspect is *lower* priority
   than a clean one, and its action is "fix the rule."

Give a single priority with a one-line justification. Do not produce a scoring rubric the
user did not ask for.

## Phase 4 — Recommend

Recommend the narrowest action that addresses the actual fault:

- **Bad data, contract-governed** → remediate the data; flag the contract breach to the
  product owner; consider a blocking rule if none exists.
- **Bad data, no contract** → propose the rule or monitor that would have caught it earlier,
  and ask whether a contract should govern this table at all.
- **Bad rule** → propose the corrected predicate. Validate with
  `validate_data_quality_rule` before proposing creation; never create without confirmation.
- **`EXCEPTION`** → report the error text; this is usually an engineering fix, not a data fix.
- **Operational failure** → name the platform fault and the retry condition.
- **Contract exists but unenforced on this job** → say which promises have no corresponding
  monitor, and offer to author them (hand off to `collibra/dq-rules`).
- **No owner** → escalate the ownership gap explicitly.

Close by recording the triage if the user wants it persisted. The `Data Quality Issue` asset
type exists for this (`publicId: DataQualityIssue`, product `HELPDESK`,
id `00000000-0000-0000-0000-000000031603`), and the environment carries issue-category assets
(`Completeness Issue`, `Accuracy Issue`, `Conformity Issue`, `Consistency Issue`,
`Duplication Issue`, …) to classify against. Creating one is a write — confirm first.

---

## Guardrails — stop and ask

- Before **any** write: creating a `Data Quality Issue`, creating or editing a rule,
  editing an asset. Preview, show it, wait for an explicit yes.
- Before running or cancelling a job. A run costs compute and may page someone.
- When the impact chain reaches something you cannot see (an uncatalogued consumer, a report
  behind a permission wall). Say what you could not check rather than implying completeness.
- When the data itself is sensitive. Report counts and column names; do not echo row values
  into the conversation unless asked.
- When two plausible assets match the user's phrasing. Ask which. Do not guess and drill down.
- When lineage or another dependency is down. Report the degradation rather than presenting
  a partial picture as whole.

## Output shape

```
FINDING      one sentence: what broke, where, data-fault or check-fault
SEVERITY     priority + one-line why
IMPACT       tables → ports → data products → contracts; note anything unverifiable
CONTRACT     breached promise, or "no governing contract", or "contract present, unenforced"
OWNER        responsible party, or an explicit ownership gap
ACTION       narrowest next step, plus what to do to prevent recurrence
CAVEATS      tools that failed, monitors in LEARNING, anything not checked
```

Keep it tight. A triage that takes longer to read than to act on has failed.

## Known tool gaps — carry these as caveats

- No way to filter monitors by state on the run-detail endpoint — `dq_get_job_run` takes only
  `run_id` and returns all monitors. (`dq_search_job_runs` does have a run-`status` filter.)
- `breakingMonitors` cannot be reconciled against the returned monitor list — on the observed
  run the breaking monitor was not in the payload at all.
- `find_data_quality_rules` returns `0 of 0` for adaptive-only jobs, which reads as a false
  all-clear. Always cross-check the run (rule 4).
- The `represents` relation from DQ jobs to tables is inconsistently populated, so catalog
  DQ coverage is silently incomplete. Use `dq_search_jobs(table=...)`.
- Technical lineage was returning HTTP 500; the DGC→lineage bridge is the documented sole
  entry point and has no fallback.
- `dq_search_job_runs` has no filter for "runs with findings" versus "runs that failed to execute."
- Nothing links a breaking monitor or job run to a `Data Quality Issue` asset, so there is no
  built-in audit trail of who triaged what and why.

