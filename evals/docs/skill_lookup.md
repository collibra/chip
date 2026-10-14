# `skill_lookup` — does the model reach for a skill on its own?

> Covers **steps 0-4** of the pipeline. See
> [pipeline.md](pipeline.md) for how this arm fits with the others.

## Why this task exists

The existing `data_product_create_happy_path` measures everything at once, so a
failure tells you *something* is wrong without telling you *what*. Three
independent things can break:

1. the model never looked for a skill → the **instructions text** is failing
2. it looked but picked the wrong one → the skill's **`description`** is failing
3. it picked right but executed badly → the **`SKILL.md` body** is failing

`skill_lookup` isolates **(1)**. It is the only task that can detect a skill
that is perfectly written and perfectly named but never triggers — which is
arguably the most important failure mode, since a skill that never loads is
indistinguishable from a skill that doesn't exist.

### Why the original task cannot answer this

`tasks/data_product_create.py` injects this system message before every run:

> You are connected to a Collibra MCP server. Before composing any multi-step
> Collibra workflow, discover the relevant skill guide by calling
> `list_collibra_skills` and load the matching one with `load_collibra_skill`,
> then follow it.

That is a direct order to go and discover a skill. So the original task **cannot**
measure whether the model would have done so unprompted — motivation is handed to
it for free, and its `skill_loaded` scorer can only ever confirm that the model
obeyed an explicit instruction. Every run starts past the point this task is
trying to measure.

One distinction worth keeping straight, because it decides what each arm is good
for. The message says "load the matching **one**" without naming which, so the
model still has to choose `data-product-create` over `asset-create`, `asset-edit`,
`discovery` and the rest. **Routing is still genuinely tested there.** It is only the *decision to look* that the prompt short-circuits — which is exactly the gap this task
fills, and why `skill_match` reuses that same message rather than removing it.

The message is also a paraphrase rather than the real thing, which matters
separately — see [Two conditions, two tasks](#two-conditions-two-tasks).

## What it does

Four prompts, run against chip with a read-only tool surface, measuring whether
the model reaches for `list_collibra_skills` / `load_collibra_skill` — and whether
it does so **before** taking any other action. Which skill it picks is
[deliberately not scored here](#why-this-arm-does-not-run-skill_selected).

| Sample id | Request shape | Prompt | Expected skill |
| --- | --- | --- | --- |
| `data-product` | multi-step write | Create a Collibra Data Product from the table SALES.ORDERS | `collibra/data-product-create` |
| `lineage-trace` | graph traversal | Where does the Monthly Recurring Revenue KPI come from? | `collibra/lineage` |
| `lineage-impact` | graph traversal | If we change ORDERS.DISCOUNT_PCT, which reports and dashboards break? | `collibra/lineage` |
| `semantic-discovery` | simple read | What customer data do we have in the catalog? | `collibra/discovery` |

### Why four, and why these four

This arm asks *"does the model consult a skill before acting?"* — a question that
varies with the **shape** of the request (how visible the multi-step-ness is), not
with which skill happens to be correct. So it runs one case per shape. Adding more
prompts would ask the same question again at extra cost; statistical power here
comes from **epochs**, not from sample count.

**Graph traversal is the deliberate exception, with two prompts.** One prompt per
shape cannot separate "this shape defeats the instructions" from "this sentence
does", and traversal is the shape where that ambiguity matters most, so it carries
a second wording. `lineage-impact` differs on every axis available — downstream
rather than upstream, impact-analysis framing, a column rather than a KPI as the
subject — so when the two agree the result is about the shape, not the phrasing.
The other shapes get a second prompt only when their result becomes contested
too.

`LOOKUP_CASES` is therefore a subset of the shared `CASES` pool — those
carrying a `request_shape`. Two kinds of case are deliberately excluded:

- **Routing distractors** (`business-term-create`, `attribute-edit`,
  `ambiguous-governance`) exist to test *precision*, which this arm doesn't
  measure. They belong to `skill_match`.
- **`no-skill-needed`** ("List the asset types available in this instance") would
  be **actively wrong** here. chip's own instructions say a single obvious tool
  call needs no skill, so *not* looking is the correct behaviour on that prompt —
  but `any_skill_discovered` scores "never looked" as INCORRECT. Including it
  would cap a perfectly-behaving model at 5/6 and make the arm's ceiling a lie.

### No environment setup required

Cases are defined inline in the task file rather than in `fixtures/`, and reference
**invented assets** ("SALES.ORDERS", "Monthly Recurring Revenue"). Nothing here
needs to exist in your Collibra instance: whether the model reaches for a skill is
decided from the *wording* of the request, not from what its nouns resolve to.

This arm reads no fixture at all. Sourcing a table name from
`fixtures/data_product_create.yaml` would drag in that fixture's validation, which
requires `expected.data_product_name` and `expected.grouped_tables` — write-arm
values irrelevant here — and would gate the cheapest, most portable arm in the
harness behind its most environment-specific config. It runs against any instance
with that fixture left completely blank.

### Relationship to `skill_match`

`skill_match` runs all **eight** cases — including the distractors, the ambiguous
case and the negative — because precision and over-triggering are exactly what it
measures. The two arms divide the work cleanly: **this arm scores whether the model
looked; routing scores which skill it chose.** Read together they give a diagnosis
neither gives alone:

| `skill_discovery_first` here | `skill_selected` in `skill_match` | Diagnosis |
| --- | --- | --- |
| 30% | 95% | The `description` is fine — the model just doesn't look. Fix the **instructions text**. |
| 95% | 30% | It looks and still picks wrong. Fix the **`description` frontmatter**. |
| 95% | 95% | Both healthy. |

### Why this arm does *not* run `skill_selected`

It would be a confounded number here. With no system message the model may never
look for a skill, and when it doesn't, `skill_selected` fails too — not because the
`description` is wrong, but because there was nothing to route. So in this arm it
measures P(looks **and** chooses right), which is approximately
`any_skill_discovered` × `skill_match`'s `skill_selected`: a product of two factors that
are each measured separately and more cleanly, blended into one number that can no
longer be decomposed. `skill_match` also runs eight cases against this arm's four, so it
is the better place to measure routing regardless.

That is also why the run stops at the *listing*: once "which skill" is out of
scope, `list_collibra_skills` on its own answers this arm's question and the
subsequent load is wasted spend.

One shared `CASES` pool rather than a list per arm keeps `any_skill_discovered`
comparable across the two — the arms only line up if their shared prompts are
identical — and gives one source of truth, so a reworded prompt cannot drift
between them unnoticed.

See [`skill_match.md`](skill_match.md) for that arm's cases, its per-case
failure-mode table, and how to read the full arm ladder together, and
[`skill_adherence.md`](skill_adherence.md) for the rung below — which seeds its own
Collibra data so it can assert on a graph rather than a name.

## Two conditions, two tasks

```bash
# both, in one command — same model, same moment, two clean logs
inspect eval tasks/skill_arms.py@skill_lookup \
             tasks/skill_arms.py@skill_lookup_no_instructions

inspect eval tasks/skill_arms.py@skill_lookup           # production only
```

| Task | System message | What the number means |
| --- | --- | --- |
| `skill_lookup` | chip's **real** `initialize` instructions, fetched live over MCP | **Production fidelity.** Reproduces what an actual Claude Code user is in. |
| `skill_lookup_no_instructions` | **nothing** — tool schemas only | **Control.** Do the two skill tools' own descriptions attract a cold model with nothing telling it skills exist? |

### Why the control earns its cost

`skill_lookup` alone tells you *whether* it works, not *why*. It's a drug
trial: 90 recoveries out of 100 means nothing until you know how many recover
untreated. The gap is the finding:

| control | production | Means | Do |
| --- | --- | --- | --- |
| low | high | the instructions carry the behaviour | load-bearing — don't casually edit |
| high | high | the tool descriptions already do the work | the ~2.2k-char text is dead weight on every request |
| low | low | the text is being ignored | rewrite it |

Passing both task specs to one `inspect eval` keeps that comparison controlled:
same model, same moment, same instance, one variable. Run them on different days
and any drift between them — a model snapshot, a skill edit — lands in the gap and
reads as signal.

### Why two tasks rather than one

An earlier version crossed the 3 cases with both conditions into a 6-sample
dataset. That is more precise on paper — one log, guaranteed-identical
conditions — but varying a system message *per sample* is surprisingly expensive
in machinery: `system_message()` is task-level, so it needs a custom solver
reading `metadata`; and Inspect fixes a scorer's metrics at decoration time, so
reporting per-condition numbers needs a second scorer factory wrapping
`grouped(..., all=False)`. Plus the crossing loop and a mode allow-list.

Two tasks need none of that — plain `system_message()`, plain
`any_skill_discovered()`, one sample per case — and because `inspect eval` accepts
several task specs, they still run together. What you give up is the single
blended log, which you did not want anyway: an average over a control and a
production run describes neither.

The MCP handshake is `lru_cache`d, so constructing the task fetches the text once
however many times Inspect builds it.

`skill_lookup` performs one throwaway MCP handshake against `.build/chip`
and reads the `instructions` field off the `initialize` response — the exact
string `pkg/skills/register.go` ships. It raises rather than falling back to a
hardcoded copy: a silent fallback is precisely how prompt drift hides.

This matters because the harness's existing hand-written `SYSTEM_MESSAGE` is a
**228-character** paraphrase of a **2162-character** shipped text. The paraphrase
is more imperative and drops the `Exceptions:` paragraph — the documented escape
hatch that lets a model skip skill discovery. So it behaves as an *upper bound*,
not a forecast.

## How to read the results

The production number predicts real-world behaviour; no user ever connects
without server instructions. The **gap** is the causal contribution of the
instructions text — what writing it bought you.

- **production ≈ control** → the instructions are on screen and being ignored.
  Rewrite them. No other arm can tell you this, because every other arm hands the
  model a reason to look for free.
- **production high, control low** → the instructions are load-bearing. Don't
  casually edit them, and be aware that MCP clients which truncate or ignore
  server instructions give users the floor experience.
- **control already high** → the tool descriptions carry it; instructions are
  belt-and-braces.

Read `skill_discovery_first` as the headline, not `any_skill_discovered` — see
below for why the two differ.

## Why `skill_discovery_first` is the headline

Both scorers ask about the same tools; they differ only on **when**.
`any_skill_discovered` passes if the catalog was consulted at any point in the
transcript. `skill_discovery_first` passes only if that was the model's opening
move.

The stricter one is the arm's question. A skill is a playbook, so one consulted
*after* the first `search_asset_keyword` did not guide that search — the model has
already committed to an approach. `any_skill_discovered` cannot distinguish that
from healthy behaviour, and being a binary over "ever looked" it also has very
little headroom: a model that always looks eventually pins it at 1.000 whatever
the instructions say, leaving no room for a gap to appear in.

Both are kept, because read together they localise the failure:

| `discovery_first` | `any_discovered` | Diagnosis |
| --- | --- | --- |
| high | high | healthy — the playbook is consulted before work begins |
| **low** | high | the model looks, but only after acting. The instructions land, but not early enough to steer the first call. |
| low | low | the model never looks at all — the instructions are being ignored outright |

### Parallel batches count as failures

Issuing `list_collibra_skills` and `search_asset_keyword` in the **same turn**
scores INCORRECT, whichever order they appear in the batch. The model cannot have
read the playbook it is requesting in that same turn, so it did not wait — and
"did not wait" is the behaviour being measured.

That makes the metric sensitive to anything nudging the model toward batching,
which is why `_sequential_prompt()` exists (see
[How it runs](#how-it-runs)): Inspect's `react()` ships an assistant prompt
telling the model to *"prioritize parallel tool calls"*, an instruction that comes
from the eval framework rather than from chip and pushes against the very ordering
this arm looks at. Runs made before that prompt was trimmed are not comparable
with runs made after it.

## How it runs

- **Repetitions:** `epochs=5` (override with `-T epochs=N`). 4 samples × 5 =
  **20 rollouts** *per task*, so 40 when you run the pair. Inspect reduces the 5
  scores per sample with the default **`mean`** reducer, so a sample that routes
  correctly 3/5 times scores 0.6. With
  1 epoch every sample is a coin flip and `stderr()` is undefined; 5 gives a rate
  you can compare across skill edits.
- **Concurrency:** Inspect uses async concurrency, not threads. `max_samples`
  defaults to **adaptive** in inspect-ai 0.3.251 — a dynamic limiter tracking
  observed rate limits (`max_connections=10` as reference). Override with
  `--max-samples N`. Epochs are **not** sequential: all 20 rollouts go into one
  concurrent queue. Safe here because nothing writes.
- **Cost:** ~1–2 tool calls per rollout — the early stop below fires on the
  listing, so the load is never paid for — against a 6-tool schema instead of 32.

## How it terminates, and why it can't create anything

Two independent mechanisms:

1. **It has no write tools.** chip is launched with `--enabled-tools` limited to
   six read-only tools (`list_collibra_skills`, `load_collibra_skill`,
   `search_asset_keyword`, `get_asset_details`, `get_table_semantics`,
   `list_asset_types`). `create_asset` isn't blocked — it's *absent from the
   menu*, so writing is impossible even if the model tries. Verified: exactly 6
   tools registered, zero write tools. This also drops 25 of chip's 31 tool
   schemas, cutting most of the ~20k-token-per-call overhead behind the harness's
   documented 429 problem.

2. **It stops as soon as the question is answered.** An `on_continue` hook ends
   the run on the first `list_collibra_skills` **or** `load_collibra_skill` —
   either one settles "did it look?", which is all this arm measures. Both count
   because a model may skip the listing and load directly. Without the hook,
   `react()` runs until the model calls `submit` or hits `message_limit`, so it
   would work through the skill's read-only discovery phases and stop only on
   reaching a write it can't perform: dozens of turns, zero extra signal. Runs
   that never touch either tool are deliberately **not** cut short, since capping
   those would manufacture false "never looked" verdicts; they end via `submit` or
   the `message_limit=30` backstop.

   `skill_match` uses a stricter gate — it stops only on the load, because it
   needs to know *which* skill was chosen.

## Scorers

| Scorer | Asserts | Metric |
| --- | --- | --- |
| `skill_discovery_first` | the model's **first** turn was catalog discovery and nothing else — **this arm's headline** | pass/fail |
| `any_skill_discovered` | `list_collibra_skills` or `load_collibra_skill` was called at all, whenever | pass/fail |
| `skill_citation_consistency` | the model never cites a skill in prose that it didn't actually load | pass/fail |

The first two differ only in *when*, and reading them together localises the
failure:

| `discovery_first` | `any_discovered` | Diagnosis |
| --- | --- | --- |
| high | high | healthy — the playbook is consulted before work begins |
| **low** | high | the model looks, but only after acting. The instructions are not landing early enough; a skill that arrives after the first tool call did not guide it. |
| low | low | the model never looks at all — the instructions are being ignored outright |

`skill_selected` is deliberately absent — see
[Why this arm does *not* run `skill_selected`](#why-this-arm-does-not-run-skill_selected).

`skill_citation_consistency` is **near-vacuous here** and confirmed so live: the
early stop fires before the model writes any prose, so it passed on all 30
rollouts with `cited: none`. It is kept because it costs nothing and would catch
the model reading skill names out of the `list_collibra_skills` response and then
writing as though it were following one it never loaded — but do not read it as
signal on this arm.

`skill_citation_consistency` catches the model reconstructing a procedure from a
skill's **name** without reading it — which can produce a plausible transcript for
entirely the wrong reason and is invisible to the end-state and write-order
scorers. It scans assistant messages only: a loaded `SKILL.md` lists siblings in
its `related:` frontmatter and `list_collibra_skills` returns every name, so
scanning tool output would manufacture citations the model never made.

## Prerequisites

```bash
go build -o .build/chip ./cmd/chip
cd evals && uv venv --python 3.12 .venv && uv pip install -r requirements.txt
export COLLIBRA_MCP_API_URL=... COLLIBRA_MCP_API_USR=... COLLIBRA_MCP_API_PWD=...
export ANTHROPIC_API_KEY=...
```

Python 3.10+ is required (the harness uses `dict | None`). `requirements.txt` now
pins `mcp>=1.23.0,<2`: `mcp` 2.0.0 renamed `McpError` → `MCPError`, which
inspect-ai still imports, so an unpinned install resolves to a combination that
cannot load MCP tools at all.

## Verification status

Verified: all tasks discovered by `inspect list tasks`; the live MCP handshake
fetches 2162 chars of real instructions (confirmed ≠ the paraphrase, and confirmed
to contain `Exceptions:`); chip's allow-list restricting to exactly 6 tools with
no write tools; this arm's early-stop predicate returning `False` on either
`list_collibra_skills` or `load_collibra_skill` and `True` on an unrelated read
tool (and `skill_match`'s stopping only on the load); `skill_lookup` building
with a `system_message` solver and `skill_lookup_no_instructions` without one; scorer
behaviour across 26 synthetic-transcript cases.

**Verified against live rollouts:** both conditions run and score; the injected
system message is chip's real text, `Exceptions:` paragraph included; the early
stop fires on the first catalog call, so no rollout runs past the point being
measured.

**Reproducibility:** Inspect records the commit in each log's `revision` field and
flags a dirty working tree. Commit before a run whose numbers you intend to cite,
or the log cannot be tied back to a source state.
