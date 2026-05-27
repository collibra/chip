# Column-level lineage workaround

The lineage tools cannot reliably reach a column directly from its DGC UUID or by name. To
trace column lineage, go through the column's **parent table**.

## Why direct column lookup fails

- `search_lineage_entities(nameContains=<column>)` does not match column entities. The filter
  only indexes table-level and higher entities.
- `search_lineage_entities(dgcId=<column UUID>)` is unreliable: there is no guaranteed mapping
  between Collibra catalog column UUIDs and the lineage graph's column entity IDs. Some
  matches resolve; others return nothing or the wrong entity.

## Workaround

Walk down from the parent table.

### Upstream column lineage

1. Resolve the **table** by name or DGC UUID:
   ```
   search_lineage_entities(nameContains="<table-name>")
   ```
   Pick the result whose `type` is `table` and whose source system matches.

2. Walk upstream from the table:
   ```
   get_lineage_upstream(entityId=<table-entity-id>)
   ```

3. In the upstream response, look for child relationships that name the specific column you
   care about. Lineage transformations between source and target tables carry column-level
   mapping in their bodies — to see those mappings, call:
   ```
   get_lineage_transformation(transformationId=<id>)
   ```
   Only do this when the user asks for the actual mapping or SQL.

### Downstream column lineage

Same shape, but with `get_lineage_downstream` from the parent table's entity ID.

## When the table-walk also fails

- If `search_lineage_entities` returns multiple tables with the same name across systems,
  pass `dgcId` (the table's catalog UUID) to narrow the result.
- If even the parent table is unregistered (e.g. a staging table that lives only in the
  lineage graph), search by name without a `dgcId`.
- If nothing matches, the asset may not be ingested into the lineage harvester yet. Tell the
  user — do not pretend the absence is a result.
