"""Trajectory scorers: walk the transcript's tool calls and check the skill's
procedural rules, independent of the final environment state.

Tool names are matched by suffix because MCP clients may prefix them with the
server name.
"""

from inspect_ai.scorer import CORRECT, INCORRECT, Score, Target, accuracy, scorer, stderr
from inspect_ai.solver import TaskState


def _tool_calls(state: TaskState) -> list:
    return [
        tc
        for m in state.messages
        if getattr(m, "tool_calls", None)
        for tc in m.tool_calls
    ]


def _is(call, name: str) -> bool:
    return call.function == name or call.function.endswith(f"_{name}") or (
        call.function.endswith(name)
    )


@scorer(metrics=[accuracy(), stderr()])
def skill_loaded():
    """The agent discovered and loaded the data-product-create skill before
    doing the work (cold start — triggering failures show up here, keeping
    them distinguishable from execution failures)."""

    async def score(state: TaskState, target: Target) -> Score:
        calls = _tool_calls(state)
        loaded = [
            i
            for i, c in enumerate(calls)
            if _is(c, "load_collibra_skill")
            and "data-product-create" in str(c.arguments)
        ]
        if not loaded:
            return Score(
                value=INCORRECT,
                explanation="load_collibra_skill(data-product-create) never called",
            )
        first_write = next(
            (i for i, c in enumerate(calls) if _is(c, "create_asset")), None
        )
        before_writes = first_write is None or loaded[0] < first_write
        return Score(
            value=CORRECT if before_writes else INCORRECT,
            explanation=(
                f"skill loaded at call #{loaded[0]}, "
                f"first create_asset at #{first_write}"
            ),
        )

    return score


@scorer(metrics=[accuracy(), stderr()])
def write_order():
    """Hard rule 1: every create_asset precedes every relation-adding edit,
    and the contract push comes after its port exists (last create)."""

    async def score(state: TaskState, target: Target) -> Score:
        calls = _tool_calls(state)
        creates = [i for i, c in enumerate(calls) if _is(c, "create_asset")]
        relation_edits = [
            i
            for i, c in enumerate(calls)
            if _is(c, "edit_asset") and "add_relation" in str(c.arguments)
        ]
        contract_pushes = [
            i for i, c in enumerate(calls) if _is(c, "push_data_contract_manifest")
        ]

        if not creates:
            return Score(value=INCORRECT, explanation="no create_asset calls at all")

        problems = []
        if relation_edits and min(relation_edits) < max(creates):
            problems.append(
                f"add_relation at #{min(relation_edits)} before "
                f"last create_asset at #{max(creates)}"
            )
        if contract_pushes and min(contract_pushes) < max(creates):
            problems.append(
                f"contract pushed at #{min(contract_pushes)} before "
                f"last create_asset at #{max(creates)}"
            )

        return Score(
            value=INCORRECT if problems else CORRECT,
            explanation="; ".join(problems)
            or (
                f"{len(creates)} creates, then {len(relation_edits)} relation "
                f"edits, {len(contract_pushes)} contract push(es)"
            ),
        )

    return score
