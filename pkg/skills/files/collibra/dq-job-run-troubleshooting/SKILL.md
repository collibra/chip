---
description: Troubleshoot why a Collibra Data Quality job failed to run, hung, or ran slowly — at the execution layer (Edge site, Kubernetes/Spark resources, JDBC connection, credentials, data-source permissions, source SQL). Reads the run's exception and Edge stage logs (Spark submit / driver) via chip's dq_get_job_run_logs, names the failing layer, and recommends concrete changes a customer DQ admin can make. Use whenever a user gives a DQ job name or run id and asks why it failed, errored, timed out, got stuck in WAITING/DISPATCHED/RUNNING, was cancelled, is slower than usual, or asks to "look at the logs", "read the Spark logs", "why won't this job run", "tune this job", or mentions OOM, executor lost, pod pending, connection refused, permission denied, or authentication errors on a DQ job. Do NOT use for low/dropped DQ scores or failing monitors/rules on a job that ran fine — that is dq-incident-triage.
related: collibra/dq-incident-triage, collibra/dq-root-cause, collibra/dq-rules
requires: data-quality
---

# DQ job run troubleshooting

The question this skill answers: **"Why didn't this job run cleanly, and what do I change?"** It is about the machinery — Edge, Spark on Kubernetes, the JDBC connection, the credentials, the source query — not about whether the data is good. If the run FINISHED and the user is unhappy with the score or a monitor, hand off to `dq-incident-triage`.

The user is a customer DQ admin or data engineer. Every recommendation must be something they can actually do: change job config, change Edge capability or connection settings, fix credentials or grants in the source system, adjust Kubernetes resources, or enable debug logging. Don't recommend internal Collibra actions other than "open a support case with this evidence".

## How DQ jobs execute (just enough to localise a failure)

Read `references/execution-model.md` if you need more detail. The short version:

- **Pushdown**: SQL is generated and executed *inside the customer's database*; Edge orchestrates. Failures are usually connection, auth, permissions, SQL dialect/syntax, or warehouse timeouts/capacity. Only a **Stage 2 (Spark submit)** log exists.
- **Pullup**: Edge launches a **Spark job on the Edge site's Kubernetes cluster**, pulls rows over JDBC, computes in Spark. Adds Spark/K8s failure modes (OOM, pods pending, executor loss, slow stages, skew). Has **Stage 2 (submit)** and **Stage 3 (driver)** logs.
- Run status lifecycle: `WAITING → DISPATCHED → SETUP → RUNNING → SENDING → FINISHED` (or `FAILED`/`CANCELLED`). *Where* a run stopped is the first clue to *which layer* broke.

## Workflow

### 1. Resolve the job and pick the run

- User gives a job name → `dq_get_job(name)` for type (PUSHDOWN/PULLUP), edge site, connection, source query, run-date window. If it returns `needs_input`, show the candidates and ask.
- `dq_search_job_runs(job_name=…)` to see recent history. Pick the run the user means; default to the most recent FAILED/CANCELLED run, or the most recent run if they're asking about slowness. Note the **last successful run** too — you'll compare against it.
- `dq_get_job_run(run_id)` for status, `exception`, `executedQuery`, `runDate`, `rowCount`, timing.

`dq_search_job_runs` matches job names fuzzily (`public.nyse_2` also returns `PUBLIC.NYSE_29`, `PUBLIC.NYSE_2`, …). Filter the results to the exact job name before reading history, and don't mix in lookalike jobs as if they were the same job.

If the run has **no exception and no logs**, read `executedQuery` yourself before anything else — a malformed source query (stray characters, unbalanced quotes, unresolved `${rd}`) often fails without a recorded error.

The history alone often tells you a lot, so look at it before reading logs:
- Started failing at a specific point after a streak of successes → something changed (credentials rotated, grant revoked, schema change, Edge upgrade, source query edited).
- Fails every time since creation → configuration was never right (connection, driver, query).
- Intermittent → capacity/contention (cluster, warehouse, network), or timeouts near a limit.
- Other jobs on the **same connection** failing in the same window → the connection (credentials, JDBC properties, grants) is the problem, not the job. This is the most common pattern for connection/auth errors, so check it whenever the exception is connection- or auth-related: find a few other jobs with the same `connectionName` (via `dq_get_job`) and look at their run history around the failure. Read their exceptions too — a sequence like "wrong password" on day 1 → "malformed authenticator" on day 3 → success on day 5 tells the story of a credential rotation and a half-finished fix, and tells you whether the connection is healthy *now* (in which case the fix is simply a rerun).
- Many *different* jobs on the *same edge site* (across connections) failing in the same window → Edge site / cluster problem. Check with `dq_search_job_runs(status="FAILED")` and group by time — it saves the user from tuning a job that isn't the problem.

Note: a `jobRunId` in a DQ UI link can be an epoch-milliseconds run date (e.g. `1790121600000` = 2026-09-23), not the run's UUID. Convert it and match against `runDate` in the job's history.

### 2. Get the logs

Call `dq_get_job_run_logs(run_id)` with defaults first. It returns a parsed view — `diagnostics` (rule-based hints with evidence), `sparkConfig` (driver/executor sizing, k8s requests/limits), `jobSummary` (phase durations, slowest Spark stages, partitions, executors, rows loaded, exit status), and distinct error lines. Start from `diagnostics` and `errors`, but verify them against the evidence rather than repeating them — they are heuristics.

Dig further only when the parsed view doesn't settle it:
- `grep` with an RE2 regex (prefix `(?i)` for case-insensitive) + `grep_context` for specific signatures. `references/failure-signatures.md` has regexes grouped by layer.
- `tail_lines` (≈200) on the stage that failed — the root exception is usually near the end, *below* the first stack trace. The first error in a log is often a symptom (e.g. a retry warning); the last `Caused by:` is usually the cause.
- `stage="DRIVER"` for pullup Spark problems; `stage="SUBMIT"` for launch, connection and pushdown problems.

**If `status` is `not_available`** (common — Edge debug logging is off by default): don't stop. Diagnose from the run's `exception`, `executedQuery`, status, and history — that's frequently enough (a JDBC exception message usually names the problem). Then tell the user plainly that stage logs weren't captured and, if the diagnosis isn't conclusive, that enabling debug logging on the job's Pushdown/Pullup **Edge capability** and rerunning will capture them. Offer to rerun the job once they've enabled it.

### 3. Localise the failure to one layer

Classify before recommending. Use the status the run died in, the exception, and the log evidence. `references/failure-signatures.md` maps signatures to layers and fixes.

| Layer | Typical evidence |
|---|---|
| **Dispatch / Edge site** | Stuck in WAITING/DISPATCHED; no logs at all; many jobs on one site failing together; Edge capability errors |
| **Kubernetes / Spark resources** (pullup) | Pods Pending, `Insufficient cpu/memory`, `OOMKilled`, exit code 137/143, `ExecutorLostFailure`, `java.lang.OutOfMemoryError`, GC overhead |
| **Connection / network** | `Connection refused`, `timed out`, `UnknownHost`, SSL/TLS handshake, proxy, `communication error`, `MalformedURL` |
| **Authentication / credentials** | `authentication failed`, `invalid username/password`, expired token/key, vault lookup failure, wrong authenticator property |
| **Authorization / permissions** | `permission denied`, `insufficient privileges`, `not authorized`, `does not exist or not authorized` (Snowflake), missing USAGE on schema/warehouse |
| **Source query / schema** | SQL syntax, `invalid identifier`, column/table not found, type cast errors, run-date filter column wrong, dialect mismatch |
| **Source capacity / timeouts** | Statement timeout, warehouse suspended/queued, query cancelled by source, lock waits |
| **Performance (ran, but slow)** | Long phase in `jobSummary`, one slow Spark stage, few partitions for many rows, skew (one task ≫ median), rows loaded much higher than usual |
| **Result write-back** | Stuck or failing in SENDING after compute finished |

If the evidence points at two layers, say which is the root and which is the symptom (e.g. an executor OOM *caused* by a `SELECT *` pulling 40 wide columns is a query-scope problem first, a sizing problem second).

State your confidence honestly. "The log shows X, which means Y" is different from "X is consistent with Y, but I couldn't see Z". Don't invent config keys or UI paths you haven't seen in the job definition, the logs, or the docs — say "in the Edge capability settings" rather than guessing a property name.

### 4. Recommend the narrowest fix

Lead with the one change most likely to fix it, then alternatives. For each, say **who** does it and **where**:

- **Job config** (source query, columns, run-date window, filters, monitors) → can be applied via `dq_update_job`.
- **Rerun** → `dq_run_job`.
- **Edge capability / connection** (credentials, JDBC properties, driver, Spark/K8s sizing, debug logging) → Edge admin in the Edge site UI or Edge CLI. Collibra Cloud sites don't support the Edge CLI.
- **Source system** (grants, user/role, warehouse size, network allow-lists, statement timeouts) → the source DBA/owner.
- **Kubernetes cluster** (node capacity, quotas, limits) → the customer's platform team (self-managed Edge only).

For performance: prefer reducing work (narrow the column list instead of `SELECT *`, tighten the run-date filter, push filters into the source query, switch a heavy pullup to pushdown if the source supports it) before adding resources. More memory hides the problem and costs money; say so when it applies.

**Slow runs without logs.** You can still size the problem from `dq_get_job` + `dq_get_job_run`:
- *Scope*: does `sourceQuery` have a `${rd}`/`${rdEnd}` filter? No filter = full-table scan every run, the most common cause of long runs.
- *Throughput*: `rowCount / executionTimeSeconds`. Low single-digit thousands of rows/s on a pullup usually means a single-threaded JDBC read — but say it's an inference until logs confirm.
- *Work per row*: `activeMonitors` and which monitor types. Uniqueness (CARDINALITY) on high-cardinality columns (observedValue ≈ rowCount, e.g. ids) forces a full distinct count per column — the most expensive adaptive monitor.
- *Sibling jobs*: a filtered or slimmed variant of the same table running in seconds is strong evidence.
Be clear which of these is proven and which is inferred, and name what debug logs would confirm (read-phase vs compute-phase time, partition count).

Compare against the last successful run whenever you can — "rows loaded went from 2M to 38M" or "duration was 90s for 30 days, now 14 min" is the most persuasive evidence you can give.

### 5. Offer to act — don't act unasked

After the diagnosis, offer the concrete action(s) you can take: update the job (show the exact before/after of what you'd change), or rerun it. Only call `dq_update_job` or `dq_run_job` after the user confirms. Never cancel or delete runs or jobs unless explicitly asked.

## Sweep mode ("did anything fail last night?")

When the user asks about a time window rather than one job:
1. `dq_search_job_runs(status="FAILED")` and `(status="CANCELLED")`; state the window you used in UTC (e.g. "9/29 20:00 – 9/30 12:00 UTC") since "last night" is ambiguous.
2. For each failed run in the window, get the run + job, and the job's own history (exact name only). Classify each as:
   - **New** — first failure after successes. Highest priority.
   - **Chronic** — failing every run for days/weeks. Check whether it looks deliberate (e.g. a test job whose query targets a non-existent object); flag it as noise rather than an incident, but say why you think so.
   - **Self-recovered** — a later run of the same job succeeded. Low priority; note the cause if knowable.
   - **Never succeeded** — only run(s) ever are failures → configuration.
3. Cancelled runs followed within minutes by a successful run of the same job are usually manual re-submits — mention in one line, don't investigate.
4. Group failures by connection and by time before diagnosing individually — a shared cause collapses several rows into one finding.
5. Output one table (job · time · type/connection · category · likely cause · confidence), then short detail only for items needing action. Be explicit about which failures you could not diagnose and what's missing (usually: no exception recorded + debug logging off).

## Output format

Keep it tight — the user wants the answer, not a log dump.

```
## <job name> — run <short run id> <STATUS> (<pushdown|pullup>, edge site <name>)

**What broke:** <one sentence, plain language>
**Layer:** <layer from the table> · **Confidence:** <high|medium|low>

**Evidence**
- <quoted log line or exception fragment, with stage and line number if from logs>
- <history / comparison fact, e.g. "last 12 runs succeeded; failing since 2026-09-24">

**Fix**
1. <primary action> — <who / where>
2. <alternative or follow-up> — <who / where>

**Can do now:** <e.g. "Update source query to select 6 columns instead of *" / "Rerun after you enable debug logging"> — want me to?
```

Add a short **"Also noticed"** section only for real secondary issues (e.g. an unrelated sizing waste). If logs weren't available, say so in the Evidence section and state what enabling them would reveal.

## Boundaries

- Job ran and finished, but score dropped or a monitor/rule fails → `dq-incident-triage`.
- Need to find which upstream asset caused bad data → `dq-root-cause`.
- Writing or fixing a rule's SQL (not the job's source query) → `dq-rules`.
- A rule-level SQL error that fails the whole run *is* in scope here — identify the rule, then hand the rule rewrite to `dq-rules`.
