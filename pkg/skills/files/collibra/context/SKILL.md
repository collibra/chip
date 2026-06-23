---
description: Extract governed metadata from Collibra using Context Specifications, reusable blueprints that define subsets of the Knowledge Graph. Three-tool workflow for discovering, inspecting, and executing metadata extractions for any target system (Snowflake, Databricks, AI agents, custom workflows).
related: collibra/discovery, collibra/index
---

# Context generation: extract shaped metadata for any target system

A **Context Specification** is a blueprint that defines a governed subset of Collibra's Knowledge Graph. Starting from an asset (e.g., a Table), it specifies which relations to traverse, what fields to pull, and what shape to return for any target system: Snowflake, Databricks, a data product UI, or an AI agent's reasoning process.

In other words: Context Specifications define **which slice of the Knowledge Graph matters** for a specific use case, and **what shape that slice should take** when delivered to downstream consumers.

This skill explains the three-tool workflow: **discover** which specs are available, **inspect** a spec if needed, and **execute** it to get the shaped output.

---

## The three tools

| Tool | Purpose | Returns | When required |
|---|---|---|---|
| `listContextSpecifications` | Discover which Context Specifications (Knowledge Graph blueprints) are available for an asset or asset type | List of spec names, descriptions, and IDs | Always, entry point |
| `getContextSpecification` | Inspect a spec's blueprint: which relations it defines, what fields it extracts from the Knowledge Graph, what transforms it applies | Complete YAML mapping and spec metadata | Optional, only when user asks what a spec covers |
| `getContext` | Execute a spec's blueprint against an asset to extract and shape its governed metadata subset | Structured metadata (JSON, YAML, etc.) shaped for the target system | Always, output step |

---

## Decision rule: which tool to call first

Always start with `listContextSpecifications`. It takes one of two parameters:

### Option A: You have an asset UUID
Call `listContextSpecifications(assetId=<UUID>)` to find specs whose source asset type matches that asset's type.

**Use when:** You've already resolved a user's request to a specific Collibra asset (e.g., "the Orders table" → resolved to UUID `abc-123`).

**Example flow:**
```
User: "Get me the semantic blueprint for the Orders table"
→ discover_data_assets(query="Orders table") → UUID = abc-123
→ listContextSpecifications(assetId="abc-123")
→ Returns: [Semantic Blueprint v1, Data Governance v2]
```

### Option B: You have an asset type PublicId
Call `listContextSpecifications(assetTypePublicId="Table")` to find specs for that asset type, without needing a specific asset UUID.

**Use when:** The user asks about an asset type in general ("What contexts exist for Tables?") or you need to show available specs before the user picks an asset.

**Example flow:**
```
User: "What metadata can I extract about Tables?"
→ listContextSpecifications(assetTypePublicId="Table")
→ Returns: [Semantic Blueprint v1, Data Governance v2, Data Quality v1]
→ Present the list with descriptions; wait for user to select one
```

### Handling multiple results

#### Exactly one spec is returned
Proceed directly to `getContext` as you have a clear path. No need to ask the user to decide.

#### Multiple specs are returned
**Don't just hand the list to the user.** Be agentic:

1. **If you have a clear task or use case**, inspect the specs (call `getContextSpecification` on each) to compare their coverage
2. **Evaluate based on your needs:**
   - Does one spec cover all the fields your task requires? (data quality scores, ownership, metrics, lineage, etc.)
   - Do any specs explicitly exclude fields you need?
   - What transformations does each apply? (e.g., case conversions, type casting)
3. **Make a recommendation with reasoning:**
   - "I found 3 specs. I recommend **'Semantic Blueprint v2'** because it includes columns with metrics, which you'll need for data quality assessment. The others exclude metrics."
   - "I found 2 specs with different scope. 'Detailed' includes lineage, 'Summary' doesn't. Which better fits your use case?"
4. **Only defer to the user if specs are equally valid** for your use case, or if the task is genuinely ambiguous

If the user has explicitly told you what they need (e.g., "I need ownership and metrics"), use that to pick the best spec. Don't ask them to choose between specs they can't evaluate.

#### Zero specs are returned
**Don't just give up.** Explain what this means and suggest a path forward:

- "No Context Specifications are currently configured for extracting [asset type] metadata. This means your organization hasn't yet created a blueprint for this asset type."
- "To enable this, a data steward would need to create a Context Specification that defines: [what fields/metadata would be useful for your task]"
- "In the meantime, you could try a related asset type (e.g., if no 'Column' specs exist, check what's available for 'Table')"
- If helpful: offer to explain what a Context Specification would need to cover for your use case

---

## When to call `getContextSpecification`

Call this tool when you (or another AI agent) need to understand the structure and coverage of a Context Specification before executing it. Examples:

**Human-initiated:**
- "Does the Semantic Blueprint context include data quality metrics?"
- "What fields does the Finance context extract?"
- "Compare these two specs: which one has more detail?"
- "Show me the mapping rules for this context"

**AI agent-initiated:**
- Before executing a context, inspect it to confirm it covers the fields you need for your task
- Compare available specs to select the best one for your use case (e.g., choosing between "Table metadata" and "Table with lineage" specs)
- Understand the transformation rules applied to data (e.g., "fields are converted to snake_case") so you can interpret the results correctly
- Determine if the spec provides provenance/ownership information you need before proceeding
- Plan downstream processing based on what the spec extracts (e.g., "this spec extracts columns as an array, so I'll iterate over them")

**Rule of thumb:** If you or another agent needs to make a decision about whether to execute a context or how to use its results, call `getContextSpecification` first. If you're simply executing a spec you've already committed to, skip this step.

When you do call it, extract and present the blueprint configuration in a readable format, explaining:
- Which asset types it starts from
- Which relations it traverses through the Knowledge Graph (e.g., "Table → Columns → Data Quality Attributes")
- What fields it extracts from that subset (e.g., name, description, ownership, metrics)
- What transformations it applies (e.g., "converts to snake_case", "casts to boolean")
- Whether it includes provenance metadata (spec name, execution timestamp)

---

---

## Common Use Cases

### Use Case 1: Export Data Product as Snowflake Semantic View

A data engineer needs to generate a Snowflake Semantic View from a Data Product.

```
1. Find Data Product
   search_asset_keyword(query="customer") → {assetId: "dp-001"}

2. List available contexts
   listContextSpecifications(assetId="dp-001") 
   → Finds "Data Product to Snowflake Semantic View" spec

3. Execute context
   getContext(assetId="dp-001", contextSpecificationId="snowflake-spec")
   → Returns YAML formatted for Snowflake Semantic View

4. Deploy to Snowflake
```

**Key point:** The context's `targetSchema="Snowflake Semantic View"` ensures output matches Snowflake's format exactly.

---

### Use Case 2: AI Agent Chains Context Calls for Progressive Exploration

An AI agent explores a Data Product by chaining context calls.

```
1. Get Data Product overview
   getContext(assetId="dp-001", contextSpecificationId="product-basic")
   → Returns product metadata + related metric UUIDs

2. Evaluate: Did context include metric UUIDs?
   ✓ Yes → proceed to step 3
   ✗ No → ask user or work with current data

3. Drill into metric details (if UUIDs available)
   getContext(assetId="metric-001", contextSpecificationId="metric-details")
   → Returns metric definition, calculation rules, source tables

4. Evaluate: Can I chain further?
   ✓ If context returned table UUIDs → call table lineage context
   ✗ If not → proceed with current information
```

**Key point:** Agent must evaluate what each context returns to decide if chaining is possible. Missing UUIDs mean dead ends; the agent adapts accordingly.

---

### Required parameters
`getContext` requires both:
- `assetId`: the UUID of the specific asset to extract from
- `contextSpecificationId`: the UUID of the Context Specification to execute

### Optional: includeMetadata
- `includeMetadata: false` (default): Returns only the extracted metadata shaped for the target system. Use this for most cases, it's what downstream systems (Snowflake, Databricks, AI agents) actually consume.
- `includeMetadata: true`: Includes provenance alongside the content (spec name, asset type, execution timestamp). Use this only when you need to surface context and traceability to the user.

---

## Typical workflows

### Workflow 1: "Give me the full context for a specific asset"

```
User: "Get me the semantic blueprint for the Orders table"

1. discover_data_assets(query="Orders table") 
   → Returns: assetId = "abc-123", type = "Table"

2. listContextSpecifications(assetId="abc-123")
   → Returns: [Semantic Blueprint v1]

3. getContext(assetId="abc-123", 
              contextSpecificationId="spec-456",
              includeMetadata=false)
   → Returns: {name: "Orders", description: "...", columns: [...], ...}

4. Return the shaped metadata to the user
```

### Workflow 2: "What contexts are available for a given asset type?"

```
User: "What metadata can I extract about Tables?"

1. listContextSpecifications(assetTypePublicId="Table")
   → Returns: [Semantic Blueprint v1, Data Governance v2, Data Quality v1]

2. Present the specs to the user with descriptions
   "You can extract metadata for Tables using one of these:"
   - Semantic Blueprint v1: Extracts name, description, columns, ownership
   - Data Governance v2: Extracts governance status, owner, steward, review dates
   - Data Quality v1: Extracts quality scores, issues, validation rules

3. Wait for user to select one

4. getContext(assetId=<user-selected-asset>, 
              contextSpecificationId=<selected-spec>)
   → Return the shaped metadata
```

### Workflow 3: "Show me what a context extracts"

```
User: "What does the Semantic Blueprint context cover?"

1. getContextSpecification(contextSpecificationId="spec-456")
   → Returns: full mapping rules, description, source asset type, target format

2. Present the configuration to the user in readable format:
   "This context extracts the following from Tables:
   - name (from asset property)
   - description (from Description attribute, or Name as fallback)
   - columns (by traversing 'contains' relation)
     - each column has name, type, description, quality score
   - owner (by traversing 'owned_by' relation)
   - governance_status (from Governance Status attribute)"
```

---

## Hard rules

1. **Always resolve the asset UUID before calling `listContextSpecifications` with `assetId`.**
   If the user names an asset, use `discover_data_assets` or `search_asset_keyword` to resolve it to a UUID first. See `collibra/discovery` for patterns.

2. **Do not call `getContext` without a spec ID.**
   Always call `listContextSpecifications` first to discover and confirm which spec to use, unless the user or another agent has explicitly provided a Context Specification UUID.

3. **Inspect a spec before executing if your decision depends on its coverage.**
   If you're uncertain whether a spec provides the fields you need, or if you need to choose between multiple specs, call `getContextSpecification` first. The extra round trip is worth it if it prevents executing the wrong spec.

4. **Be intelligent when multiple specs are available.**
   Don't passively present a list and ask the user to choose. Inspect them (call `getContextSpecification`), compare their coverage against your task requirements, and either pick the best one or recommend it with reasoning. Only defer to the user if specs are genuinely equivalent or if the choice depends on subjective preferences you can't determine.

5. **Use `includeMetadata=true` sparingly.**
   Only set it to `true` when you (or a downstream consumer) explicitly needs provenance, timestamps, or traceability. For most use cases (generating YAML for export, grounding AI agents), use the default `false`.

---

## Related skills

- **`collibra/discovery`**: How to resolve asset names to UUIDs
- **`collibra/index`**: Asset type reference and relation types
