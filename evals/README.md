# Skill evals

See [../EVALS.md](../EVALS.md) for the why and the overall plan. This directory is the
harness ([Inspect AI](https://inspect.aisi.org.uk)).

## Setup (once)

```bash
# from the repo root
go build -o .build/chip ./cmd/chip

cd evals
python3 -m venv .venv
.venv/bin/pip install -r requirements.txt
```

Credentials — same resolution as chip itself: `COLLIBRA_MCP_API_URL/USR/PWD` env vars,
falling back to the repo-root `mcp.yaml`. If chip works locally, the scorers work too.

## Run the happy path

1. Fill in `fixtures/data_product_create.yaml` (source table + expected outcome).
2. ```bash
   export ANTHROPIC_API_KEY=<key>
   cd evals && source .venv/bin/activate
   inspect eval tasks/data_product_create.py --model anthropic/claude-sonnet-5
   ```
3. Browse the result:
   ```bash
   inspect view
   ```
4. Clean up before the next run (duplicate names break reruns):
   ```bash
   python scripts/cleanup.py          # dry run
   python scripts/cleanup.py --apply
   ```

Useful flags: `--epochs 3` (repeat for stability), `--limit 1`, `--log-dir logs/`.

### Hitting 429 rate limits?

Every model call carries all 28 chip tool schemas (~20k input tokens), which can
exceed a low-tier key's per-minute input-token budget outright — the run then 429s
forever, no matter how long Inspect backs off. Options, in order of preference:

1. Raise the key's tier / check workspace limits in the Anthropic console.
2. Trim chip's tool surface to what the skill needs (~halves the request size;
   slightly less faithful to real clients, which see every tool):
   ```bash
   export EVAL_ENABLED_TOOLS="list_collibra_skills,load_collibra_skill,search_asset_keyword,get_asset_details,get_table_semantics,prepare_create_asset,create_asset,edit_asset,list_data_contract,pull_data_contract_manifest,push_data_contract_manifest"
   ```

## Layout

```text
tasks/       Inspect task definitions
scorers/     end_state.py + seeded.py (REST assertions),
             trajectory.py + skill_choice.py (transcript assertions);
             the skill arms use skill_choice.py only
solvers/     seed.py — creates a throwaway star schema per rollout
fixtures/    per-environment values; read ONLY by tasks/data_product_create.py
scripts/     cleanup.py (fixture-based), teardown_seeded.py (seeded arm)
docs/        pipeline.md (start here — the map), then the per-arm writeups:
             skill_lookup.md, skill_match.md, skill_adherence.md
```

## Test arms

**New here? Read [docs/pipeline.md](docs/pipeline.md) first** — it draws the eleven
steps between a user's prompt and a created Data Product, brackets the stretch each
arm covers, and links onward to the per-arm docs.

`data_product_create_happy_path` measures everything at once, so a failure says
*something* is wrong without saying what. `tasks/skill_arms.py` splits it into
arms that each remove one layer of scaffolding — where the score drops tells you
which thing to fix.

| Arm | Scaffolding | A drop here implicates | Writes? | Rollouts |
| --- | --- | --- | --- | --- |
| `skill_lookup` | chip's **real** `initialize` instructions | `skills.Instructions` in `pkg/skills/register.go` | no | 4 × 5 |
| `skill_lookup_no_instructions` | none — tool schemas only | (the control for the above) | no | 4 × 5 |
| `skill_match` | "discover skills first" | the skill's frontmatter `description` | no | 8 × 5 |
| `skill_adherence` | skill named outright | the `SKILL.md` body | **yes** (seeds + tears down its own) | 1 × 3 |

The first two are one arm under two instruction conditions. `skill_lookup` is
the production number; the control makes it interpretable, and the *gap* is the
finding:

| control | production | Means | Do |
| --- | --- | --- | --- |
| low | high | the instructions carry the behaviour | they're load-bearing — don't casually edit |
| high | high | tool descriptions already do the work | the ~2.2k-char text is dead weight on every request |
| low | low | the text is being ignored | rewrite it |

Pass both task specs to a single `inspect eval` so they share a model, a moment
and an instance — then the gap is attributable to the instructions text alone.
Run on different days and drift between the runs lands in the gap and reads as
signal.

The two cheap arms run **different datasets**, because they ask different
questions. Both derive from one shared `CASES` pool: `skill_match` runs all of it;
`skill_lookup` runs `LOOKUP_CASES`, the subset
carrying a `request_shape` — one case per *shape* of request (multi-step write,
graph traversal, simple read), which is the only axis that changes whether a model
bothers to look for a skill. Graph traversal carries **two** prompts, because it is
the shape that failed under both instruction conditions and one prompt cannot
separate "the shape defeats the instructions" from "that wording does". Adding routing distractors there would ask the same
question again at extra cost, and adding `no-skill-needed` would be actively
wrong: not looking is the *correct* answer on that prompt, but
`any_skill_discovered` counts "never looked" as a failure, so including it caps a
perfect model at 5/6. Statistical power on this arm comes from epochs, not from
more prompts.

**Each arm scores only its own question.** `skill_lookup` runs
`skill_discovery_first`; `skill_match` runs `skill_selected`. Reading the two
together separates "never looked" from "looked and chose wrong". `skill_lookup`
deliberately does **not** run `skill_selected`: there it fails both when the model
never looked *and* when it looked and chose wrong, making it roughly
`any_skill_discovered` × `skill_match`'s `skill_selected` — a product of two factors
already measured separately and more cleanly, which cannot be decomposed once
blended.

`skill_discovery_first` rather than `any_skill_discovered` is the lookup headline
because a skill consulted *after* the first tool call did not guide it — the model
has already committed to an approach. `any_skill_discovered` cannot tell that apart
from healthy behaviour, and as a binary over "ever looked" it has little headroom:
a model that always looks eventually pins it at 1.000 whatever the instructions
say. Both scorers are kept, since discovered-but-late and never-looked need
different fixes. Details in
[docs/skill_lookup.md](docs/skill_lookup.md#why-skill_discovery_first-is-the-headline).

For the same reason each arm stops as soon as its own question is answered.
`skill_lookup` ends on the first `list_collibra_skills` **or** `load_collibra_skill`
(either answers "did it look?"); routing must see the load itself, or there is no
skill to score.

One pool rather than a list per arm keeps `any_skill_discovered` comparable across
the two — the arms only line up if their shared prompts are identical — and gives
one source of truth so a reworded prompt can't drift between them unnoticed.

```bash
# cheap: read-only, parallel-safe, no cleanup needed.
# both instruction conditions in one command, so they share a model and a moment
inspect eval tasks/skill_arms.py@skill_lookup \
             tasks/skill_arms.py@skill_lookup_no_instructions
inspect eval tasks/skill_arms.py@skill_match -T epochs=5

# expensive: writes to Collibra, but seeds unique names so it still runs in
# parallel — no --max-samples 1 needed (defaults to 3 epochs)
inspect eval tasks/skill_arms.py@skill_adherence
```

Two things make the cheap arms cheap and safe:

- **Read-only tool surface** — six tools; no `create_asset`, `edit_asset`, or
  contract push. They *cannot* mutate Collibra even if the model tries. That also
  drops 25 of chip's 31 tool schemas, cutting most of the ~20k-token per-call
  overhead behind the 429 problem above.
- **Early stop** — an `on_continue` hook ends the run the moment
  `load_collibra_skill` returns, because that is when the routing question is
  answered. Without it the model would keep going, work through the skill's
  read-only discovery phases, and stop only on reaching a write it cannot perform
  — dozens of turns that cost tokens and add no signal. Runs that never load a
  skill are *not* cut short (that would manufacture false "never looked"
  verdicts); they end via the submit tool or `message_limit`.

Being write-free is why they can run at high epoch counts, which matters because
editing a skill `description` is the most common change and is fully covered here.

### The harness's own prompt is trimmed

Every arm passes `prompt=_sequential_prompt()` to `react()`, which is Inspect's
default assistant prompt minus one sentence: *"Prioritize parallel tool calls:
when operations are independent, run them in one response."*

That instruction comes from the eval framework, not from chip, and it pushes
against the exact behaviour these arms measure — a skill is a playbook to consult
*before* acting, while batching independent calls means firing
`search_asset_keyword` in the same turn as `list_collibra_skills`, which
`skill_discovery_first` counts as not having waited. It matters for
`skill_adherence` too: parallel writes land in one message, and `write_sequence`
indexes into the call sequence to check ordering.

It is built by subtraction from upstream's own constants rather than pasted, so a
reworded default still reaches the model, and it raises if the sentence is no
longer where it expects — running with the contamination silently restored is
worse than a loud failure. **Runs from before this change are not comparable with
runs after it** on any ordering metric.

`skill_lookup` performs one throwaway MCP handshake and uses the
instructions chip actually ships (~2.2k chars) instead of the hand-written
paraphrase in `tasks/data_product_create.py` (~230 chars, more imperative, and
missing the "Exceptions:" paragraph). That is the production-fidelity number.

### Precision, not just recall

`CASES` in `tasks/skill_arms.py` includes cases where
`data-product-create` must **not** win — "Create a new Business Term for Churn
Rate" is the sharp one, since it contains *create* and *new*.

The cases are inline rather than in `fixtures/`, and every one names **invented
assets** ("SALES.ORDERS", "Churn Rate"). Routing is decided from the wording of a
request, not from whether its nouns resolve, so **neither cheap arm reads any
fixture** — both run against any instance with `fixtures/data_product_create.yaml`
left blank. Sourcing a table name from that fixture would pull in its validation of
`expected.data_product_name` and `expected.grouped_tables`, gating the harness's
most portable arms behind its most environment-specific config.

Without negatives you only measure recall, and a
`description` sharpened until it wins the happy path can start hijacking unrelated
requests. That regression is invisible to a single-skill eval and is a risk on
every skill edit.

`Case.expect` is a tuple, so a case can accept **more than one** skill.
`ambiguous-governance` ("set an owner for it and make it discoverable to other
teams") legitimately reads as either `asset-edit` or `data-product-create`, and
both pass. What it catches is landing somewhere else entirely — that means the
routing signal is noise rather than a close call. Holding an ambiguous prompt to a
single answer would invent failures.

## The adherence arm seeds its own data

`skill_adherence` does not read the fixture. Each rollout creates a throwaway
community → domain → schema → 4 tables → 18 columns in **one**
`POST /import/json-job`, then tears it down with **one** cascading
`DELETE /communities/{id}`. Full details in
[docs/skill_adherence.md](docs/skill_adherence.md).

```bash
inspect eval tasks/skill_arms.py@skill_adherence
python scripts/teardown_seeded.py            # dry run
python scripts/teardown_seeded.py --apply
```

Three problems this solves at once, all of which came from the fixture's single
hardcoded table plus a name-based assertion:

- **A false pass.** Looking a Data Product up *by name* means a leftover product
  from an earlier run satisfies the check even when this run wrote nothing (the
  agent correctly stops per hard rule 5, and still scored 4/4). `seeded_end_state`
  anchors on the seeded table's UUID and walks outward, so another run's product is
  simply unreachable. Structural, not procedural — it does not depend on a cleanup
  step having run.
- **Forced serialization.** One shared name meant concurrent rollouts raced on
  `create_asset`'s duplicate gating. Names are now unique per rollout, so this arm
  is safe to run in parallel and with `-T epochs=N`.
- **A fragile assertion.** `expected.data_product_name` required the eval to predict
  the model's generative naming. The graph walk needs no name at all.

Teardown is now **hygiene rather than correctness**: skipping it can only leave
clutter, never cause a false pass. It runs from `logs/seed-manifest.jsonl` rather
than as a task hook, because a hook does not run when a rollout crashes — exactly
when assets get left behind.

## ⚠️ `data_product_create_happy_path` must run with `--max-samples 1`

Inspect expands `samples × epochs` into **one concurrently-executed queue** — it
does not run epochs sequentially. The original fixture-based task uses a single
shared asset name, so parallel rollouts race on `create_asset`'s duplicate gating:

```bash
inspect eval tasks/data_product_create.py --epochs 3 --max-samples 1
```

`skill_adherence` needs no such flag — that is the point of seeding.
