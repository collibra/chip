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

Most new tools ship gated. Decide first whether the tool set needs a gate at all (3.1); a tool set
that is generally available, read-only and settled may ship ungated (`search_catalog_columns` did).
If it does need one, these are the two axes, and picking the wrong one is a review finding:

- **Experimental feature** (`--experimental=<name>`) — an opt-in *name* with no stability promise.
  Use it while a tool set is in preview: it may change shape or be removed without a deprecation
  cycle.
- **Capability flag** (a top-level boolean, e.g. `--data-quality`) — a generally available domain
  that an operator switches on as a whole, typically because it writes to (or deletes from)
  Collibra. The tool contract is stable; what is optional is whether the deployment exposes it.

A domain's registration block is gated by **one** of the two, never both. (A single preview tool
inside a generally available domain is the one exception — see the nested shape in 3.2.) A domain
has at most two gates ever: its domain gate, and one shared preview feature name used by every
preview tool in it. That holds **across PRs and teams**: reuse the names that exist rather than
adding a flag per PR or per tool, and opening a second gate for a domain is a review finding
whoever opens it.

### 3.1 Preview tool sets go behind an experimental feature flag

A new tool set whose shape is not yet settled ships behind an experimental feature name, off by
default. That is the default expectation while the domain is in preview, and it is mandatory for
admin/write tools (create / edit / delete) that are not yet generally available.

Once a tool set is generally available, it does not stay behind `--experimental`: either it needs
no gate at all, or — if it writes to Collibra and an operator should choose to expose it — it gets
a capability flag (3.2). Migrating off `--experimental` also means deleting the feature name's
entry in `knownExperimentalFeatures` once nothing gates on it (3.3).

**"Generally available" means the tool's shape is settled** — its name, input and output schema and
behaviour are ones we are willing to keep. It is a property of the tool, not of its age.

### 3.2 One gate per domain, wrapping the registrations as a block

Gate all of a domain's registrations as a single block in `pkg/tools/register.go`, in one of these
two shapes.

A preview domain, gated on an experimental feature name:

```go
const YourDomainFeatureName = "your-domain"   // existing: "context-specifications"

if toolConfig.IsExperimentalEnabled(YourDomainFeatureName) {
    toolRegister(server, toolConfig, your_tool.NewTool(client))
    // ...
}
```

A generally available domain, gated on a capability flag — a `bool` field on
`chip.ServerToolConfig`:

```go
if toolConfig.DataQuality {
    toolRegister(server, toolConfig, create_dq_job.NewTool(client))
    // ...
}
```

Wire that flag **end to end, exactly as `DataQuality` is wired**. Five touch points, none of which
the compiler checks:

1. `cmd/chip/config.go`, `initConfigOptions` — `pflag.Bool`, `viper.BindEnv`, `viper.BindPFlag`,
   `viper.SetDefault`.
2. `cmd/chip/config.go`, `printUsage` — the hand-maintained `ENVIRONMENT VARIABLES` and
   `CONFIGURATION FILE EXAMPLE` blocks.
3. `cmd/chip/config.go`, `McpConfig` — the `mapstructure` field.
4. `cmd/chip/main.go` — the assignment from `config.Mcp` into `chip.ServerToolConfig`.
5. `pkg/chip/server.go` — the `ServerToolConfig` field, its name constant (e.g.
   `DataQualityCapabilityName`) and its entry in `capabilityFields` (3.5).

Forgetting step 4 is the dangerous one: the flag parses, appears in `--help`, and silently turns
nothing on, because the struct field just stays `false` with no compile error. A test like
`cmd/chip/config_test.go`'s precedence test, plus the registration tests in 3.4, catches it.

Name the triple after the domain: `--<domain>`, `COLLIBRA_MCP_<DOMAIN>`, `mcp.<domain>` — e.g.
`--data-quality` / `COLLIBRA_MCP_DATA_QUALITY` / `mcp.data-quality`. (`--enable-debug-tools`
predates this and is not the model for the name, only for the wiring.)

Only put tools of the domain inside the block. A tool that merely sits next to them in the file
does not belong in the gate (`search_catalog_columns` is a Knowledge Graph search over catalog
Column assets, not a data quality tool, and stays ungated).

**Adding a tool to a domain that is already gated:** put the registration inside the existing
block and add no flag. A new tool inherits the domain's status; it does not need a preview gate
because it is new, and not because it writes. Add a nested preview check only when that tool's own
shape is unsettled.

**A preview tool inside a generally available domain nests its own experimental check *inside* the
domain block**, so it needs both flags:

```go
if toolConfig.DataQuality {
    // ... the generally available tools ...
    if toolConfig.IsExperimentalEnabled(YourDomainPreviewFeature) {
        toolRegister(server, toolConfig, your_preview_tool.NewTool(client))
    }
}
```

That preview name is **one per domain**, shared by every preview tool in it — not one per tool and
not one per PR. Graduating a tool means deleting its inner check; if it was the last tool gated on
that preview name, delete the `knownExperimentalFeatures` entry with it (3.3), or the map is left
advertising a name nothing gates on. Do not add the nested block, or a preview feature name, before
there is a preview tool to put in it.

### 3.3 Register an experimental feature in `knownExperimentalFeatures`

Add an entry in `cmd/chip/experimental.go` so `--experimental`, `--help` and the YAML config
recognize the name. Help text and validation read from that map, so nothing else needs to change.

This is for experimental feature names only. A capability flag is **not** an experimental feature
and must not be added to that map — the two axes are independent, and a name in that map that
nothing gates on is worse than no flag at all.

For the same reason the entry is deleted when the last registration gated on that name goes away,
whether the tool graduated (3.2) or the tool set moved to a capability flag (3.1).

### 3.4 Add gating tests

`pkg/tools/register_test.go` must assert both directions **by tool name**: every tool of the domain
absent with an empty config, every one present with the gate enabled, and the surrounding tool
surface identical in both states so the wrapper cannot have swallowed a neighbour. Follow the
existing pattern.

Assert **every combination of the gates the tools (or their skills) depend on**, not just one flag
off and on. With a nested preview check that is domain-off, domain-on/preview-off and
domain-on/preview-on; with a second capability flag the interesting cases are the mixed ones. Where
an existing test loops over the states of one flag, extend the loop rather than copying it.

### 3.5 A skill for a gated domain declares its own gate

A skill that instructs the agent to call gated tools must not be served when those tools are not
registered. Declare the capability in the skill's frontmatter — `requires: data-quality` — rather
than hardcoding skill names in Go; the catalog filters on it at load, external skills from
`--skills-dir` gate themselves the same way, and a rename cannot silently un-gate a skill.

A capability name only becomes usable in `requires:` once Go knows it: declare the name as a
constant beside its `ServerToolConfig` field (see `chip.DataQualityCapabilityName`) and add it to
`capabilityFields` in `pkg/chip/server.go`, which `chip.ServerToolConfig.CapabilityEnabled`
resolves against. Without that entry no skill can require the capability.

An unrecognized `requires:` value **fails catalog load, and so aborts startup** — deliberately,
because a skill whose gate chip does not understand would otherwise be served unconditionally.
Note the asymmetry with `--experimental`, where an unknown name only warns so that stale configs
survive: renaming or retiring a capability name is therefore a breaking change for anyone whose
`--skills-dir` skills declare it, and needs the same treatment as 3.7. Say so in `docs/CONFIG.md`
when you add a capability.

Filtering the catalog does not rewrite markdown, so remove the skill from `collibra/index` (and
from any other served skill's body or `related:` header) when its domain is gated: the navigator
must never route to a skill the configuration filtered out. The same applies to **a gated tool's
name in a served skill's body** — a guide that tells the agent to call `dq_delete_job` is just as
broken when that tool is not registered, even if no skill name is involved.
`TestServedSkillsOnlyNameRegisteredTools` in `pkg/tools/register_test.go` enforces this for every
gate state.

### 3.6 `enabled-tools` is a filter, not an escape hatch

Because a gate skips registration, `--enabled-tools` cannot re-open it — a tool inside a closed
gate never reaches `toolRegister` and therefore never reaches `IsToolEnabled`. The allow-list
selects among the tools of the enabled capabilities. Document that for your domain; do not work
around it.

### 3.7 Gating a domain that already shipped ungated

Section 3 is otherwise about new tools. Putting a gate around tools that already ship removes them
from the default surface of every existing deployment, which is a breaking change: signal it per
[`CONTRIBUTING.md`](../CONTRIBUTING.md) (a `!` after the type and/or a `BREAKING CHANGE:` footer at
the very bottom of the commit) and say in the README how an operator gets the tools back — the flag
name, its env var and its YAML field. `--data-quality` is the worked example: 18 generally
available tools left the default surface, so the change carried a `BREAKING CHANGE:` footer and a
README section naming the flag.

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

**Exception:** a read-only enumeration of a type or vocabulary catalog — one that is bounded and
small (e.g. asset types on an instance) — may accept an unfiltered call, provided it also offers
filters and reports whether a filtered result is truncated. `list_asset_types` is the instance:
unfiltered listing stays legal, `name`/`publicId`/`product` narrow it, and `resultsTruncated`
tells the caller when a `publicId`/`product` scan stopped short of the whole catalog.

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

Otherwise the navigator cannot route to it. The exception is a skill gated by a capability flag
(3.5): the index is served in every configuration, so it must not name a skill that may be
filtered out.

### 10.3 Skills must not contradict each other or the tools

Cross-check every reference, including one-line summaries of other teams' skills. When a skill and
a tool disagree, the skill is wrong — fix it to match actual tool capability.

### 10.4 Document known limitations honestly

List what the workflow cannot do, in a known-limitations section: unsupported filters, missing
APIs, shapes the backend will not return.
