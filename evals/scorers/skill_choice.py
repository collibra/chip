"""Transcript scorers for the skill arms: did the model reach for a skill, did
it reach for the *right* one, do its prose claims match what it actually called,
and did it write in the order the skill mandates?

These are cheap and transcript-only — no Collibra access, no writes — so the
arms that use them can run at high epoch counts in parallel.

Tool names need suffix matching because MCP clients prefix them with the server
name (`collibra_create_asset`), but plain `endswith` is wrong: both
`create_asset` and the read-only `prepare_create_asset` would match a query for
`create_asset` — and so would an underscore-boundary check, since
`collibra_prepare_create_asset` genuinely ends with `_create_asset`. See
_canonical for how that is resolved. Getting this right is what separates
write_sequence below from scorers/trajectory.py::write_order, which it replaces.
"""

import re

from inspect_ai.scorer import (
    CORRECT,
    INCORRECT,
    Score,
    Target,
    accuracy,
    scorer,
    stderr,
)
from inspect_ai.solver import TaskState

LIST_TOOL = "list_collibra_skills"
LOAD_TOOL = "load_collibra_skill"

# Skill slugs as they appear in the catalog. Matched in prose either fully
# qualified (`collibra/lineage`) or bare (`lineage`); see _cited_skills.
KNOWN_SKILLS = (
    "index",
    "discovery",
    "lineage",
    "asset-create",
    "asset-edit",
    "data-product-create",
    "context",
)

# chip tool names worth looking for in prose. Longest-first so that
# `prepare_create_asset` is preferred over `create_asset` when both match at
# the same position.
KNOWN_TOOLS = (
    "push_data_contract_manifest",
    "pull_data_contract_manifest",
    "prepare_create_asset",
    "search_asset_keyword",
    "get_table_semantics",
    "get_asset_details",
    "list_collibra_skills",
    "load_collibra_skill",
    "init_data_contract",
    "list_data_contract",
    "list_asset_types",
    "create_asset",
    "edit_asset",
)


def _canonical(function: str) -> str | None:
    """Resolve an MCP tool call's function name to a known chip tool.

    Suffix matching alone is not enough, and neither is requiring an underscore
    boundary: `collibra_prepare_create_asset` ends with `_create_asset`, so both
    would report the read-only prepare tool as a write. Resolving to the
    *longest* known tool the name ends with disambiguates the pair — and any
    future pair like it.
    """
    best: str | None = None
    for tool in KNOWN_TOOLS:
        if function == tool or function.endswith(f"_{tool}"):
            if best is None or len(tool) > len(best):
                best = tool
    return best


def _is(call, name: str) -> bool:
    """True if `call` is exactly `name`, allowing an MCP server-name prefix."""
    return _canonical(call.function) == name


def _tool_calls(state: TaskState) -> list:
    return [
        tc
        for m in state.messages
        if getattr(m, "tool_calls", None)
        for tc in m.tool_calls
    ]


def _has_operation(call, op_type: str) -> bool:
    """True if an `edit_asset` call carries an operation of `op_type`.

    Reads the structured `operations: [{"type": ...}]` payload rather than
    searching the whole argument blob as text: a `set_attribute` whose *value*
    mentions "add_relation" — a Description discussing relations, say — would
    otherwise be counted as a relation write.
    """
    ops = (call.arguments or {}).get("operations")
    if not isinstance(ops, list):
        return False
    return any(isinstance(op, dict) and op.get("type") == op_type for op in ops)


def _loaded_skills(state: TaskState) -> list[str]:
    """Skill names actually loaded, in call order. Ground truth — the call
    happened and chip returned a body."""
    loaded = []
    for call in _tool_calls(state):
        if not _is(call, LOAD_TOOL):
            continue
        name = (call.arguments or {}).get("skillName", "")
        if name:
            loaded.append(_normalize(str(name)))
    return loaded


def _normalize(skill: str) -> str:
    """`data-product-create` and `collibra/data-product-create` are the same
    skill; compare on the qualified form."""
    skill = skill.strip().strip("`'\"")
    return skill if "/" in skill else f"collibra/{skill}"


def _assistant_prose(state: TaskState) -> str:
    """Concatenated text the model wrote *itself*.

    Deliberately excludes tool result messages: a loaded SKILL.md lists its
    siblings in `related:` frontmatter and list_collibra_skills returns every
    name, so scanning tool output would manufacture citations the model never
    made.
    """
    parts: list[str] = []
    for message in state.messages:
        if getattr(message, "role", None) != "assistant":
            continue
        content = getattr(message, "content", None)
        if isinstance(content, str):
            parts.append(content)
        elif isinstance(content, list):
            # Content blocks: pull text off whichever ones carry it.
            for block in content:
                text = getattr(block, "text", None)
                if isinstance(text, str):
                    parts.append(text)
    return "\n".join(parts)


def _cited_skills(prose: str) -> set[str]:
    """Skills the model *claims* to be using.

    Qualified `collibra/<slug>` always counts. A bare slug counts only when
    it is hyphenated (`data-product-create`, `asset-edit`) — bare `lineage`,
    `discovery`, `context`, and `index` are ordinary English words that appear
    constantly in this domain and would produce nothing but false positives.
    """
    cited = set()
    for slug in KNOWN_SKILLS:
        if re.search(rf"\bcollibra/{re.escape(slug)}\b", prose):
            cited.add(f"collibra/{slug}")
        elif "-" in slug and re.search(rf"\b{re.escape(slug)}\b", prose):
            cited.add(f"collibra/{slug}")
    return cited


@scorer(metrics=[accuracy(), stderr()])
def any_skill_discovered():
    """Did the model reach for the skill catalog at all, unprompted?

    The lookup arms' headline metric: does the model look for a skill, and
    does chip's instructions text change whether it does? Each instruction
    condition is a separate task (`skill_lookup` /
    `skill_lookup_no_instructions`), so this reports one number and the comparison
    is between the two runs' numbers.
    """

    async def score(state: TaskState, target: Target) -> Score:
        calls = _tool_calls(state)
        listed = [i for i, c in enumerate(calls) if _is(c, LIST_TOOL)]
        loaded = [i for i, c in enumerate(calls) if _is(c, LOAD_TOOL)]
        if not listed and not loaded:
            return Score(
                value=INCORRECT,
                explanation=(
                    f"neither {LIST_TOOL} nor {LOAD_TOOL} called "
                    f"in {len(calls)} tool call(s)"
                ),
                metadata={"tool_calls": [c.function for c in calls]},
            )
        return Score(
            value=CORRECT,
            explanation=(
                f"{LIST_TOOL} at {listed or 'never'}, {LOAD_TOOL} at {loaded or 'never'}"
            ),
            metadata={"listed_at": listed, "loaded_at": loaded},
        )

    return score


@scorer(metrics=[accuracy(), stderr()])
def skill_discovery_first():
    """Did the model consult the catalog *before* doing any other Collibra work?

    `any_skill_discovered` asks only whether the model ever looked, and a look
    that happens after the first search still passes. That distinction is the
    whole measurement: a skill consulted after the first `search_asset_keyword`
    did not guide that search, because the model has already committed to an
    approach. It also matters for headroom — as a binary over "ever looked",
    `any_skill_discovered` pins at 1.000 for any model that looks eventually,
    leaving no room for an instructions gap to show up in.

    Scored on the first *message* carrying tool calls, not the first call, so a
    parallel batch containing both the skill lookup and a search counts as a
    failure. Issuing them together means the model never waited for the playbook,
    which is the behaviour this measures however the calls are ordered inside the
    batch.
    """

    async def score(state: TaskState, target: Target) -> Score:
        batches = [
            [_canonical(c.function) or c.function for c in m.tool_calls]
            for m in state.messages
            if getattr(m, "tool_calls", None)
        ]
        if not batches:
            return Score(
                value=INCORRECT,
                explanation="no tool calls at all",
                metadata={"first_batch": []},
            )

        first = batches[0]
        discovery = [n for n in first if n in (LIST_TOOL, LOAD_TOOL)]
        other = [n for n in first if n not in (LIST_TOOL, LOAD_TOOL)]
        meta = {"first_batch": first, "batches": batches}

        if not discovery:
            return Score(
                value=INCORRECT,
                explanation=f"first action was {other}, not skill discovery",
                metadata=meta,
            )
        if other:
            return Score(
                value=INCORRECT,
                explanation=(
                    f"skill discovery {discovery} issued in the same turn as "
                    f"{other} — the model did not wait for the playbook"
                ),
                metadata=meta,
            )
        return Score(
            value=CORRECT,
            explanation=f"first action was {discovery}",
            metadata=meta,
        )

    return score


@scorer(metrics=[accuracy(), stderr()])
def skill_selected():
    """Was the first skill loaded one this sample accepts?

    Reads `expected_skill` from sample metadata: a list of acceptable skills (a
    bare string is also accepted). Empty means "no skill applies here" and is
    correct only if the model loaded nothing. More than one entry means the
    request is genuinely ambiguous and any of them passes — holding an ambiguous
    prompt to a single answer invents failures.

    First-loaded rather than any-loaded on purpose: a model that loads three
    skills hoping one sticks has not routed correctly, even if the right one
    is in the pile. `collibra/index` is skipped — it is the documented
    navigator, so loading it on the way to the answer is correct behaviour,
    not a wrong choice.
    """

    async def score(state: TaskState, target: Target) -> Score:
        raw = state.metadata.get("expected_skill") or []
        if isinstance(raw, str):
            raw = [raw] if raw else []
        expected = {_normalize(skill) for skill in raw}

        loaded = _loaded_skills(state)
        substantive = [s for s in loaded if s != "collibra/index"]
        actual = substantive[0] if substantive else ""

        if not expected:
            ok = not substantive
            return Score(
                value=CORRECT if ok else INCORRECT,
                explanation=(
                    "no skill expected; none loaded"
                    if ok
                    else f"no skill expected but loaded {substantive}"
                ),
                metadata={"expected": [], "actual": actual, "loaded": loaded},
            )

        want = sorted(expected)
        return Score(
            value=CORRECT if actual in expected else INCORRECT,
            explanation=(
                f"expected {'any of ' if len(want) > 1 else ''}{want}, "
                f"first substantive load was {actual or 'nothing'} "
                f"(all loads: {loaded or 'none'})"
            ),
            metadata={"expected": want, "actual": actual, "loaded": loaded},
        )

    return score


@scorer(metrics=[accuracy(), stderr()])
def skill_citation_consistency():
    """Do the model's prose claims match its actual tool calls?

    Fails when the model writes about a skill it never loaded — it is then
    reconstructing a plausible-looking procedure from the skill's *name*,
    which can produce a passing end state for entirely the wrong reason and is
    invisible to both the end-state and write-order scorers.

    The converse (loaded but never named in prose) is reported but not a
    failure: silently following a loaded skill is fine.
    """

    async def score(state: TaskState, target: Target) -> Score:
        prose = _assistant_prose(state)
        cited = _cited_skills(prose)
        loaded = set(_loaded_skills(state))

        fabricated = sorted(cited - loaded)
        silent = sorted(loaded - cited)

        notes = []
        if cited:
            notes.append(f"cited: {sorted(cited)}")
        else:
            # Not a pointer to another scorer: which load-based scorer is present
            # varies by arm (skill_lookup runs any_skill_discovered, the others
            # skill_selected), and naming an absent one sends readers hunting.
            notes.append(
                "cited: none (vacuous pass — no skill claimed, so nothing to contradict)"
            )
        notes.append(f"loaded: {sorted(loaded) or 'none'}")
        if silent:
            notes.append(f"loaded but never named in prose (allowed): {silent}")

        if fabricated:
            return Score(
                value=INCORRECT,
                explanation="; ".join(
                    [f"cited without ever loading: {fabricated}"] + notes
                ),
                metadata={"fabricated": fabricated, "cited": sorted(cited),
                          "loaded": sorted(loaded)},
            )
        return Score(
            value=CORRECT,
            explanation="; ".join(notes),
            metadata={"fabricated": [], "cited": sorted(cited),
                      "loaded": sorted(loaded)},
        )

    return score


# Phase 7's relation roles, as the model passes them to edit_asset.
REL_PORT_TO_TABLE = "is implemented as"

# Init has to see the Port's table links, or the manifest it generates omits them.
INIT_TOOL = "init_data_contract"
PUSH_TOOL = "push_data_contract_manifest"


@scorer(metrics=[accuracy(), stderr()])
def write_sequence():
    """Does Phase 7 happen in the order Phase 7 requires?

    Two orderings, both stated outright in the SKILL.md body:

      * step 2 before step 4 — "the tables must be linked to the Port before init
        so the generated manifest covers them". Init reads the Port's
        `is implemented as` links to build the base manifest, so linking after
        init silently produces a manifest missing those tables.
      * step 4 before step 5 — init creates version 0.0.1 and push adds 0.0.2, so
        a push with no prior init has no base to improve on.

    Plus a guard that is not about ordering at all: a rollout with **no**
    create_asset failed to write, and on this arm that is a failure rather than a
    vacuous pass.

    Deliberately does NOT require every create to precede every relation edit.
    Phase 7 mandates the opposite: create Product and Port, wire their relations,
    *then* create the Data Contract, then link it. scorers/trajectory.py's
    write_order enforces that non-existent rule and so fails a correct
    trajectory — as does any check written from "hard rule 1", which is about
    confirming once, not about write order.

    Two known limits. Ordering is checked by call index, so a model batching
    writes into one message defeats it — see _sequential_prompt in
    tasks/skill_arms.py. And a `create_asset` sent with `allowDuplicate=false`
    purely to resolve a name returns `duplicate_found` without writing, yet still
    counts toward the guard; seeded_end_state is the robust check for "nothing was
    written".
    """

    async def score(state: TaskState, target: Target) -> Score:
        calls = _tool_calls(state)
        creates = [i for i, c in enumerate(calls) if _is(c, "create_asset")]
        prepares = [i for i, c in enumerate(calls) if _is(c, "prepare_create_asset")]
        table_links = [
            i
            for i, c in enumerate(calls)
            if _is(c, "edit_asset")
            and any(
                op.get("relationType") == REL_PORT_TO_TABLE
                for op in ((c.arguments or {}).get("operations") or [])
                if isinstance(op, dict) and op.get("type") == "add_relation"
            )
        ]
        inits = [i for i, c in enumerate(calls) if _is(c, INIT_TOOL)]
        pushes = [i for i, c in enumerate(calls) if _is(c, PUSH_TOOL)]

        if not creates:
            return Score(
                value=INCORRECT,
                explanation=(
                    f"nothing was created: no create_asset call in {len(calls)} "
                    f"tool call(s) ({len(prepares)} prepare_create_asset call(s), "
                    "which write nothing)"
                ),
                metadata={"creates": [], "prepares": prepares},
            )

        problems = []
        if inits and table_links and max(table_links) > min(inits):
            problems.append(
                f"Port linked to a table at #{max(table_links)}, after "
                f"init_data_contract at #{min(inits)} — the generated manifest "
                "cannot cover that table"
            )
        if pushes and not inits:
            problems.append(
                f"manifest pushed at #{min(pushes)} with no init_data_contract — "
                "init produces the base the push is meant to improve"
            )
        elif pushes and inits and min(pushes) < min(inits):
            problems.append(
                f"manifest pushed at #{min(pushes)} before init_data_contract at "
                f"#{min(inits)}"
            )

        return Score(
            value=INCORRECT if problems else CORRECT,
            explanation="; ".join(problems)
            or (
                f"{len(creates)} create(s) at {creates}; Port->table links at "
                f"{table_links or 'none'}; init at {inits or 'none'}; "
                f"push at {pushes or 'none'}"
            ),
            metadata={
                "creates": creates,
                "prepares": prepares,
                "table_links": table_links,
                "inits": inits,
                "pushes": pushes,
            },
        )

    return score
