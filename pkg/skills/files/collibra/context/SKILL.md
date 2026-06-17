---
description: Generate governed YAML context for a Collibra asset using Context Specifications — covers discovery, inspection, and execution of the three-tool workflow.
related: collibra/discovery, collibra/index
---

# Context generation — governed YAML output for assets

Context Specifications are governed output shapes: they define which fields, metrics, and
mappings are collected for a given asset type and render them as structured YAML. The workflow
always follows the same three-step shape, though the middle step is optional.

## The three tools

| Tool | Purpose | When required |
|---|---|---|
| `list_context_specifications` | Discover which specs are available for an asset or asset type | Always — this is the entry point |
| `get_context_specification` | Inspect a spec's full mapping configuration before executing it | Optional — only when comparing specs or explaining coverage to the user |
| `get_context` | Execute a spec against an asset and return the generated YAML | Always — this is the output step |

## Decision rule: which tool to call first

Start with `list_context_specifications`. It takes either:
- `assetId` — returns specs whose source asset type matches the type of that specific asset.
  Use this when you already have a UUID from a prior discovery step.
- `assetTypePublicId` — returns specs filtered by asset type (e.g. `"Table"`). Use this when
  the user names a type but not a specific asset.

If the list returns exactly one spec, proceed directly to `get_context` — do not pause to
ask the user which spec to use. If it returns multiple, present the names and descriptions
and ask the user to pick.

## When to call `get_context_specification`

Only call `get_context_specification` if:
- The user asks what a spec covers ("Does this context include metrics?", "What fields are
  mapped?"), or
- You need to compare two specs before recommending one.

Do not call it as a default step before every `get_context` execution — it adds a round trip
and the `mappingYaml` is not needed to generate output.

## Executing with `get_context`

`get_context` requires both `assetId` and `contextSpecificationId`. Set `includeMetadata`
to `false` (the default) for most uses — it returns the raw YAML, which is what downstream
agents and data platforms consume. Set it to `true` only when you need to surface provenance
(spec name, asset type, timestamp) alongside the content to the user.

The response may include structured warnings for partially mapped fields. Surface these to the
user rather than silently discarding them — partial output is still useful, but the user
should know which fields are missing.

## Typical workflow

```
1. User: "Get me the semantic blueprint for the Orders table"
2. discover_data_assets / search_asset_keyword → resolve "Orders table" to a UUID
3. list_context_specifications(assetId=<UUID>) → pick the right spec
4. get_context(assetId=<UUID>, contextSpecificationId=<spec UUID>) → return YAML to user
```

If the user's question is "what context specs exist for Tables?":
```
1. list_context_specifications(assetTypePublicId="Table") → list names + descriptions
2. Present the list; wait for the user to select one before executing
```

## Hard rules

1. **Always resolve the asset UUID before calling `list_context_specifications` with `assetId`.**
   See `collibra/discovery` for the name-to-UUID pattern.
2. **Do not call `get_context` without a spec ID.** Always call `list_context_specifications`
   first unless the user has explicitly provided a spec UUID.
3. **Do not silently drop warnings from `get_context`.** Report partial results and missing
   fields to the user.
4. **Present multiple specs to the user — do not guess.** If `list_context_specifications`
   returns more than one result, show names and descriptions and ask which to execute.
