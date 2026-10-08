"""Ablation arms for skill selection: lookup, match, adherence.

data_product_create.py measures everything at once, so a failure says
"something is wrong" without saying what. These arms each remove one layer of
scaffolding, so where the score drops tells you which thing to fix:

    skill_lookup                  chip's real instructions -> the instructions text
    skill_lookup_no_instructions  no system message        -> (the control)
    skill_match                   "discover skills first"  -> the `description`
    skill_adherence               skill named outright     -> the SKILL.md body

The first two are the same arm under two instruction conditions, and the *gap*
between their scores is the finding — see skill_lookup's docstring. Run them
in one command so they share a model and a moment.

The cheap arms make no writes (they run with a read-only tool surface), need no
cleanup, and cost 1-3 tool calls — so they run at high epoch counts in parallel
and are cheap enough to gate a PR on. Only skill_adherence is a live-write run.

    inspect eval tasks/skill_arms.py@skill_lookup \\
                 tasks/skill_arms.py@skill_lookup_no_instructions
    inspect eval tasks/skill_arms.py@skill_match -T epochs=5
    inspect eval tasks/skill_arms.py@skill_adherence -T epochs=3
"""

import asyncio
import os
import sys
from concurrent.futures import ThreadPoolExecutor
from functools import lru_cache
from pathlib import Path
from typing import NamedTuple

EVALS_DIR = Path(__file__).resolve().parents[1]
REPO_ROOT = EVALS_DIR.parent
sys.path.insert(0, str(EVALS_DIR))

from inspect_ai import Task, task
from inspect_ai.agent import AgentPrompt, react
from inspect_ai.dataset import Sample
from inspect_ai.solver import system_message
from inspect_ai.tool import mcp_server_stdio

from scorers.seeded import columns_discovered, seeded_end_state
from scorers.skill_choice import (
    LIST_TOOL,
    LOAD_TOOL,
    _canonical,
    any_skill_discovered,
    skill_citation_consistency,
    skill_discovery_first,
    skill_selected,
    write_sequence,
)
from solvers.seed import seed_star_schema

# Real MCP clients receive chip's server-level instructions, which tell the
# model to discover and load skills before multi-step workflows. Inspect does
# not surface those instructions, so we replicate the relevant part here —
# without it we would be measuring Inspect's plumbing, not the skill.
SYSTEM_MESSAGE = """You are connected to a Collibra MCP server. Before composing
any multi-step Collibra workflow, discover the relevant skill guide by calling
list_collibra_skills and load the matching one with load_collibra_skill, then
follow it."""

# skill_adherence's user prompt. `{source_table}` and `{target_domain}` are NOT
# formatted here — neither exists until the setup solver seeds it, and every epoch
# needs its own. solvers/seed.py::seed_star_schema substitutes both into the user
# message at run time, once the real names are known. Rename a placeholder here
# and you must rename its counterpart default there too.
#
# Naming the skill up front is what makes this the *adherence* arm: routing is
# handed over, so a failure is the SKILL.md body's fault. The "sign-off already
# given" clause overrides the skill's confirm-once gate, without which an
# unattended run would block forever waiting for approval.
#
# Naming the target domain stands in for the user the same way. Phase 4 has the
# user choose from every domain that accepts a Data Product, which with parallel
# rollouts in flight would include every other rollout's seeded catalog — so the
# agent could write into one of them. Supplying it removes that ambiguity instead
# of hoping the agent resolves it, and keeps every created asset inside the
# community teardown cascades.
ADHERENCE_PROMPT = """Use the collibra/data-product-create skill for this task.

Create a Collibra Data Product from the table {source_table}.

Create the Data Product, its Port and its Data Contract in the domain
{target_domain}.

Include only the dimension tables you identify with high confidence and skip
anything you would normally ask me to confirm. Treat my sign-off on your
proposal as already given — proceed with creation without waiting, and finish
by reporting the UUIDs of everything you created."""

# Enough surface to identify tables and read the catalog, with every write tool
# removed. The arms therefore *cannot* mutate Collibra and terminate naturally
# once they run out of read work — no fragile low message_limit needed to force
# an early stop (a truncated run looks identical to a failed one). Also drops 25
# of chip's 31 tool schemas, cutting the ~20k-token per-call overhead that makes
# low-tier API keys 429 forever.
READ_ONLY_TOOLS = (
    "list_collibra_skills,load_collibra_skill,search_asset_keyword,"
    "get_asset_details,get_table_semantics,list_asset_types"
)


def chip_server(enabled_tools: str | None = None):
    """chip as an MCP stdio server, with an optional tool allow-list.

    Mirrors tasks/data_product_create.py::chip_server but takes the tool set as
    an argument: the arms need different surfaces within one process, and the
    original reads the process-wide EVAL_ENABLED_TOOLS.
    """
    args = [
        "--experimental=skills",
        # Working-tree skills override the embedded catalog on name match, so
        # SKILL.md edits are picked up with no Go rebuild.
        f"--skills-dir={REPO_ROOT / 'pkg/skills/files'}",
    ]
    enabled_tools = enabled_tools or os.environ.get("EVAL_ENABLED_TOOLS")
    if enabled_tools:
        args.append(f"--enabled-tools={enabled_tools}")
    return mcp_server_stdio(
        name="collibra",
        command=str(REPO_ROOT / ".build/chip"),
        args=args,
        cwd=REPO_ROOT,  # chip resolves ./mcp.yaml from here; env vars still win
    )


@lru_cache(maxsize=1)
def _sequential_prompt() -> AgentPrompt:
    """react's own prompt, minus its "prioritize parallel tool calls" directive.

    A skill is a playbook to consult *before* acting, so an instruction to batch
    independent calls pushes directly against the behaviour these arms measure:
    it invites firing search_asset_keyword in the same turn as
    list_collibra_skills, which skill_discovery_first scores as not having
    waited. The directive comes from Inspect, not from chip, so any number it
    moves is an artefact of the harness.

    It also matters for skill_adherence: parallel writes land in one message, and
    write_sequence's ordering checks index into the call sequence.

    Built by subtraction from upstream's own constants rather than pasted, so a
    reworded default still reaches the model. Raises if the directive is no longer
    where it was, because silently running with it back in place would quietly
    reintroduce the contamination.
    """
    from inspect_ai.agent._types import (
        DEFAULT_ASSISTANT_PROMPT,
        PARALLEL_TOOLS_PROMPT,
    )

    if PARALLEL_TOOLS_PROMPT not in DEFAULT_ASSISTANT_PROMPT:
        raise RuntimeError(
            "inspect_ai's PARALLEL_TOOLS_PROMPT is no longer part of "
            "DEFAULT_ASSISTANT_PROMPT — re-check what react() now sends before "
            "trusting any ordering metric (skill_discovery_first, write_sequence)."
        )
    return AgentPrompt(
        assistant_prompt=DEFAULT_ASSISTANT_PROMPT.replace(
            f" {PARALLEL_TOOLS_PROMPT}", ""
        )
    )


def _run_sync(coro_fn):
    """Run an async function from task-construction code.

    Task functions are called synchronously, but Inspect also has an async eval
    entry point — and a bare asyncio.run() raises "cannot be called from a
    running event loop" if construction ever happens inside one. Falling back to
    a private loop on a worker thread makes this correct either way.
    """
    try:
        asyncio.get_running_loop()
    except RuntimeError:
        return asyncio.run(coro_fn())
    with ThreadPoolExecutor(max_workers=1) as pool:
        return pool.submit(lambda: asyncio.run(coro_fn())).result()


@lru_cache(maxsize=1)
def _server_instructions() -> str:
    """chip's real `initialize` instructions, fetched over MCP.

    Cached because the text is a property of the binary, not of the run, and
    reading it costs a throwaway chip process — worth doing once however many
    times a task gets constructed.

    The hand-written SYSTEM_MESSAGE in data_product_create.py is a paraphrase of
    pkg/skills/register.go's Instructions: more imperative, missing the
    exceptions paragraph, and free to drift out of sync the moment someone edits
    the Go string. Fetching the real thing measures the text that actually
    ships.

    Raises rather than falling back to the paraphrase — a silent fallback is
    exactly how the drift this avoids would hide.
    """
    from mcp import ClientSession, StdioServerParameters
    from mcp.client.stdio import stdio_client

    async def fetch() -> str:
        params = StdioServerParameters(
            command=str(REPO_ROOT / ".build/chip"),
            args=[
                "--experimental=skills",
                f"--skills-dir={REPO_ROOT / 'pkg/skills/files'}",
            ],
            cwd=str(REPO_ROOT),
        )
        async with stdio_client(params) as (read, write):
            async with ClientSession(read, write) as session:
                result = await session.initialize()
                return result.instructions or ""

    instructions = _run_sync(fetch)
    if not instructions.strip():
        raise RuntimeError(
            "chip returned empty initialize instructions — is --experimental=skills "
            "set and .build/chip current? Refusing to fall back to the hand-written "
            "paraphrase, which would silently measure the wrong text."
        )
    return instructions


class Case(NamedTuple):
    """One routing question: a prompt, and the skill(s) that should win it."""

    id: str
    prompt: str
    # Empty = no skill applies and loading one is over-triggering. More than one
    # = the request is genuinely ambiguous and any of them is a defensible read;
    # scoring it against a single "right" answer would invent a failure.
    expect: tuple[str, ...]
    # Set on the cases skill_lookup runs. That arm asks "does the model look
    # for a skill at all?", so it needs one prompt per *shape* of request and
    # nothing more — see LOOKUP_CASES below for why the rest are excluded.
    request_shape: str | None = None


# The shared pool both cheap arms draw from — the whole test, readable in one
# place. Deliberately *not* in fixtures/, and deliberately free of any
# environment-specific value: routing is decided from the *wording* of a request,
# not from whether the assets it names exist. Every case names invented assets
# ("Churn Rate", "SALES.ORDERS"), so these arms run against any Collibra instance
# with no setup.
#
# In particular these do NOT read fixtures/data_product_create.yaml. That fixture
# validates expected.data_product_name and expected.grouped_tables, which are
# write-arm concerns — sourcing a table name from it would gate the cheapest,
# most portable arms behind the most environment-specific config in the harness.
#
# One pool rather than one list per arm, because the arms are only comparable if
# their shared prompts are identical. skill_selected is read across both arms to
# separate "the model never looked" from "the model looked and chose wrong"; if a
# prompt drifted between them, each arm would still pass its own checks while that
# inference broke silently. Deriving both views from here makes drift impossible.
#
# The negatives are the point. With only the first case you measure recall ("is
# the right skill found when it applies?") and never precision ("is it found when
# it does not?"). A `description` sharpened until it wins case 1 can start
# hijacking the rest — invisible without them, and a risk on every skill edit.
CASES = [
    Case(
        id="data-product",
        prompt="Create a Collibra Data Product from the table SALES.ORDERS.",
        expect=("collibra/data-product-create",),
        request_shape="multi-step write",
    ),
    # The sharp one: contains "Create" and "new", the exact words that make
    # data-product-create attractive, but the target is a plain asset.
    Case(
        id="business-term-create",
        prompt="Create a new Business Term for Churn Rate in the Finance domain.",
        expect=("collibra/asset-create",),
    ),
    Case(
        id="attribute-edit",
        prompt=(
            "Add a definition to the Customer Lifetime Value business term and "
            "mark me as its steward."
        ),
        expect=("collibra/asset-edit",),
    ),
    Case(
        id="lineage-trace",
        prompt=(
            "Where does the Monthly Recurring Revenue KPI come from? Show me the "
            "upstream tables that feed it."
        ),
        expect=("collibra/lineage",),
        request_shape="graph traversal",
    ),
    # A second graph-traversal prompt, so the shape is not represented by a
    # single wording. With one prompt per shape a failure cannot be attributed —
    # is it the shape that defeats the instructions, or just that sentence? This
    # one differs on every axis available: downstream rather than upstream,
    # impact-analysis framing, a column rather than a KPI as the subject. When
    # the two agree, the result is about the shape and not the phrasing.
    #
    # Sharing lineage's expected skill with lineage-trace also gives skill_match a
    # second reading on that description's recall, which is a free side benefit
    # rather than the reason this exists.
    Case(
        id="lineage-impact",
        prompt=(
            "If we change the ORDERS.DISCOUNT_PCT column, which reports and "
            "dashboards break?"
        ),
        expect=("collibra/lineage",),
        request_shape="graph traversal",
    ),
    Case(
        id="semantic-discovery",
        prompt="What customer data do we have in the catalog?",
        expect=("collibra/discovery",),
        request_shape="simple read",
    ),
    # Genuinely ambiguous: "set an owner" is asset-edit, "discoverable to other
    # teams" is data-product-create. Both are defensible, so both pass — what
    # this case catches is landing somewhere else entirely (lineage, discovery),
    # which means the routing signal is noise rather than a close call.
    Case(
        id="ambiguous-governance",
        prompt=(
            "I need the CUSTOMER table properly governed — set an owner for it "
            "and make it discoverable to other teams."
        ),
        expect=("collibra/asset-edit", "collibra/data-product-create"),
    ),
    # chip's own instructions say a single obvious tool call needs no skill, so
    # loading one here is over-triggering.
    Case(
        id="no-skill-needed",
        prompt="List the asset types available in this Collibra instance.",
        expect=(),
    ),
]

# skill_lookup asks "does the model consult a skill before acting?" — a question
# that varies with the *shape* of the request (is the multi-step-ness visible?),
# not with which skill is correct. So it runs one case per shape and drops:
#
#   - the routing distractors (business-term-create, attribute-edit,
#     ambiguous-governance) — they exist to test precision, which this arm does
#     not measure, so they would only add cost;
#   - no-skill-needed — actively harmful here. Not looking is the *correct*
#     behaviour on that prompt, but any_skill_discovered scores "never looked"
#     as INCORRECT, so including it caps a perfect model at 5/6.
#
# "One case per shape" has one deliberate exception: graph traversal carries two.
# A single prompt per shape cannot separate "this shape defeats the instructions"
# from "this sentence does", so the shape whose result you most need to trust is
# worth a second wording. Give the other shapes a second prompt only when their
# result becomes contested too; until then it buys a duplicate answer, and power
# on this arm comes from epochs.
LOOKUP_CASES = [c for c in CASES if c.request_shape]


def _samples(cases: list[Case]) -> list[Sample]:
    """Cases as Inspect samples."""
    return [
        Sample(
            id=case.id,
            input=case.prompt,
            target=case.expect[0] if case.expect else "none",
            metadata={"expected_skill": list(case.expect)},
        )
        for case in cases
    ]


def _stop_once(*tool_names: str):
    """End the run as soon as one of `tool_names` has been called.

    Each arm stops when *its own* question is answered — nothing more is
    measured after that point, and letting the run continue means the model
    works through the skill's read-only discovery phases and stops only on
    reaching a write it cannot perform: dozens of turns that cost tokens and
    tell us nothing.

    Returning False from on_continue breaks react's loop; it is called every
    turn. Runs where the tool never fires are not cut short (that would
    manufacture false "never looked" verdicts) and end via submit or
    message_limit.
    """

    async def on_continue(state) -> bool:
        for message in state.messages:
            for call in getattr(message, "tool_calls", None) or []:
                if _canonical(call.function) in tool_names:
                    return False
        return True

    return on_continue


def _stop_once_skill_discovered():
    """skill_lookup's gate: the model reached for the catalog, either way.

    That arm asks only "does it look?", which `list_collibra_skills` answers on
    its own — so there is no reason to pay for the subsequent load. Either tool
    counts, because a model may skip the listing and load directly, and that
    still answers the question.
    """
    return _stop_once(LIST_TOOL, LOAD_TOOL)


def _stop_once_skill_chosen():
    """skill_match's gate: a skill was actually loaded.

    Routing needs to know *which* skill, so unlike the lookup arm it has to
    see the load itself — stopping at the listing would leave nothing to score.
    """
    return _stop_once(LOAD_TOOL)


def _lookup_task(instructions: str | None, epochs: int) -> Task:
    """Arm A's shared body; `instructions` None is the control condition.

    The two conditions are separate tasks rather than one dataset because
    varying a system message *per sample* needs a custom solver and a grouped
    metric, where two tasks need neither — and Inspect takes several tasks in
    one command, so they still run under the same model at the same moment.
    """
    return Task(
        # One case per request shape, not the full routing matrix — see
        # LOOKUP_CASES. Statistical power comes from epochs, not from adding
        # prompts that ask the same question again.
        dataset=_samples(LOOKUP_CASES),
        solver=[
            *([system_message(instructions)] if instructions else []),
            react(
                tools=[chip_server(READ_ONLY_TOOLS)],
                on_continue=_stop_once_skill_discovered(),
                prompt=_sequential_prompt(),
            ),
        ],
        # Deliberately NOT skill_selected. In this arm that scorer conflates two
        # things — it fails both when the model never looked and when it looked
        # and chose wrong — making it approximately
        # `any_skill_discovered` x skill_match's `skill_selected`. Both factors
        # are measured separately and more cleanly (here and in skill_match
        # respectively), so the product adds no information and cannot be
        # decomposed. `skill_match` also runs 7 cases against this arm's 3, so it is
        # the better place to measure routing regardless.
        scorer=[
            # The headline. `any_skill_discovered` is a binary over "ever
            # looked", so it pins at 1.000 for any model that looks eventually
            # and has no headroom left to show an instructions gap; this
            # separates "looked" from "looked *before* acting". Both are kept:
            # the pair localises the failure, since discovered-but-not-first is a
            # different problem from never-looked.
            skill_discovery_first(),
            any_skill_discovered(),
            # Near-vacuous on this arm — the early stop usually fires before the
            # model writes any prose — but it costs nothing and catches the model
            # reading skill names out of the list_collibra_skills response and
            # then writing as though it were following one it never loaded.
            skill_citation_consistency(),
        ],
        epochs=epochs,
        # Read-only, so it ends on its own; this is a runaway-loop backstop.
        message_limit=30,
    )


@task
def skill_lookup(epochs: int = 5):
    """Arm A — does the model reach for a skill at all, under production
    conditions?

    The model gets chip's real `initialize` instructions, fetched live from the
    binary, which is what an actual MCP client receives. `any_skill_discovered`
    is then the production number: how often the shipped setup gets the model to
    look for a skill unprompted.

    That number alone does not tell you whether the ~2.2k-character
    instructions text earns its place on every request — for that you need
    `skill_lookup_no_instructions` and the gap between the two:

        this high, control low   the instructions carry the behaviour
        both high                tool descriptions already do; the text is
                                 dead weight and could be deleted
        both low                 the text is being ignored; rewrite it

    Run both in one command so they share a model, a moment and an instance —
    otherwise drift between the runs lands in the gap and reads as signal:

        inspect eval tasks/skill_arms.py@skill_lookup \\
                     tasks/skill_arms.py@skill_lookup_no_instructions
    """
    return _lookup_task(_server_instructions(), epochs)


@task
def skill_lookup_no_instructions(epochs: int = 5):
    """Arm A's control — the same thing with no system message at all.

    The model sees only the tool schemas; nothing tells it skills exist. Its
    score is the floor `skill_lookup` is measured against, and the absence
    of a system message *is* the manipulation, so there is deliberately nothing
    here but the react loop.
    """
    return _lookup_task(None, epochs)


@task
def skill_match(epochs: int = 5):
    """Arm B — given that it looks, does it pick the right skill?

    The decision to look is handed to it via SYSTEM_MESSAGE — a local copy of the
    text data_product_create.py uses, deliberately not imported: that file belongs
    to another author, and their edits should not silently change what this arm
    measures. What is left is pure routing against real distractors: asset-create
    and asset-edit both compete for "create"/"add" prompts.

    CASES includes negatives, so this measures precision as well as recall — a
    `description` sharpened until it wins the happy path can start hijacking
    unrelated requests, and that only shows up here.
    """
    return Task(
        dataset=_samples(CASES),
        solver=[
            system_message(SYSTEM_MESSAGE),
            react(
                tools=[chip_server(READ_ONLY_TOOLS)],
                on_continue=_stop_once_skill_chosen(),
                prompt=_sequential_prompt(),
            ),
        ],
        scorer=[
            skill_selected(),
            any_skill_discovered(),
            skill_citation_consistency(),
        ],
        epochs=epochs,
        message_limit=30,
    )


@task
def skill_adherence(epochs: int = 3):
    """Arm C — given the right skill, is it followed correctly?

    Routing is removed from the equation by naming the skill in the prompt, so
    a failure here is the SKILL.md body's fault and nothing else. `skill_selected`
    guards that premise: it should read ~100%, and a dip means the model never
    loaded the skill it was handed, making the rest of the arm meaningless.

    Each rollout seeds its **own** throwaway star schema (community -> domain ->
    schema -> 4 tables -> 18 columns) in a single import request, so:

      * asset names are unique per rollout, which removes create_asset's
        duplicate gating — this arm is safe to run in parallel and with epochs,
        unlike the fixture-based original;
      * seeded_end_state anchors on the seeded table's UUID and walks outward
        instead of matching a Data Product by name, so a leftover product from
        another run is unreachable and cannot be scored as success;
      * teardown is one cascading delete (scripts/teardown_seeded.py), and is
        hygiene rather than a correctness requirement.

    `{source_table}` stays unsubstituted here on purpose — seed_star_schema
    fills it in once the table exists.

    epochs defaults to 3 rather than the cheap arms' 5: this is the expensive
    arm (~30-50 tool round-trips against all 31 tool schemas per rollout, plus
    23 seeded assets to tear down), so 5 would make a bare `inspect eval` a
    costly surprise. 3 is the least that still yields a rate rather than a
    single outcome — and the scorers declare `stderr()`, which is undefined at
    n=1. No `--max-samples 1`: unique per-rollout names are exactly what makes
    concurrent epochs safe here.
    """
    return Task(
        dataset=[
            Sample(
                id="execution",
                input=ADHERENCE_PROMPT,
                target="collibra/data-product-create",
                metadata={"expected_skill": "collibra/data-product-create"},
            )
        ],
        setup=seed_star_schema(),
        solver=[react(tools=[chip_server()], prompt=_sequential_prompt())],
        scorer=[
            seeded_end_state(),
            columns_discovered(),
            write_sequence(),
            skill_selected(),
            # Guards the *order* skill_selected does not: it checks which skill
            # was loaded, not that the load preceded the work. A model that
            # starts searching and reads the named playbook afterwards is not
            # being guided by the SKILL.md body this arm exists to measure.
            skill_discovery_first(),
            skill_citation_consistency(),
        ],
        epochs=epochs,
        message_limit=120,
    )
