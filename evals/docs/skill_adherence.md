# `skill_adherence` — given the right skill, is it followed correctly?

> Covers **steps 7-10** of the pipeline. See
> [pipeline.md](pipeline.md) for how this arm fits with the others.

## Why this task exists

The existing `data_product_create_happy_path` measures everything at once, so a
failure tells you *something* is wrong without telling you *what*. Three
independent things can break:

1. the model never looked for a skill → the **instructions text** is failing
2. it looked but picked the wrong one → the skill's **`description`** is failing
3. it picked right but executed badly → the **`SKILL.md` body** is failing

`skill_adherence` isolates **(3)**. The skill is named outright in the prompt, so
routing is removed from the equation and a failure here is the body's fault.

## What it does

One sample. Each rollout:

1. **Seeds** its own throwaway star schema (below).
2. Runs the agent against the full chip tool surface with the skill named.
3. **Scores** the resulting graph by walking outward from the seeded table.
4. Is torn down by `scripts/teardown_seeded.py`.

```bash
inspect eval tasks/skill_arms.py@skill_adherence
inspect eval tasks/skill_arms.py@skill_adherence -T epochs=3
python scripts/teardown_seeded.py --apply
```

### No system message

Unlike the two cheap arms, this one ships **no** `system_message`. The cheap arms
use one to tell the model to discover and load the matching skill; here the prompt
already names the exact skill, so that instruction decides nothing. It is also the
*paraphrase* of chip's shipped instructions rather than the real text (see
[skill_lookup.md](skill_lookup.md#two-conditions-two-tasks)), and this arm exists to
isolate the `SKILL.md` **body** — every extra piece of scaffolding is one more
variable that isn't it.

That makes verifying the load essential rather than optional — see
[`skill_selected` as a precondition](#skill_selected-as-a-precondition).

## Seeding: one request

Every rollout creates its own graph, named `TEST_<tag>_<static>` where `<tag>`
comes from the rollout's uuid — so parallel epochs cannot collide:

```text
TEST_<tag>_COMMUNITY
├── TEST_<tag>_DP_CATALOG              (Data Product Catalog — empty, the agent writes here)
└── TEST_<tag>_SALES_DOMAIN            (Physical Data Dictionary)
    └── TEST_<tag>_SALES               (Schema)
        ├── TEST_<tag>_ORDERS          (Table, 6 columns, has a Description)
        ├── TEST_<tag>_CUSTOMER        (Table, 4 columns)
        ├── TEST_<tag>_PRODUCT         (Table, 4 columns)
        └── TEST_<tag>_STORE           (Table, 4 columns)
```

`ORDERS` carries `CUSTOMER_ID` / `PRODUCT_ID` / `STORE_ID` deliberately: Phase 3 of
the skill picks related tables by *plain-language reasoning over names*, not by
foreign keys or lineage, so the shared key columns are what give that reasoning
something to work with.

All of it — community, domain, assets, attributes **and relations** — is one
`POST /rest/2.0/import/json-job`. Verified live: 23 assets, 22 relations, 1
attribute, 0 errors.

```text
POST /rest/2.0/import/json-job     multipart/form-data
  file            = @seed.json
  fileName        = seed.json
  continueOnError = false          # a half-seeded graph is worse than none
  simulation      = true           # optional dry run; creates nothing
→ 200 {"id": "<jobId>"}

GET /rest/2.0/jobs/{jobId}                      # poll to COMPLETED/ERROR/CANCELED
GET /rest/2.0/import/results/{jobId}/errors     # must be {"total": 0}
GET /rest/2.0/assets?domainId=<seeded domain>   # one call → every seeded UUID
```

Relations are declared **on the source asset**, keyed by relation type and
direction. This format is undocumented in the OpenAPI schema — it was confirmed
against a live instance (`RELATION added=22`):

```json
"relations": { "<relationTypeId>:TARGET": [ { "name": "...", "domain": {...} } ] }
```

### Verified type ids

All system UUIDs, i.e. out-of-the-box and stable across instances. Probed live
rather than assumed.

| Thing | Id |
| --- | --- |
| Asset type `Schema` | `00000000-0000-0000-0001-000400000002` |
| Asset type `Table` | `00000000-0000-0000-0000-000000031007` |
| Asset type `Column` | `00000000-0000-0000-0000-000000031008` |
| Domain type `Physical Data Dictionary` | `00000000-0000-0000-0000-000000030011` |
| Domain type `Data Product Catalog` | `00000000-0000-0000-0000-000000050010` |
| `Description` attribute type | `00000000-0000-0000-0000-000000003114` |
| `Schema` **contains** `Table` | `00000000-0000-0000-0000-000000007043` |
| `Column` **is part of** `Table` | `00000000-0000-0000-0000-000000007042` |
| `Data Product Port` **is implemented as** `Table` | `00000000-0000-0000-0000-000000050042` |
| `Data Product` **exposes data as** `Port` | `00000000-0000-0000-0000-000000050040` |
| `Data Contract` **governs functioning of** `Port` | `00000000-0000-0000-0000-000000050044` |

**Direction is not guessable and matters.** `Table→Column` returns *zero* relation
types — the only one is `Column is part of Table`, with the **column** as source.
Same story for ports: the *Port* is the source of `is implemented as`, and the
*Data Product* is the source of `exposes data as`. Getting any of these backwards
produces a silently empty graph.

## The agent needs somewhere to write

`Data Product`, `Data Product Port` and `Data Contract` are only allowed in a
domain of type **Data Product Catalog**. On an instance with none, every
`prepare_create_asset` comes back with

```text
"domain is required for asset type \"Data Product\". Pick one from domainOptions
 and call again. Filtered to Data Product Catalog domains."   domainOptions: []
```

and the agent cannot reach Phase 7 at all. So the seed creates an **empty
`TEST_<tag>_DP_CATALOG` domain** alongside the source domain. Its only job is to
exist. `seed()` verifies it resolves after the import and fails loudly if not —
otherwise a missing domain type would surface as the agent finding nowhere to
write, and the arm would report that as a `SKILL.md` failure.

### Why the prompt names it

Phase 4 has the **user** pick the target domain from every domain that accepts a
Data Product, never defaulting silently. Under parallel epochs that list contains
*every other rollout's* seeded catalog, so the agent could write into one of them.
The prompt therefore supplies the domain, exactly as it already supplies the
user's sign-off — `{target_domain}`, substituted by `seed_star_schema` at run
time.

Two consequences worth knowing:

- **Everything lands inside the seeded community**, so the teardown cascade
  removes it and the sweep stage below becomes a fallback rather than the main
  path.
- **The arm no longer tests whether the agent can find a catalog domain unaided.**
  That is a deliberate trade: domain choice is Phase 4 *user input*, and what this
  arm measures is Phase 7's writes and relations.

## Scoring: anchored on the seeded table, never on a name

`seeded_end_state` starts from `metadata["seed"]["fact_table_id"]` and walks
outward. Four equally-weighted checks:

| Check | How |
| --- | --- |
| `port_exposes_table` | a `Data Product Port` implements the seeded fact table |
| `product_exposes_port` | a `Data Product` exposes that Port |
| `tables_grouped` | the Port also implements **every** seeded dimension table, compared by UUID |
| `contract_governs` | a `Data Contract` governs the Port |

Why anchored rather than name-based — this is the whole reason the arm was
rewritten. The original looks a Data Product up **by name**, so a leftover product
from an earlier run satisfies `product_exists` even when the current run wrote
nothing at all: the agent correctly stops per hard rule 5 ("existing coverage is a
stop condition"), writes nothing, and still scores 4/4. A false pass in the worst
direction. Anchoring makes that impossible rather than merely unlikely — another
run's product is not reachable from *this* run's table, with no dependence on a
cleanup step having run.

Both directions are verified: the scorer returns **0.0** on a freshly seeded graph
with no agent run, and **1.0** once a Port/Product/Contract are wired up.
`grouped_fraction` is reported in metadata so a partially-grouped result is visible
even though the check itself is boolean.

## `skill_selected` as a precondition

This arm also runs `skill_selected` — **not** as a routing measurement (routing is
handed over by the prompt) but as a check on the arm's own premise. It should read
**~100%**; a dip means the model never loaded the skill it was told to use.

That matters because without it the failure is invisible. If the model works from
the skill's *name* rather than its contents, the arm silently stops measuring the
`SKILL.md` body and starts measuring what the model can do unaided — and every
other scorer still reports a normal-looking number. Nothing else catches it:
`skill_citation_consistency` only fires when the model *cites* a skill it never
loaded, so a model that simply stays quiet about it passes.

It is the exact counterpart of `any_skill_discovered` in
[`skill_match`](skill_match.md): a sanity check that should be pinned at the
ceiling, and whose dip invalidates everything measured beside it.

`skill_selected` checks *which* skill was loaded but not *when*, so this arm also
runs **`skill_discovery_first`**, which requires the load to be the opening move
with nothing beside it.

Read it as **informational here, not as a premise check.** It is strict about
batching: a model that calls `load_collibra_skill` and `search_asset_keyword` in
the same turn fails it, and that has happened on a rollout which then followed
Phase 7 correctly and scored 4/4 on `seeded_end_state`. So a low number does not
invalidate the body measurement on this arm — it says the model started resolving
the table in parallel with reading the playbook, which is a different (and milder)
concern than never reading it.

The premise that actually matters here is covered elsewhere: `skill_selected`
confirms the named skill was loaded at all, and `write_sequence` confirms nothing
was written before the procedure ran.

## `columns_discovered` — a check that exists because the skill has a bug

[SKILL.md:48](../../pkg/skills/files/collibra/data-product-create/SKILL.md#L48)
tells the model to read the column list "from the **outgoing** `Column` relations".
That is wrong. There is no `Table→Column` relation type in Collibra's model at all;
the only one is `Column is part of Table` with the column as source. So columns
always arrive as **incoming** relations, and `get_asset_details` returns them under
`incomingRelations`.

A model following Phase 1 literally looks in `outgoingRelations`, finds nothing,
and concludes the table has no columns. That breaks Phase 2's Data Set check and
strips the column context Phase 1 is supposed to feed into generated attributes.

Crucially, **none of the four end-state checks would notice** — ports, product,
contract and grouped tables do not depend on columns. So the bug degrades quality
invisibly, which is exactly why it needs its own check. `columns_discovered` passes
if the agent names ≥2 of the seeded fact table's columns in its own prose;
deliberately lenient, since the skill only needs "a few" columns for the Data Set
check.

The skill is **intentionally left unfixed** so the eval demonstrates the bug.

## `write_sequence` — the skill's write order, not just its end state

A **transcript** check, so it catches what `seeded_end_state` cannot: a run that
arrives at the right final graph by a route the skill forbids. It asserts the two
orderings Phase 7 states outright:

| Assertion | Why it matters |
| --- | --- |
| Port→table `is implemented as` links precede `init_data_contract` | Phase 7: *"the tables must be linked to the Port before init so the generated manifest covers them"*. Link afterwards and the manifest silently omits them — invisible in the end state. |
| `init_data_contract` precedes `push_data_contract_manifest` | init produces the base manifest (`0.0.1`) that the push is meant to improve on (`0.0.2`). |

Plus one assertion that is not about ordering: **a rollout with no `create_asset`
wrote nothing**, which on this arm is a failure rather than a vacuous pass.

### What it deliberately does not check

It does **not** require every create to precede every relation edit. Phase 7
mandates the opposite — create Product and Port, wire their relations, *then*
create the Data Contract, then link it — so that rule fails a correct trajectory.
`scorers/trajectory.py::write_order` (PR #108, untouched and still used by that
task) enforces exactly that non-existent rule, and attributes it to "hard rule 1",
which is about **confirming once**, not about write order.

`write_order` has a second, independent problem: it matches tool names with a bare
`endswith`, so the read-only `prepare_create_asset` counts as a write. A rollout
that only prepared and created nothing therefore scores CORRECT there — the same
false-pass class `seeded_end_state` was built to eliminate, and worst on the write
arm. `write_sequence` resolves names through `_canonical` (longest match) and reads
`edit_asset`'s structured `operations` payload rather than substring-searching the
argument blob.

The name differs on purpose: the two are not interchangeable, and a shared name
across arms in a log would imply they were.

### Known limits

- **Ordering is by call index**, so a model batching writes into a single message
  defeats it. `_sequential_prompt()` removes react's parallel-tool-calls directive
  partly for this reason.
- **`create_asset` with `allowDuplicate=false`** is sometimes used purely to
  resolve a name to a UUID; it returns `duplicate_found` and writes nothing, yet
  still satisfies the no-writes guard. `seeded_end_state` is the robust check for
  "nothing was written".
- **Manifest *content* is not checked** — including the version number, so pushing
  `0.0.1` instead of the prescribed `0.0.2` passes.

## Teardown: two stages, and the order matters

```bash
python scripts/teardown_seeded.py           # dry run, lists everything
python scripts/teardown_seeded.py --apply
python scripts/teardown_seeded.py --tag <tag> --apply
```

1. **Sweep the model-created assets** — Data Contract → Data Product → Port. Now
   that the prompt names a seeded target domain these normally land *inside* the
   seeded community, so stage 2 already covers them and this stage is a
   **fallback**: it catches a run that wrote somewhere else anyway, which is still
   possible since nothing forces the agent to obey the named domain. It is only
   able to find them by navigating from the seeded fact table, so it must run
   **first** — once the community is gone, so is the anchor.
2. **One cascading delete** — `DELETE /rest/2.0/communities/{id}` removes both
   domains and every seeded asset.

Driven by `logs/seed-manifest.jsonl`, written at seed time, rather than a post-run
hook — a hook does not run when a rollout crashes, which is precisely when assets
are left behind.

Teardown is **hygiene, not correctness**. Because names are unique per rollout, a
skipped teardown can only leave clutter; it can no longer cause a false pass.

## How it runs

- **Repetitions:** `epochs=3` by default; `-T epochs=N` to change. Three rather
  than the cheap arms' five because a rollout here costs ~100× one of theirs — but
  not one, because a single rollout gives an outcome rather than a rate, and both
  `seeded_end_state` and `columns_discovered` declare `stderr()`, which is
  undefined at n=1.
- **Parallelism:** unlike the fixture-based task this arm needs **no
  `--max-samples 1`** — each rollout seeds its own uniquely-named graph, so
  concurrent rollouts cannot collide on `create_asset`'s duplicate gating. That is
  the point of the seeding design.
- **Cost:** the full flow is ~30–50 tool round-trips against all 31 tool schemas,
  plus ~4 seeding requests and a few seconds of job polling — call it $5–15 per
  rollout before prompt caching, so ~$15–45 at the default 3 epochs, and 23 seeded
  assets per rollout for teardown to sweep. This is the expensive arm; the cheap
  ones ([lookup](skill_lookup.md), [match](skill_match.md)) are where a
  PR gate belongs.
- `message_limit=120` is a runaway-loop backstop.

## No fixture

This arm reads no per-environment values. `fixtures/data_product_create.yaml`
remains only for `tasks/data_product_create.py` (PR #108, untouched). Note that its
committed `source_table` UUID 404s on at least one dev instance, so that task is not
portable between environments — another reason the adherence arm seeds instead.

## Verification status

Verified live: one-request import (23 assets / 22 relations / 1 attribute / 0
errors); relation directions as chip reports them (`incomingRelations` carries the
Schema *and* all columns); prompt substitution of `{source_table}` at setup time;
`seeded_end_state` scoring 0.0 without an agent graph and 1.0 with one; the teardown
sweep finding Contract/Product/Port in dependency order; the cascade leaving 0
`TEST_*` assets and 0 `TEST_*` communities.

**Not yet run:** a full agent rollout, which needs `ANTHROPIC_API_KEY` and API
spend. The scorers are verified against real Collibra graphs, not against real model
output.

## See also

- [`skill_lookup.md`](skill_lookup.md) — does the model look for a skill at all?
- [`skill_match.md`](skill_match.md) — having looked, does it choose correctly?

- [`pipeline.md`](pipeline.md) — the map: all eleven steps, and which arm brackets which.
