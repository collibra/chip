# Eval suite: dq-job-from-data-contract (skill)

Behavioral evals for the `collibra/dq-job-from-data-contract` skill
([pkg/skills/files/collibra/dq-job-from-data-contract/SKILL.md](../../pkg/skills/files/collibra/dq-job-from-data-contract/SKILL.md)).
These are **behavioral evals**: does an LLM, given a natural-language request and the
real tool attached over MCP, follow the skill correctly? That's a different question
from the Go unit tests in `pkg/tools/*/tool_test.go` and `pkg/skills/*_test.go`, which
prove the handler/catalog logic is correct given a specific input. Evals assume the
handlers are correct and check whether the skill's guidance leads a model to call the
right tools with the right arguments.

**This suite evaluates a whole *skill*, not a single tool's schema.** A skill is a
multi-phase Markdown guide that orchestrates several tools in sequence
(`pull_data_contract_manifest` → `get_asset_details` → `create_data_quality_job` →
`find_data_quality_rules` → `create_data_quality_rule` → `get_data_quality_rule_results`,
plus `list_collibra_skills` / `load_collibra_skill` to load the skill in the first
place). So there are two kinds of scenario here, not one:

- **Task scenarios** (`scenarios.yaml` + `checks.py`) - does the skill-guided agent do
  the *right thing* across that whole workflow?
- **Trigger scenarios** (`trigger_scenarios.yaml` + `trigger_checks.py`) - does the
  skill's `description` cause `load_collibra_skill` to fire at the right times, and
  yield to sibling skills (`collibra/dq-rules`, `collibra/dq-rule-workbench`,
  `collibra/data-product-create`) on near-misses?

Both were ported from the skill's own test plan, produced when the skill was authored
via the skill-creator tool (`evals.json` and `trigger_evals.json` in that source tree).
The task prompts and assertions, and the trigger queries and labels, are carried over
essentially as-is; `checks.py` and `trigger_checks.py` translate the prose assertions
into deterministic, transcript-based grading functions, following this repo's existing
convention (no LLM-graded rubrics).

## The golden rule: dry-run only

This skill performs **live writes** in the real product (DQ jobs, DQ rules) - carried
over directly from the skill's own testing guide. Every scenario in this suite MUST run
in preview/dry-run mode: no tool call may ever set `confirm=true`. `runner.py` appends
an explicit instruction to every prompt forbidding it, and `checks.py`'s
`_confirm_never_true` independently re-checks the full transcript regardless of what the
agent was told - a single violation there is a real defect, not sampling noise, exactly
like `create_data_quality_job`'s `confirm_gate_blocks_first_write` scenario.

## What's covered

### Task scenarios (`scenarios.yaml`)

| Scenario | Property under test |
|---|---|
| `email-full-flow` | Reads the contract manifest, matches the job to the manifest's Snowflake server (not the fixture's decoy Postgres copy), and proposes tolerance-0 rules - all before any write |
| `existing-job-reuse` | Detects the job already scanning the table, runs the duplicate-rule check against it, and doesn't spin up a redundant second job |
| `verify-and-results` | Maps contract clauses to rules and attempts to read run results before answering a "passing or failing" question |
| `timeslice-latest-only` | Does not invent a time-slice column on a table whose only date-like field is stored as VARCHAR (no genuine load timestamp) |
| `referential-integrity-clause-blocked` | Declines the one clause requiring a second table (no JOIN/subquery support) while still proposing the contract's single-table clauses |

### Trigger scenarios (`trigger_scenarios.yaml`)

20 queries (10 should-trigger, 10 should-not-trigger), all graded by the single
`trigger_check` function in `trigger_checks.py`: pass if a should-trigger query caused
a `load_collibra_skill` call naming `collibra/dq-job-from-data-contract`, and pass if a
should-not-trigger query did *not* load this specific skill (loading a sibling skill, or
none at all, is fine and expected for the near-misses).

Each is graded by a deterministic function against the tool-call transcript - no LLM
grading, no dependency on a real Collibra tenant. See
[`fixtures/mock_collibra.py`](fixtures/mock_collibra.py) for the canned contract
manifest, connections, table/columns, and existing-job/rule data every scenario runs
against.

## Prerequisites

```bash
cd evals/dq_job_from_data_contract
pip install -r requirements.txt
export ANTHROPIC_API_KEY=...   # the model used to drive the agent under test
```

Requires `go` on PATH (the runner builds the real `chip` binary from this checkout, the
same way `create_data_quality_job`'s runner does, so the eval exercises the actual tool
descriptions/schemas and the actual embedded skill catalog - not a re-implementation of
either).

**Note on the SDK call:** like `create_data_quality_job`'s runner, this is written
against the Python [`claude-agent-sdk`](https://pypi.org/project/claude-agent-sdk/)'s
`query()` API. If your installed version's shapes differ, `runner.py` is the only file
that touches the SDK - adjust `run_trial()` there.

## Running

```bash
python3 runner.py                                # everything: task + trigger
python3 runner.py --suite task                    # only the 5 workflow scenarios
python3 runner.py --suite trigger                  # only the 20 trigger queries
python3 runner.py --scenario email-full-flow       # a single named scenario
python3 runner.py --trials 5                        # override every trial count
```

Output is a pass rate per scenario, plus the failure reason for each failing trial.
Task-scenario trials default to **3**, not the 5 used elsewhere in this directory:
each trial here is a multi-tool-call workflow (potentially several LLM turns, each
possibly calling more than one tool), so it costs materially more per trial than a
single-tool-call scenario - 3 still gives a usable pass-rate signal without ballooning
run time/cost. Trigger scenarios default to 3 for the same reason, even though a single
turn is typically enough to see whether the skill loaded.

Treat every task scenario as effectively a safety-relevant check, since the underlying
skill performs live writes: a `confirm=true` anywhere, or a proposed cross-table
referential-integrity rule (`referential-integrity-clause-blocked`), is a real defect.
The `timeslice-latest-only` and `existing-job-reuse` outcomes are more reasonably judged
on trend across trials given normal model variance.

## Fixture design

[`fixtures/mock_collibra.py`](fixtures/mock_collibra.py) reuses the shape of the
connection/dataSource/schema/table/column discovery endpoints and the job-create
endpoint from `create_data_quality_job`'s own eval fixture wholesale, and adds what the
additional tools need: the data-contract manifest download, a GraphQL
`get_asset_details` stand-in, the DQ rule search/create/results endpoints.

The fixture story mirrors the skill's own worked example
([`references/examples.md`](../../pkg/skills/files/collibra/dq-job-from-data-contract/references/examples.md)):
an `EMPLOYEES` table on a Snowflake-like `PUSHDOWN` connection (`DQ_SNOWFLAKE_PUSHDOWN`),
governed by an active data contract whose manifest states `EMAIL` not-null,
`EMPLOYEE_ID` not-null + unique, and a `DEPARTMENT_ID`-must-exist-in-`DEPARTMENTS`
referential-integrity clause (for the cross-table-block scenario). `EMPLOYEES`'
only date-like column, `DATE_OF_HIRE`, is stored as `VARCHAR` - deliberately, so the
time-slice scenario has a real "not possible" answer to find rather than a contrived
one. A DQ job (`PUBLIC.EMPLOYEES_1`) already scans the table with no contract rules on
it yet, for the reuse scenario.

A second, wrong-platform connection (`DQ_POSTGRES_PULLUP`, fronting a Postgres copy of
the table) is included to exercise the connection-mismatch trap mentioned in the
skill's rule 2 - kept to one extra connection/table/column set rather than a full
parallel fixture, per the scope note in the task that asked for this suite: add it only
if it doesn't balloon complexity.

## Known simplifications (flagged for review)

- **The data contract UUID is given directly in every task prompt.** The real skill's
  Phase 1 can walk from a Data Product / Port to its Data Contract via
  `get_asset_details` relations, but this suite's allowed toolset has no general
  asset-search tool (only the specific tools the skill's phases name), so prompts name
  the fixture's data contract ID directly rather than exercising that discovery walk.
  Asset discovery/search is already covered by other parts of this codebase (see
  `collibra/discovery`); this suite focuses on the parts specific to this skill -
  manifest reading, connection matching, rule mapping, and the guardrails.
- **No "already fully enforced" fixture state.** The skill's worked examples include an
  idempotent-reuse case where the contract's rules already exist on the job, but that
  scenario isn't one of the five ported from the skill's `evals.json` task evals, so
  the fixture doesn't seed it. `existing-job-reuse` here tests that the duplicate check
  runs and the job is reused - not that "add nothing" outcome specifically.
- **`get_data_quality_rule_results` returns one canned PASSING result for any rule name**
  under the existing job, rather than modeling exactly which rules would realistically
  have run history. This is enough to exercise `verify-and-results`'s "attempt to read
  results before answering" behavior without a much larger fixture.

## Extending

To add a scenario:
1. Add fixture data if it needs something the current mock doesn't have.
2. Add an entry to `scenarios.yaml` (task) or `trigger_scenarios.yaml` (trigger).
3. Write the `check` function in `checks.py`, or rely on the existing `trigger_check` in
   `trigger_checks.py` for a new trigger query - keep grading code-based.

To start the next skill's suite, copy this directory's shape rather than sharing
fixtures across skills - each skill's mock backend should only serve what that skill's
tools actually call.
