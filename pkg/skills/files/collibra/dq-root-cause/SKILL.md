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
   DGC UUID straight to a lineage tool (see `collibra/lineage`).
3. **Walk one hop upstream.** Call `get_lineage_upstream(entityId=<id>)`. This returns the
   immediate source entities and the transformation ID connecting them. If the user wants to
   see *why* the columns are combined, call `get_lineage_transformation` on that ID (e.g. a
   `UNION ALL` or SQL view definition) — otherwise skip it, per `collibra/lineage`'s rule.
4. **Resolve each source entity's asset.** Call `get_lineage_entity` for each upstream source
   ID to get its table/column name and DGC identifier. Take the `dgcId` from that response and
   call `get_asset_details(assetId=<dgcId>)` on it. Use `asset.displayName` from the result as
   the `tableName` you'll pass to `dq_get_job` in step 5. If the response's `incomingRelations`
   contains a Schema relation, use that schema asset's `displayName` as the `schemaName` for the
   same call — this disambiguates same-named tables in different schemas.
5. **Check that asset for its own DQ failures — latest run only.** Find the job on the upstream
   table by calling `dq_get_job(tableName=<displayName>, schemaName=<schema displayName if found>)`,
   using the values resolved in step 4 — never pass `name`, since job names are free text and not
   guaranteed to match `schema.table`. If multiple jobs are returned, first select the latest one
   by comparing each job's `runDate.value` field. Then, from its runs, take the single most recent
   run (by run date/ID, not just the first one returned) and call `dq_get_job_run` on that run.
   Look for `BREAKING` monitors on that run — especially ones in the same dimension as
   the original failure (e.g. another NULL/Completeness break) — these are candidate root causes.
   - Never check monitors from older/historical runs of the same job — a monitor that broke in
     a prior run but is passing in the latest run is not a live root cause.
   - If `dq_get_job` returns `needs_input` with several candidates, pick the one whose
     data location matches the resolved asset (schema, data source, edge connection/site) and
     note the ambiguity to the user rather than guessing.
   - Not every upstream table has a DQ job configured; a miss here just means move to the next
     hop, it does not mean the trail is clean.
6. **Recurse.** Repeat steps 3–5 from each upstream source entity, continuing to walk further
   upstream until either:
   - you find an asset with a breaking monitor in the same dimension as the original failure —
     that is your leading root-cause candidate, or
   - `get_lineage_upstream` returns no further sources (a root table with no upstream lineage).
7. **Summarize the chain.** Present the full upstream path (e.g. `combined_col` ←
   `NYSE_COPY_3.TRADE_DATE` ← `NYSE_COPY_2` ← `NYSE_COPY`), state which hop(s) have breaking
   monitors, and call out the earliest/most severe one as the likely root cause. Don't stop at
   the first breaking monitor found without also reporting how it connects back down to the
   original failure — that causal link is the point of the trace.

## Hard rules

1. **Bridge before walking.** As in `collibra/lineage`, never pass a DGC UUID directly to
   `get_lineage_upstream`/`get_lineage_entity` — go through `search_lineage_entities` first.
2. **Don't over-resolve.** Only call `get_lineage_entity` for sources you're actually going to
   check for DQ jobs or report to the user, not every ID a page returns.
3. **A missing DQ job is not a clean bill of health.** Some upstream tables simply have no
   job configured — treat that as "unknown," not "passing," and keep walking upstream if the
   original symptom is still unexplained.
4. **Bound the recursion.** Stop once you hit a root source (no further upstream lineage), or
   once you've found and confirmed a root cause — don't keep walking past the answer. If the
   graph is very wide (many source entities per hop), prioritize hops that share the same
   column/dimension as the original failure before exploring unrelated branches.
5. **Latest run only, per asset.** When checking an upstream asset's DQ job for breaking
   monitors (step 5 of the workflow), always use that job's most recent run — never aggregate
   or check monitors across multiple historical runs of the same job.
6. **Never look up the job by name.** Job lookup (step 5) must go through `dq_get_job`'s
   table-based lookup, passing the asset's `displayName` (from `get_asset_details`, resolved via
   the entity's `dgcId`) as `tableName`, and its Schema relation's `displayName` as `schemaName`
   when present — never pass `name`, since job names are free text and not guaranteed to match
   `schema.table`.
7. **Only open transformation SQL if asked.** `get_lineage_transformation` is for when the
   user wants to see the actual logic (e.g. a `UNION ALL`) — it's not needed to establish the
   causal chain itself.
