---
description: Trace upstream sources and downstream consumers in Collibra's technical lineage graph. Covers the DGC UUID → lineage entity ID bridge.
related: collibra/discovery, collibra/index
---

# Technical lineage

Technical lineage maps the physical data flow — including unregistered assets, temporary
tables, and source code — across systems. This is broader than business lineage (which only
covers cataloged Collibra assets). Almost every lineage question follows the same three-stepit 
shape:

1. **Resolve** the entity with `search_lineage_entities` (the entry point).
2. **Walk** the graph with `get_lineage_upstream` or `get_lineage_downstream`.
3. **Selectively resolve** the most relevant entities with `get_lineage_entity`. Do not
   resolve every ID — summarize from graph structure.

`get_lineage_transformation` returns SQL or script bodies; only call it when the user asks to
see the actual transformation logic. `search_lineage_transformations` is a specialized search
by transformation name, not a general entry point.

## Hard rule: lineage IDs are not DGC UUIDs

`get_lineage_upstream`, `get_lineage_downstream`, `get_lineage_entity`, and
`get_lineage_transformation` all use **lineage entity IDs**, which live in a separate ID
space from Collibra DGC asset UUIDs. Passing a DGC UUID directly to any of these tools will
fail or return the wrong entity.

The only bridge from a DGC asset to the lineage graph is:

```
search_lineage_entities(dgcId=<DGC UUID>)  →  lineage entity ID
```

Always start there when the user gives you a catalog asset.

## Column-level lineage limitation

`search_lineage_entities` cannot reliably resolve columns:

- `nameContains` does not work for columns — the lineage graph does not expose column names
  through that filter.
- `dgcId` does not reliably resolve column UUIDs because there is no consistent mapping
  between catalog column UUIDs and lineage column entity IDs.

The workaround: find the parent **table** first, then walk into its columns from the lineage
graph. See `references/column-lineage-workaround.md`.

## Workflows

### Upstream sources for an asset

1. `search_lineage_entities(nameContains="<table-name>")` or `(dgcId=<UUID>)` → entity ID
2. `get_lineage_upstream(entityId=<id>)` → source entities + transformation IDs (paginated)
3. Summarize the graph. If the user wants details on a specific source, call
   `get_lineage_entity` for that one ID.
4. Only call `get_lineage_transformation` if the user wants the SQL or script body.

### Impact analysis (downstream consumers)

1. `search_lineage_entities(...)` → entity ID
2. `get_lineage_downstream(entityId=<id>)` → consumer entities (paginated)
3. Summarize what would break if this asset changed. Resolve names only for the consumers
   the user asks about.

### Column-level lineage

See `references/column-lineage-workaround.md` — never pass a column UUID to lineage tools
expecting it to resolve.

## Hard rules

1. **Bridge before walking.** Never call `get_lineage_upstream` / `_downstream` /
   `_entity` / `_transformation` with a DGC asset UUID. Use `search_lineage_entities` first.
2. **Summarize from graph structure, don't resolve everything.** Each `get_lineage_entity`
   call is round-trip cost; the upstream/downstream responses already contain enough
   structure to describe the graph. Resolve only the IDs the user asks about by name.
3. **Don't open transformations unless asked.** `get_lineage_transformation` returns SQL or
   script bodies that can be large and rarely useful for high-level questions.
4. **Use the parent table for columns.** Column lineage via `search_lineage_entities` is
   unreliable — go through the table.
