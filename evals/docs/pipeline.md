# The pipeline, and which arm tests which part

Start here. This is the map; the per-arm docs are the territory.

Between a user typing a request and a Data Product existing in Collibra there are
twelve steps, and **any one of them can be the thing that broke**. Each arm of the
eval covers a different stretch, so a failing score points at a specific file to
edit rather than at "the skill doesn't work".

## 1. The pipeline

No tasks yet — just what actually happens, in order.

```text
   who              what
   ──────────────   ────────────────────────────────────────────────────────

   ── before the user types anything ──
 0  chip → model    MCP handshake: chip returns its `instructions` text
 1  chip → model    tools/list: the tool schemas reach the model

   ── the user types ──
 2  user → model    "Create a Data Product from the table SALES.ORDERS"
 3         model    decides whether this needs a skill at all
 4  model → chip    list_collibra_skills        → catalog of 7 skills
 5         model    reads the descriptions, picks one
 6  model → chip    load_collibra_skill(name)   → the SKILL.md body

   ── now following the skill's seven phases ──
 7  model → chip    Phase 1-3  locate the table, check existing
                               coverage, suggest related tables     [reads]
 8         model    Phase 4-5  collect governance metadata, propose
                               the full set of assets               [reads]
 9  user → model    Phase 6    single approval gate
10  model → chip    Phase 7    create + verify                     [WRITES]

11  model → user    final answer: "Created <product>, <port>, <contract>"
```

Three things worth noticing:

- **Steps 0-1 happen before the prompt.** The instructions text and the tool
  schemas are already in the context window when the user types.
- **Step 3 is a decision nothing forces.** No code makes the model look for a
  skill; the instructions text is only *persuasion*.
- **Only step 10 writes.** Steps 0-9 are read-only, which is what makes most of
  this pipeline cheap and safe to test.

## 2. `skill_lookup` and `skill_lookup_no_instructions` — steps 0-4

Does the model reach for a skill **before doing anything else**?

```text
 0  chip → model    MCP handshake: instructions text     ◀── THE VARIABLE
 1  chip → model    tools/list: 6 read-only tools
 2  user → model    the prompt
 3         model    decides whether this needs a skill   ◀── MEASURED HERE
 4  model → chip    list_collibra_skills                 ◀── must be the FIRST
                                                             action; run stops here
────────────────────────────────────────────────────── never reached ─────────
 5         model    picks one
 6  model → chip    load_collibra_skill
 7-10               the seven phases
11  model → user    final answer
```

**"Before anything else" is the whole point.** A model that calls
`search_asset_keyword` first and reaches step 4 afterwards has already taken its
first action unguided — so `skill_discovery_first` is the headline, not
`any_skill_discovered`, which a late look still satisfies. See
[why that is the headline](skill_lookup.md#why-skill_discovery_first-is-the-headline).

The two tasks are the **same diagram run twice**, differing only at step 0:
`skill_lookup` sends chip's real instructions text, `skill_lookup_no_instructions`
sends nothing. Neither number means much alone — the *gap* between them is what
tells you whether that text is doing any work.

A drop implicates `skills.Instructions` in
[`pkg/skills/register.go`](../../pkg/skills/register.go).

→ [**skill_lookup.md**](skill_lookup.md) for the cases, the control's
rationale, and how to read the gap.

## 3. `skill_match` — steps 5-6

Given that it looks, does it pick the **right** skill?

```text
 0  chip → model    MCP handshake
 1  chip → model    tools/list: 6 read-only tools
 2  user → model    the prompt
 3         model    decides whether this needs a skill   ◀── HANDED OVER: a system
                                                             message orders it to look,
                                                             so this is held constant
 4  model → chip    list_collibra_skills                      → 7 descriptions
 5         model    reads the descriptions, picks one    ◀── MEASURED HERE
 6  model → chip    load_collibra_skill(name)            ◀── run stops here
────────────────────────────────────────────────────── never reached ─────────
 7-10               the seven phases
11  model → user    final answer
```

Step 3 is deliberately short-circuited: motivation is pinned at maximum so it
cannot contaminate the routing measurement. Step 6 is where the run stops, because
routing needs to know *which* skill — stopping at step 4 would leave nothing to
score.

This arm runs **8 prompts, including ones where the skill must lose** — that is how
it measures precision and not just recall. A drop implicates the skill's
`description:` frontmatter.

→ [**skill_match.md**](skill_match.md) for all eight cases and the
precision/recall trade-off.

## 4. `skill_adherence` — steps 7-10

Given the right skill, is the **procedure** followed?

```text
 0  chip → model    MCP handshake
 1  chip → model    tools/list: all 31 tools (writes included)
 2  user → model    the prompt — NAMES the skill outright
────────────────────────────────────────────────────── skipped ───────────────
 3         model    decides whether this needs a skill
 4  model → chip    list_collibra_skills
 5         model    picks one
──────────────────────────────────────────────────────────────────────────────
 6  model → chip    load_collibra_skill(named skill)     ◀── premise check: must
                                                             happen, else this arm
                                                             measures nothing
 7  model → chip    Phase 1-3  locate, coverage, kin     ◀─┐
 8         model    Phase 4-5  metadata, proposal          │  MEASURED HERE
 9  user → model    Phase 6    approval (pre-granted)      │
10  model → chip    Phase 7    create + verify   [WRITES] ◀─┘
11  model → user    final answer
```

Naming the skill in the prompt deletes steps 3-5 from the run, which is exactly
what makes a failure attributable to the `SKILL.md` **body** and nothing upstream.

Because step 10 writes, this arm seeds its own throwaway star schema per rollout
and scores the resulting graph by walking outward from the seeded table's UUID —
never by looking a Data Product up by name.

→ [**skill_adherence.md**](skill_adherence.md) for the seeding, the four end-state
checks, and the teardown.

## 5. `data_product_create_happy_path` — steps 0-11

The original task (PR #108, `tasks/data_product_create.py`). It runs the whole
pipeline and gives you **one** number.

```text
 0 ─────────────────────────────────────────────────────────────────────── 11
    everything above, as a single pass/fail
```

That number answers "does the feature work?" — which is worth knowing. What it
cannot answer is "which part broke?", and that is the entire reason the arms above
exist.

## Summary

| Steps | Arm | Headline scorer | A drop implicates |
| --- | --- | --- | --- |
| 0-4 | [`skill_lookup`](skill_lookup.md) | `skill_discovery_first` | the `instructions` text |
| 0-4 | [`skill_lookup_no_instructions`](skill_lookup.md#why-the-control-earns-its-cost) | `skill_discovery_first` | *(nothing — it is the control)* |
| 5-6 | [`skill_match`](skill_match.md) | `skill_selected` | the `description:` frontmatter |
| 7-10 | [`skill_adherence`](skill_adherence.md) | `seeded_end_state` | the `SKILL.md` body |
| 0-11 | `data_product_create_happy_path` | `end_state` | *headline only* — diagnose with the rows above |

Work **top-down**: the first failing row is the one to fix, because every row below
it starts from a premise the row above just disproved.

| lookup | match | adherence | Diagnosis |
| --- | --- | --- | --- |
| 30% | 95% | 90% | The skill is fine and never gets invoked. Fix the **instructions text**. |
| 95% | 30% | 90% | It looks, then picks wrong. Fix the **`description`**. |
| 95% | 95% | 40% | Right skill, wrong execution. Fix the **`SKILL.md` body**. |
| 95% | 95% | 95% | Healthy. |

## Running it

```bash
# cheap: read-only, 6-tool surface, 1-3 tool calls per rollout
inspect eval tasks/skill_arms.py@skill_lookup \
             tasks/skill_arms.py@skill_lookup_no_instructions
inspect eval tasks/skill_arms.py@skill_match

# expensive: writes to Collibra, seeds and tears down its own data
inspect eval tasks/skill_arms.py@skill_adherence
python scripts/teardown_seeded.py --apply
```

Steps 0-6 are cheap; steps 7-10 are not (~30-50 tool round-trips against the full
tool surface, plus seeding). That asymmetry is why the arms you can afford to run
on every commit are the ones covering the top of the pipeline — which is also where
the most common edit lands, a reworded `description`.

## See also

- [`skill_lookup.md`](skill_lookup.md) — steps 0-4: does it reach for a
  skill unprompted?
- [`skill_match.md`](skill_match.md) — steps 5-6: having looked, does it choose
  correctly, and does it *avoid* choosing when it shouldn't?
- [`skill_adherence.md`](skill_adherence.md) — steps 7-10: given the right skill, is
  the procedure followed?
- [`../README.md`](../README.md) — setup, credentials, cost and rate-limit notes.
