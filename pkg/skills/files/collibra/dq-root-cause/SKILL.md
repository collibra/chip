---
description: Trace a data-quality monitor failure upstream through the lineage graph, hop by hop, to find which source asset actually caused it.
related: collibra/lineage, collibra/dq-rules, collibra/discovery
---

# DQ root-cause tracing

A breaking monitor on a job run is often a symptom, not the cause — the affected column may
be a view or transformation over one or more upstream tables, and the real failure lives
further back in the pipeline. This skill combines `collibra/lineage` (walking the graph) and
DQ job/run tools (checking monitors) into one recursive workflow: walk upstream one hop at a
time, and at each hop check whether that asset has its own breaking monitors.

This skill assumes familiarity with `collibra/lineage`'s DGC UUID → lineage entity ID bridge
and `collibra/dq-rules`' monitor/result vocabulary (PASSING/BREAKING, dimensions).

## Workflow

1. **Get the starting failure.** Call `dq_get_job_run(run_id=<jobRunId>)`. Note
   every monitor with `state: BREAKING` — its `primaryColumn` and `dimensions` (e.g.
   Completeness/NULL) are what you're chasing upstream.
2. **Bridge to the lineage graph.** If you only have a DGC asset UUID for the failing
   column/table, resolve it first with `search_lineage_entities(dgcId=<UUID>)` — never pass a
   DGC UUID straight to a lineage tool (see `collibra/lineage`). Then, before you start
   traversing the graph, call `get_lineage_transformation` on the failing entity to understand
   how it's derived (e.g. a `UNION ALL` or SQL view definition) — this context informs which
   upstream sources matter as you walk.
3. **Walk one hop upstream.** Call `get_lineage_upstream(entityId=<id>)`. This returns the
   immediate source entities and the transformation ID connecting them.
4. **Resolve each source entity's asset.** Call `get_lineage_entity` for each upstream source
   ID to get its table/column name and DGC identifier. Take the `dgcId` from that response and
   call `get_asset_details(assetId=<dgcId>)` on it. Use `asset.displayName` from the result as
   the `tableName` you'll pass to `dq_get_job` in step 5. If the response's `incomingRelations`
   contains a Schema relation, use that schema asset's `displayName` as the `schemaName` for the
   same call — this disambiguates same-named tables in different schemas.
5. **Check that asset for its own DQ failures — most recently run job only.** Find the job on
   the upstream table by calling `dq_get_job(tableName=<displayName>, schemaName=<schema
   displayName if found>)`, using the values resolved in step 4 — never pass `name`, since job
   names are free text and not guaranteed to match `schema.table`. If multiple jobs are returned
   (including `needs_input` candidates), compare each job's `runDate.value` field and select
   only the single most recently run job — never guess based on data location, name similarity,
   or any other heuristic, and never investigate more than one job for the same asset. Then,
   from that job's runs, take the single most recent run (by run date/ID, not just the first one
   returned) and call `dq_get_job_run` on that run. Look for `BREAKING` monitors on that run —
   especially ones in the same dimension as the original failure (e.g. another NULL/Completeness
   break) — these are candidate root causes.
   - Never check monitors from older/historical runs of the same job — a monitor that broke in
     a prior run but is passing in the latest run is not a live root cause.
   - Never consider any job other than the most recently run one; if it's ambiguous which job
     is most recent (e.g. missing/equal `runDate.value`), note the ambiguity to the user rather
     than guessing or checking multiple jobs.
   - Not every upstream table has a DQ job configured; a miss here just means move to the next
     hop, it does not mean the trail is clean.
6. **Recurse — trace the entire graph, don't stop at the first hit.** Repeat steps 3–5 from
   every upstream source entity at every hop, continuing until `get_lineage_upstream` returns no
   further sources (a root table with no upstream lineage) on each branch. Finding a breaking
   monitor is not a stopping condition: intermediate assets in the graph may have out-of-date
   job runs (stale, not yet scheduled, or simply less current than an asset further upstream), so
   a "clean" hop doesn't prove the branch is innocent and a "breaking" hop doesn't prove it's the
   deepest cause. Keep walking every branch to its root(s) and record every asset's status
   (breaking / passing / no job configured / stale run) along the way.
7. **Summarize the chain.** Present the full upstream graph (not just one linear path), state
   which hop(s) have breaking monitors, flag any hop whose latest run looked stale or out of
   date relative to its neighbors, and call out the earliest/most upstream breaking monitor(s) as
   the likely root cause(s). Report how each breaking hop connects back down to the original
   failure — that causal link is the point of the trace — and don't omit branches just because an
   earlier branch already turned up a candidate.

## Hard rules

1. **Bridge before walking.** As in `collibra/lineage`, never pass a DGC UUID directly to
   `get_lineage_upstream`/`get_lineage_entity` — go through `search_lineage_entities` first.
2. **Don't over-resolve.** Only call `get_lineage_entity` for sources you're actually going to
   check for DQ jobs or report to the user, not every ID a page returns.
3. **A missing DQ job is not a clean bill of health.** Some upstream tables simply have no
   job configured — treat that as "unknown," not "passing," and keep walking upstream if the
   original symptom is still unexplained.
4. **Trace the whole graph — don't stop at the first root cause.** Finding a breaking monitor
   does not end the trace; keep walking every branch until each one hits a root source (no
   further upstream lineage). Intermediate assets may have out-of-date job runs, so an
   upstream-of-an-upstream asset can be the real cause even after a nearer hop looked clean or
   already looked breaking. If the graph is very wide (many source entities per hop), you may
   prioritize hops that share the same column/dimension as the original failure, but still visit
   the remaining branches rather than skipping them.
5. **Latest run only, per asset.** When checking an upstream asset's DQ job for breaking
   monitors (step 5 of the workflow), select only the single most recently run job for that
   asset, then use that job's most recent run — never aggregate or check monitors across
   multiple jobs or multiple historical runs of the same job.
6. **Never look up the job by name.** Job lookup (step 5) must go through `dq_get_job`'s
   table-based lookup, passing the asset's `displayName` (from `get_asset_details`, resolved via
   the entity's `dgcId`) as `tableName`, and its Schema relation's `displayName` as `schemaName`
   when present — never pass `name`, since job names are free text and not guaranteed to match
   `schema.table`.
