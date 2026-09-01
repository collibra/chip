# CHIP Tool Contribution Standards (for feature teams)

**Status:** Draft for Confluence
**Audience:** Any feature team adding MCP tools or skills to `collibra/chip` — data quality,
assessments, classification, lineage, writing assistant, data contracts, and the teams that follow.

CHIP is a shared MCP server. A tool you add is visible to every agent and every customer that
enables it, and it is read by an LLM that knows nothing about your domain. These standards apply to
every domain; examples are drawn from tools already in the repo.

Also applies: [`CONTRIBUTING.md`](../CONTRIBUTING.md) (Conventional Commits, tests, lint),
[`AGENTS.md`](../AGENTS.md) (stepdown rule, PR template), [`SKILLS.md`](../SKILLS.md).

---

## 1. Tool surface and scope

### 1.1 One tool per user-facing capability — no `prepare_` / `create_` pairs

Fold discovery, preview and write into a single tool with a status/confirm state machine (see 5).

Splitting couples two tools through the LLM, which is not guaranteed to call both. In Maestro,
users select individual tools rather than functional packages, so they can enable one half of a
pair and get a broken flow.

### 1.2 Ship the narrowest surface that covers the agreed use case

Read-only observability, admin CRUD, and execution/trigger tools are separate scope decisions, not
a package deal. If a capability is not in the agreed scope, do not include it "since it's cheap".

---

## 2. Data privacy — hard rule

**No tool may return live customer data (actual rows, cell values, sample records, file contents)
to the LLM.**

A "preview" that executes against the customer's source system and returns sample rows is not
acceptable, however useful it is. Replace it with a surface that returns a verdict plus an error
message and no customer data.

If you believe your tool needs to surface customer data, raise it with the CHIP maintainers before
writing the code — do not assume review will catch it.

---

## 3. Rollout gating

### 3.1 New tools go behind an experimental feature flag, off by default

Mandatory for admin/write tools (create / edit / delete), and the default expectation for
everything else until the tool set graduates.

### 3.2 One shared feature name per domain

All of a domain's tools share a single feature name, across PRs and teams. Do not add a flag per PR
or per tool. Gate the registrations as a block in `pkg/tools/register.go`:

```go
const YourDomainFeatureName = "your-domain"   // existing: "context-specifications"

if toolConfig.IsExperimentalEnabled(YourDomainFeatureName) {
    toolRegister(server, toolConfig, your_tool.NewTool(client))
    // ...
}
```

### 3.3 Register the flag in `knownExperimentalFeatures`

Add an entry in `cmd/chip/experimental.go` so `--experimental`, `--help` and the YAML config
recognize it. Help text and validation read from that map, so nothing else needs to change.

### 3.4 Add gating tests

`pkg/tools/register_test.go` must assert both directions: hidden with an empty config, visible with
the feature enabled. Follow the existing pattern.

---

## 4. Naming

### 4.1 Spell out domain abbreviations in the MCP tool name

The tool name is LLM-facing: `create_data_quality_rule`, not `create_dq_rule`. Go package
directories and identifiers may keep the short form — only the `Name` string and LLM-facing prose
need expanding.

### 4.2 Qualify names that would collide across domains

Bare nouns like "job", "run", "template", "match" and "entity" mean different things to data
quality, lineage, classification and assessments. Prefix with the domain:
`data_quality_rule_template`, `data_classification_match`.

---

## 5. Write safety: the confirm checkpoint

Applies to any tool that writes to Collibra or a downstream system.

### 5.1 Implement the confirm checkpoint in the tool, not the skill

- `confirm=false` (the default) → return a `preview` status with the composed payload and **write
  nothing**.
- `confirm=true` → perform the write.

"Review before saving" in a skill is guidance the model can skip. In the tool it is enforced.

### 5.2 The preview must echo *every* field that will be written

Otherwise the user approves something other than what gets saved. If a field is sent on write, it
appears in the preview.

### 5.3 Set MCP annotations explicitly

Set `DestructiveHint` and `OpenWorldHint` on every tool; read-only tools also set `ReadOnlyHint`
and `IdempotentHint`. `TestRegisterAll_AllToolsHaveProperAnnotations` enforces this.

---

## 6. Input validation and error handling

### 6.1 Validate everything before any network call

Return a structured `validation_error` status. Never let a preventable mistake surface as a raw
downstream 400.

### 6.2 Enforce every constraint you document

Name patterns, length caps, conditionally-required fields — if a schema, comment or skill states a
rule, `validate()` enforces it. Where code and documentation disagree, either implement the claim
or delete it; the mismatch is the defect.

### 6.3 Error messages must be self-correcting for the agent

State what is wrong *and* what would be valid, so the model can retry without a round trip to the
user.

```go
Message: fmt.Sprintf("monitorType %q is invalid. Use %q or %q.", input.MonitorType, a, b)
Message: "columnName is required for a SIMPLE_SQL rule (the single column the check targets)."
```

### 6.4 Search and list tools require at least one filter

An unfiltered call must be a `validation_error`, not a full-instance scan.

### 6.5 Be consistent with sibling tools

Two tools solving the same problem two different ways is a finding. Before inventing a pattern,
check how the nearest existing tool does it.

### 6.6 Map downstream errors to structured statuses

400 / 403 / 404 / 422 become typed outputs with readable messages, not opaque failures.

### 6.7 Never interpolate user-supplied values into a query

Filter operators, filter values, names and identifiers that reach SQL, GraphQL or a query DSL must
be validated or parameterised.

---

## 7. Tool descriptions (the LLM-facing contract)

The description is not documentation for a human who already knows the product. It is the contract
the model uses to decide whether to call your tool, when, and with what arguments. Treat it as part
of the implementation.

### 7.1 Write a full paragraph, not a one-line summary

"Creates a rule" tells the model nothing it can act on. Descriptions run several sentences — a
short paragraph, or two for a complex tool — in prose, covering:

1. **What it does**, in plain language.
2. **What object it operates on**, and how that object relates to the ones around it.
3. **When to use it and when not to**, including the tool it is most likely confused with.
4. **Prerequisites and ordering** — what must exist first, and which tool supplies those inputs.
5. **Key parameters**, especially enums and anything that changes behaviour.
6. **What it returns**, and what to do with it.
7. **Side effects, safety and permissions** — whether it writes, whether it has a confirm
   checkpoint, what access the user needs.

```go
Description: "Create a data quality rule (a single data-quality check on a table's data; Collibra calls it a 'monitor') " +
    "on an existing data quality job (a saved data-quality check on ONE database table that scans the table and runs its rules; also called a 'dataset'), identified by its job name. " +
    "monitorType is 'FREEFORM_SQL' (a full SQL query) or 'SIMPLE_SQL' (a single-column check). " +
    "The rule defaults to active and not suppressed (suppressed = kept but not scored). " +
    "Built around a confirm checkpoint: confirm=false (default) returns a PREVIEW of the rule and its SQL without creating anything — review it with the user; confirm=true creates the rule. " +
    "Returns the job name and rule name on success. " +
    "Note: requires permission to create rules on the target job.",
```

Field-level `jsonschema` tags follow the same standard — a sentence or two, not a restated field
name. Say whether it is required, what it means, its format or units, the default, and an example:

```go
JobName string `json:"jobName" jsonschema:"Required. Name of the existing data quality job the rule is attached to (a job, also called a 'dataset', is a saved data-quality check on one database table), e.g. 'PUBLIC.SAMPLE_DATASET'."`
```

Length is not the goal — decision-usefulness is. Keep whatever the model needs to choose correctly
between your tool and its neighbours, and cut the rest.

### 7.2 Write for an LLM with zero Collibra knowledge

Gloss every Collibra-coded term inline, **keeping the original term** so domain vocabulary still
matches. If a word means something specific inside Collibra and something else outside it — job,
dataset, monitor, entity, match, template — gloss it, in both the description and the field tags.

### 7.3 Each description must stand alone

Assume the model sees your tool in isolation, without its siblings and without the skill. Do not
write "companion to X" or rely on a neighbouring tool to supply context.

### 7.4 Include example user questions

Every description carries a few realistic prompts it should answer, **including vague ones**, so
behaviour can be checked by evaluation.

### 7.5 State units and semantics precisely

Ambiguity in a field description becomes a wrong write. "Tolerance" is not self-explanatory;
"number of failing records allowed before the rule is considered failed — a count, NOT a
percentage" is.

---

## 8. API selection and contracts

### 8.1 Prefer the public, versioned API of the producing service

Use the public endpoint where one exists, even if it means reworking a merged design (name-keyed
instead of id-keyed, different field names). Where only an internal endpoint exists, say so and why
in the package comment, the commit message and the PR body.

### 8.2 Derive contracts from the producing service, not from guesswork

Read the controllers / OAS spec of the service you are calling, and flag any hand-written client
built without a spec. If the schema varies by deployment (for example a GraphQL endpoint that must
be enabled on the instance), say which deployment you verified against and what may differ.

### 8.3 Add contract tests on the producing service side

CHIP cannot defend itself against a silent API change in your service. Your team owns a test in
your repo that fails when a contract CHIP depends on changes. Plan for this before the CHIP PR is
opened, and expect to be asked about it in review.

The mechanism arrives in **.09**; this section will then specify the approach, where the tests
live, and what each feature team provides.

### 8.4 Reuse the shared HTTP helper in the client layer

Do not hand-roll the marshal → request → read cycle per call when the client file already has a
`do` helper for that service.

---

## 9. Personas and permissions

Map every tool to the persona / permission model in
**[Chip Tool Personas and Permission Mapping](https://engineering-collibra.atlassian.net/wiki/spaces/AIENG/pages/19152601130/Chip+Tool+Personas+and+Permission+Mapping)**
and populate the tool's `Permissions` field:

```go
Permissions: []string{"dgc.classify", "dgc.catalog"}  // add_data_classification_match
Permissions: []string{"dgc.data-contract"}            // init_data_contract
Permissions: []string{"dgc.ai-copilot"}               // discover_data_assets
```

---

## 10. Skills

Skills are governed by [`SKILLS.md`](../SKILLS.md), plus:

### 10.1 Write a skill only for genuinely multi-tool workflows

Ordering constraints, ID bridging, error-recovery loops, format quirks. A skill that restates a
tool description should not exist. Examples that clear the bar: `collibra/lineage` (the DGC UUID ↔
lineage entity ID bridge), `collibra/asset-create` (RICH_TEXT Markdown handling, duplicate gating).

### 10.2 Register the skill in `collibra/index`

Otherwise the navigator cannot route to it.

### 10.3 Skills must not contradict each other or the tools

Cross-check every reference, including one-line summaries of other teams' skills. When a skill and
a tool disagree, the skill is wrong — fix it to match actual tool capability.

### 10.4 Document known limitations honestly

List what the workflow cannot do, in a known-limitations section: unsupported filters, missing
APIs, shapes the backend will not return.
