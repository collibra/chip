"""Happy-path eval for the collibra/data-product-create skill.

Fill in fixtures/data_product_create.yaml (or point EVAL_FIXTURE at another
fixture file), then:

    inspect eval tasks/data_product_create.py --model anthropic/claude-sonnet-5
    inspect view
"""

import os
import sys
from pathlib import Path

import yaml

EVALS_DIR = Path(__file__).resolve().parents[1]
REPO_ROOT = EVALS_DIR.parent
sys.path.insert(0, str(EVALS_DIR))

from inspect_ai import Task, task
from inspect_ai.agent import react
from inspect_ai.dataset import Sample
from inspect_ai.solver import system_message
from inspect_ai.tool import mcp_server_stdio

from scorers.end_state import data_product_end_state
from scorers.trajectory import skill_loaded, write_order

# Real MCP clients receive chip's server-level instructions, which tell the
# model to discover and load skills before multi-step workflows. Inspect does
# not surface those instructions, so we replicate the relevant part here —
# without it we would be measuring Inspect's plumbing, not the skill.
SYSTEM_MESSAGE = """You are connected to a Collibra MCP server. Before composing
any multi-step Collibra workflow, discover the relevant skill guide by calling
list_collibra_skills and load the matching one with load_collibra_skill, then
follow it."""

PROMPT_TEMPLATE = """Create a Collibra Data Product from the table {source_table}.

Include only the dimension tables you identify with high confidence and skip
anything you would normally ask me to confirm. Treat my sign-off on your
proposal as already given — proceed with creation without waiting, and finish
by reporting the UUIDs of everything you created."""


def _load_fixture() -> dict:
    path = Path(
        os.environ.get("EVAL_FIXTURE", EVALS_DIR / "fixtures/data_product_create.yaml")
    )
    fixture = yaml.safe_load(path.read_text())
    if not fixture.get("source_table"):
        raise ValueError(
            f"fixture {path} has no source_table — fill it in before running "
            "(see the comments in the fixture file)"
        )
    expected = fixture.get("expected") or {}
    for key in ("data_product_name", "grouped_tables"):
        if not expected.get(key):
            raise ValueError(f"fixture {path} is missing expected.{key}")
    return fixture


def chip_server():
    args = [
        "--experimental=skills",
        # Working-tree skills replace the embedded catalog on name match,
        # so SKILL.md edits are picked up with no rebuild.
        f"--skills-dir={REPO_ROOT / 'pkg/skills/files'}",
    ]
    # All 28 tool schemas are ~20odel ck input tokens per mall. Real clients
    # see all of them, so unset (the default) is the faithful setup — but on
    # low API rate-limit tiers a request that large can 429 forever. Set
    # EVAL_ENABLED_TOOLS to trim (see README for the skill's minimal set).
    enabled_tools = os.environ.get("EVAL_ENABLED_TOOLS")
    if enabled_tools:
        args.append(f"--enabled-tools={enabled_tools}")
    return mcp_server_stdio(
        name="collibra",
        command=str(REPO_ROOT / ".build/chip"),
        args=args,
        cwd=REPO_ROOT,  # chip resolves ./mcp.yaml from here; env vars still win
    )


@task
def data_product_create_happy_path():
    fixture = _load_fixture()
    return Task(
        dataset=[
            Sample(
                id="happy-path",
                input=PROMPT_TEMPLATE.format(source_table=fixture["source_table"]),
                target=fixture["expected"]["data_product_name"],
                metadata={"expected": fixture["expected"]},
            )
        ],
        solver=[
            system_message(SYSTEM_MESSAGE),
            react(tools=[chip_server()]),
        ],
        scorer=[data_product_end_state(), skill_loaded(), write_order()],
        # The full flow takes ~30-50 tool round-trips; a runaway loop should
        # fail fast instead of burning tokens.
        message_limit=120,
    )
