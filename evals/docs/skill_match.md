# `skill_match` — given that it looks, does it pick the right skill?

> Covers **steps 5-6** of the pipeline. See
> [pipeline.md](pipeline.md) for how this arm fits with the others.

## Why this task exists

The existing `data_product_create_happy_path` measures everything at once, so a
failure tells you *something* is wrong without telling you *what*. Three
independent things can break:

1. the model never looked for a skill → the **instructions text** is failing
2. it looked but picked the wrong one → the skill's **`description`** is failing
3. it picked right but executed badly → the **`SKILL.md` body** is failing

`skill_match` isolates **(2)**. It is the only task that measures whether a
skill's frontmatter `description` wins the requests it should — *and loses the ones
it shouldn't*.

### Why `skill_lookup` cannot answer this

**This is the only arm that runs `skill_selected`**, and that split is deliberate.
In `skill_lookup` the same scorer would be **confounded**: that arm ships no
system message, so the model may never look for a skill at all, and when it doesn't,
`skill_selected` fails too — not because the `description` is wrong, but because
there was nothing to route. It would measure P(looks **and** chooses right), roughly
`any_skill_discovered` × this arm's `skill_selected` — a product of two factors each
measured separately and more cleanly, blended into one number you can no longer
decompose.

`skill_match` removes that variable by handing motivation over up front, so its
`skill_selected` measures routing and nothing else. `skill_lookup` keeps
`any_skill_discovered`; the two read together give the diagnosis — see
[Reading both arms together](#reading-both-arms-together).

Second, and the bigger reason: `skill_lookup` runs only 4 unambiguous positive
cases. It deliberately excludes the distractors, and it *cannot* include the
negative case without capping its own ceiling. So three properties are measurable
only here — **precision**, **over-triggering**, and **ambiguity**.

## What it does

Eight prompts, run against chip with a read-only tool surface and a system message
that instructs skill discovery. The task measures which skill the model loads
first.

| Sample id | Prompt | Accepted skill(s) | Tests |
| --- | --- | --- | --- |
| `data-product` | Create a Collibra Data Product from the table SALES.ORDERS | `collibra/data-product-create` | recall |
| `business-term-create` | Create a **new** Business Term for Churn Rate in the Finance domain | `collibra/asset-create` | **precision** |
| `attribute-edit` | Add a definition to the Customer Lifetime Value term and mark me as steward | `collibra/asset-edit` | recall |
| `lineage-trace` | Where does the Monthly Recurring Revenue KPI come from? | `collibra/lineage` | recall |
| `lineage-impact` | If we change ORDERS.DISCOUNT_PCT, which reports and dashboards break? | `collibra/lineage` | recall (2nd traversal wording) |
| `semantic-discovery` | What customer data do we have in the catalog? | `collibra/discovery` | recall |
| `ambiguous-governance` | Set an owner for the CUSTOMER table and make it discoverable to other teams | `asset-edit` **or** `data-product-create` | ambiguity |
| `no-skill-needed` | List the asset types available in this instance | *(none — loading any is a failure)* | **over-triggering** |

### Why all eight, and why the awkward ones matter

With only `data-product` you measure **recall** — "is the right skill found when it
applies?" — and never **precision** — "is it found when it doesn't?" Those pull in
opposite directions, and that tension is the whole reason this task has the shape
it does:

> Broaden a `description` until it reliably wins its own case, and it starts
> winning its neighbours' too. Narrow it to stop poaching, and it stops firing when
> it should.

You cannot see that trade-off from a single-skill eval. Running both directions in
one task makes it visible in one run, which matters because **editing any
`description` moves both at once**.

- **`business-term-create` is the sharp one.** It contains *Create* and *new* — the
  exact vocabulary that makes `data-product-create` attractive — but the correct
  answer is `asset-create`. If `data-product-create` wins here, its description is
  too greedy, and that regression is invisible to every other arm.
- **`ambiguous-governance` accepts two answers.** "Set an owner" reads as
  `asset-edit`; "make it discoverable to other teams" reads as
  `data-product-create`. Both are defensible, so both pass. What it catches is
  landing somewhere *else* — `lineage`, `discovery` — which means the routing
  signal is noise rather than a close call. Holding an ambiguous prompt to one
  "right" answer would invent failures and push you to over-tune a description to
  win a question that has no single answer.
- **`no-skill-needed` inverts the assertion.** chip's own instructions say a single
  obvious tool call needs no skill, so here loading *anything* is the failure.
  `skill_selected` with an empty expectation passes only if nothing substantive was
  loaded (`collibra/index` is exempt — it's the documented navigator).

### Why it keeps the paraphrased system message

This arm uses the same hand-written `SYSTEM_MESSAGE` as
`tasks/data_product_create.py` — a local copy, deliberately **not** imported: that
file belongs to another author, and their edits should not silently change what
this arm measures.

> You are connected to a Collibra MCP server. Before composing any multi-step
> Collibra workflow, discover the relevant skill guide by calling
> `list_collibra_skills` and load the matching one with `load_collibra_skill`, then
> follow it.

That message is a 228-character paraphrase of the 2162-character text chip actually
ships, and it's more imperative than the real thing. For
[`skill_lookup`](skill_lookup.md) that's a defect — it short-circuits the
very thing being measured. **Here it's a feature.** This arm wants motivation held
constant at maximum so it doesn't contaminate the routing measurement, and a blunt
instruction is the most reliable way to do that.

Note what it does *not* give away: it says "load the matching **one**" without
naming which. The model still has to choose among seven skills whose descriptions
compete. Routing is genuinely tested.

### No environment setup required

Cases live inline in the task file, not in `fixtures/`, and reference **invented
assets** ("Churn Rate", "SALES.ORDERS", "the CUSTOMER table"). Nothing here needs
to exist in your Collibra instance, because routing is decided from the *wording*
of a request — the model reads "Create a Collibra Data Product from the table X"
and routes on that sentence shape regardless of what X resolves to.

This is deliberate. Sourcing the table name from
`fixtures/data_product_create.yaml` would drag in that fixture's validation, which
requires `expected.data_product_name` and `expected.grouped_tables` — write-arm
values with no bearing on routing. That would gate the cheapest, most portable arm
in the harness behind its most environment-specific config. As it stands this arm
runs against any instance with the fixture left completely blank.

## How to read the results

**Read per-case, not the aggregate.** A single `skill_selected` average over eight
heterogeneous cases hides everything useful. What you want is the per-sample
breakdown in `inspect view`.

`skill_selected` records `expected`, `actual` and the full `loaded` list in its
`Score.metadata`, so the per-case results form a **confusion matrix**: for each
failure you can see *which* skill won instead. That's the actionable part — it names
the description that's stealing traffic.

| Failing case | What it implicates |
| --- | --- |
| `data-product` | `data-product-create`'s description is too weak for its own core use case |
| `business-term-create` → loads `data-product-create` | `data-product-create` is too greedy; a **precision** regression |
| `business-term-create` → loads `asset-edit` | `asset-create` vs `asset-edit` boundary is unclear |
| `attribute-edit` → loads `asset-create` | same boundary, other direction — "add a definition" read as creation |
| `lineage-trace` / `semantic-discovery` | those descriptions lose to louder neighbours |
| `ambiguous-governance` → neither accepted skill | routing is noise, not a close call |
| `no-skill-needed` → anything loaded | some description is too eager; over-triggering |

Two sanity checks worth glancing at:

- **`any_skill_discovered` should be ~100% here.** The model was explicitly told to
  discover. If it isn't, the model is ignoring a direct instruction and every
  `skill_selected` number below it is suspect — investigate that first.
- **`skill_citation_consistency`** failing means the model wrote about a skill it
  never loaded, i.e. reconstructed a procedure from the skill's *name*. That makes
  a routing "pass" untrustworthy for the wrong reason.

### Reading both arms together

Each arm scores one question — `skill_lookup` "did it look?", `skill_match` "which skill?" —
and the pair is the diagnosis:

| `any_skill_discovered` in `skill_lookup` | `skill_selected` here | Diagnosis |
| --- | --- | --- |
| 30% | 95% | The description is fine — the model just doesn't look. Fix the **instructions text** (`skills.Instructions` in `pkg/skills/register.go`). |
| 95% | 30% | It looks and still picks wrong. Fix the **`description` frontmatter**. |
| 95% | 95% | Both healthy. |

Neither number distinguishes those rows alone. Note the two arms deliberately do
**not** both run `skill_selected` — in `skill_lookup` it would blend these two factors
into one inseparable figure (see [Why `skill_lookup` cannot answer
this](#why-skill_lookup-cannot-answer-this)).

Both arms still draw their prompts from one shared `CASES` pool: `any_skill_discovered`
runs in both, so the arms only line up if the shared prompts are identical, and one
source of truth means a reworded prompt cannot drift between them unnoticed.

Across the full ladder, each arm answers one question and the first failing rung is
the one to fix:

| Arm | Question | Fix on failure |
| --- | --- | --- |
| `skill_lookup` (vs `skill_lookup_no_instructions`) | Does it look, under production conditions? | the instructions text |
| `skill_match` | Having looked, does it choose correctly — both directions? | the `description` frontmatter |
| `skill_adherence` | Having chosen, does it follow the procedure? | the `SKILL.md` body |
| `data_product_create_happy_path` | Does the whole thing work end to end? | headline number; diagnose via the rungs above |

Work top-down. A failing `skill_adherence` with a failing `skill_match` above it
usually needs the routing fixed first — the body may be fine and simply never
reached.

## How it runs

```bash
inspect eval tasks/skill_arms.py@skill_match
inspect eval tasks/skill_arms.py@skill_match -T epochs=10
```

- **Repetitions:** `epochs=5` (override with `-T epochs=N`). 8 samples × 5 =
  **40 rollouts**. Inspect reduces the 5 scores per sample with the default
  **`mean`** reducer, so a case that routes correctly 3/5 times scores 0.6. With 1
  epoch every case is a coin flip and `stderr()` is undefined; 5 gives a rate you
  can compare across skill edits. `skill_match` is where you most want epochs — a
  description change that shifts a case from 100% to 60% is a real regression that
  a single run would report as a clean pass.
- **Concurrency:** Inspect uses async concurrency, not threads. `max_samples`
  defaults to **adaptive** in inspect-ai 0.3.251 — a dynamic limiter tracking
  observed rate limits (`max_connections=10` as reference). Override with
  `--max-samples N`. Epochs are **not** sequential: all 40 rollouts go into one
  concurrent queue. Safe here because nothing writes.
- **Cost:** ~2–3 tool calls per rollout thanks to the early stop below, against a
  6-tool schema instead of 32. Cheap enough to gate a PR on — which matters,
  because editing a skill `description` is the most common change and is fully
  covered by this arm.

## How it terminates, and why it can't create anything

Two independent mechanisms:

1. **It has no write tools.** chip is launched with `--enabled-tools` limited to
   six read-only tools (`list_collibra_skills`, `load_collibra_skill`,
   `search_asset_keyword`, `get_asset_details`, `get_table_semantics`,
   `list_asset_types`). `create_asset` isn't blocked — it's *absent from the
   menu*, so writing is impossible even if the model tries. This also drops ~26
   tool schemas, cutting most of the ~20k-token-per-call overhead behind the
   harness's documented 429 problem.

2. **It stops as soon as the question is answered.** An `on_continue` hook ends the
   run the moment `load_collibra_skill` returns, because that's when routing is
   decided. Without it, `react()` runs until the model calls `submit` or hits
   `message_limit` — so the model would pick its skill and then keep going, working
   through that skill's read-only phases and stopping only on reaching a write it
   can't perform. Dozens of turns, zero extra signal.

   This gate is stricter than `skill_lookup`'s, which stops on the *listing* as
   well — that arm only needs to know the model looked, whereas this one has to see
   which skill it settled on.

   A side effect worth knowing: because the run ends on the *first* load, this arm
   structurally sees at most one skill load. "Did it also load other skills?" is
   not measurable here — and in the full-flow arms extra loads are usually correct
   anyway, since `data-product-create`'s body explicitly directs the model to
   `asset-create` and `asset-edit`.

## Scorers

| Scorer | Asserts | Metric |
| --- | --- | --- |
| `skill_selected` | the **first** skill loaded is one this case accepts; `collibra/index` is exempt as the documented navigator; an empty expectation passes only if nothing was loaded; a multi-entry expectation passes on any of them | pass/fail |
| `any_skill_discovered` | `list_collibra_skills` or `load_collibra_skill` was called at all — a sanity check here, since the model was told to | pass/fail |
| `skill_citation_consistency` | the model never cites a skill in prose that it didn't actually load | pass/fail |

`skill_selected` uses *first* load rather than *any* load on purpose: a model that
loads three skills hoping one sticks has not routed correctly, even if the right
one is in the pile.

`skill_citation_consistency` catches the model reconstructing a procedure from a
skill's **name** without reading it. It scans assistant messages only: a loaded
`SKILL.md` lists siblings in its `related:` frontmatter and `list_collibra_skills`
returns every name, so scanning tool output would manufacture citations the model
never made.

## Prerequisites

```bash
go build -o .build/chip ./cmd/chip
cd evals && uv venv --python 3.12 .venv && uv pip install -r requirements.txt
export COLLIBRA_MCP_API_URL=... COLLIBRA_MCP_API_USR=... COLLIBRA_MCP_API_PWD=...
export ANTHROPIC_API_KEY=...
```

Python 3.10+ is required (the harness uses `dict | None`). `requirements.txt` pins
`mcp>=1.23.0,<2`: `mcp` 2.0.0 renamed `McpError` → `MCPError`, which inspect-ai
still imports, so an unpinned install resolves to a combination that cannot load
MCP tools at all.

Credentials are needed even though this arm never writes — chip refuses to start
without an API URL, and the model may legitimately call `search_asset_keyword` or
`get_asset_details` while orienting itself.

## Verification status

Verified: the task is discovered by `inspect list tasks`; the dataset builds with
all 7 cases; chip's allow-list restricting to exactly 6 tools with no write tools;
this arm's early-stop predicate returning `True` after a bare `list_collibra_skills`
and `False` only once `load_collibra_skill` fires (and `skill_lookup`'s stopping
on either); `skill_selected` accepting either skill on the ambiguous case and
rejecting a third; the 3 prompts shared with `skill_lookup` being byte-identical
across both arms.

## See also

- [`skill_lookup.md`](skill_lookup.md) — the arm above this one on the
  ladder: does the model look for a skill at all?
- [`skill_adherence.md`](skill_adherence.md) — the arm below: having chosen, does it
  follow the procedure? Seeds its own data and asserts on the resulting graph.

- [`pipeline.md`](pipeline.md) — the map: all eleven steps, and which arm brackets which.
