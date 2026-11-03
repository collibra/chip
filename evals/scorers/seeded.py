"""Scorers for the seeded adherence arm.

Both key off `state.metadata["seed"]`, which `solvers/seed.py` populates.

`seeded_end_state` is the graph-anchored replacement for `end_state`: it starts
from the seeded fact table's UUID and walks outward, instead of looking a Data
Product up by name. That difference is the point — a leftover product from
another run is not reachable from *this* run's table, so it cannot be mistaken
for success. Name-based lookup could be satisfied by a previous run's assets even
when the current run wrote nothing at all.

`columns_discovered` exists because of a bug in the skill. Phase 1 says to read
the column list "from the outgoing `Column` relations", but there is no
Table->Column relation type in Collibra's model at all — the only one is
`Column is part of Table`, with the *column* as source. So columns always arrive
as **incoming** relations, and `get_asset_details` returns them under
`incomingRelations`. A model following Phase 1 literally finds nothing there and
concludes the table has no columns. None of the four end-state checks would
notice: ports, product, contract and grouped tables do not depend on columns. So
the bug degrades quality invisibly, and needs its own check to surface.

Relation type ids below are system UUIDs, probed live. Traversing by type id
rather than fetching each counterpart's asset type also avoids one GET per
relation.
"""

from inspect_ai.scorer import CORRECT, INCORRECT, Score, Target, accuracy, scorer, stderr
from inspect_ai.solver import TaskState

from . import collibra

REL_PORT_IMPLEMENTED_AS_TABLE = "00000000-0000-0000-0000-000000050042"
REL_PRODUCT_EXPOSES_PORT = "00000000-0000-0000-0000-000000050040"
REL_CONTRACT_GOVERNS_PORT = "00000000-0000-0000-0000-000000050044"

CHECK_COUNT = 4  # fixed, so a partial graph scores proportionally


def _sources(relations: list[dict], type_id: str) -> list[dict]:
    return [
        r["source"]
        for r in relations
        if (r.get("type") or {}).get("id") == type_id and r.get("source")
    ]


def _targets(relations: list[dict], type_id: str) -> list[dict]:
    return [
        r["target"]
        for r in relations
        if (r.get("type") or {}).get("id") == type_id and r.get("target")
    ]


@scorer(metrics=[accuracy(), stderr()])
def seeded_end_state():
    """Did the agent build the expected graph around *our* seeded table?

    product_exposes_port  - a Data Product exposes that Port as an output port
    port_exposes_table    - a Data Product Port implements the seeded fact table
    tables_grouped        - the Port also implements every seeded dimension table
    contract_governs      - a Data Contract governs the Port
    """

    async def score(state: TaskState, target: Target) -> Score:
        seed = state.metadata.get("seed")
        if not seed:
            return Score(
                value=0.0,
                explanation="no seed metadata — did seed_star_schema() run as setup?",
            )

        fact_id = seed["fact_table_id"]
        want_tables = set(seed["dimension_table_ids"])
        checks: dict[str, bool] = {}
        notes: list[str] = []

        async with collibra.client() as client:
            onto_fact = await collibra.relations(client, target_id=fact_id)
            ports = _sources(onto_fact, REL_PORT_IMPLEMENTED_AS_TABLE)
            checks["port_exposes_table"] = bool(ports)
            if not ports:
                notes.append(
                    f"no Data Product Port implements {seed['fact_table_name']!r} "
                    f"({len(onto_fact)} relation(s) onto it)"
                )
                return _result(checks, notes, {})
            notes.append(f"ports: {[p.get('name') for p in ports]}")

            product_ok = False
            contract_ok = False
            grouped_best: set[str] = set()
            for port in ports:
                onto_port = await collibra.relations(client, target_id=port["id"])
                if _sources(onto_port, REL_PRODUCT_EXPOSES_PORT):
                    product_ok = True
                    names = [
                        p.get("name")
                        for p in _sources(onto_port, REL_PRODUCT_EXPOSES_PORT)
                    ]
                    notes.append(f"product(s) exposing {port.get('name')!r}: {names}")
                if _sources(onto_port, REL_CONTRACT_GOVERNS_PORT):
                    contract_ok = True
                    names = [
                        c.get("name")
                        for c in _sources(onto_port, REL_CONTRACT_GOVERNS_PORT)
                    ]
                    notes.append(f"contract(s) governing {port.get('name')!r}: {names}")

                from_port = await collibra.relations(client, source_id=port["id"])
                implemented = {
                    t["id"] for t in _targets(from_port, REL_PORT_IMPLEMENTED_AS_TABLE)
                }
                grouped_best |= implemented & want_tables

            checks["product_exposes_port"] = product_ok
            checks["contract_governs"] = contract_ok
            checks["tables_grouped"] = bool(want_tables) and grouped_best == want_tables

        missing = want_tables - grouped_best
        if missing:
            # strict: ids and names come from the same seed manifest and must be
            # parallel. If they ever aren't, `by_id[i]` below would raise an
            # opaque KeyError instead of naming the real problem.
            by_id = dict(
                zip(
                    seed["dimension_table_ids"],
                    seed["dimension_table_names"],
                    strict=True,
                )
            )
            notes.append(f"dimension tables not grouped: {[by_id[i] for i in missing]}")
        extra = {
            "grouped_fraction": (
                len(grouped_best) / len(want_tables) if want_tables else 0.0
            )
        }
        return _result(checks, notes, extra)

    return score


def _result(checks: dict, notes: list[str], extra: dict) -> Score:
    passed = sum(checks.values())
    lines = [f"{'PASS' if ok else 'FAIL'} {name}" for name, ok in checks.items()]
    return Score(
        value=passed / CHECK_COUNT,
        explanation="\n".join(lines + notes),
        metadata={"checks": checks, **extra},
    )


@scorer(metrics=[accuracy(), stderr()])
def columns_discovered():
    """Did the agent actually see the seeded table's columns?

    Surfaces the Phase 1 wording bug described in this module's docstring: a
    model told to read "outgoing Column relations" finds none, because columns
    are incoming. Scored from the agent's own prose — if it discovered the
    columns it names at least a couple of them while reasoning about the table.

    Deliberately lenient (>= 2 distinct columns) rather than requiring all of
    them: the skill only needs "a few" columns for the Data Set check, so
    demanding the full list would fail runs that behaved correctly.
    """

    async def score(state: TaskState, target: Target) -> Score:
        seed = state.metadata.get("seed")
        if not seed:
            return Score(value=INCORRECT, explanation="no seed metadata")

        prose_parts = []
        for message in state.messages:
            if getattr(message, "role", None) != "assistant":
                continue
            content = getattr(message, "content", None)
            if isinstance(content, str):
                prose_parts.append(content)
            elif isinstance(content, list):
                for block in content:
                    text = getattr(block, "text", None)
                    if isinstance(text, str):
                        prose_parts.append(text)
        prose = "\n".join(prose_parts)

        fact_prefix = seed["fact_table_name"] + "_"
        fact_columns = [c for c in seed["column_names"] if c.startswith(fact_prefix)]
        # Match the bare column name too: the model usually writes ORDER_TOTAL
        # rather than the fully-qualified TEST_<tag>_ORDERS_ORDER_TOTAL.
        found = [
            c
            for c in fact_columns
            if c in prose or c[len(fact_prefix) :] in prose
        ]

        ok = len(found) >= 2
        return Score(
            value=CORRECT if ok else INCORRECT,
            explanation=(
                f"{len(found)}/{len(fact_columns)} seeded fact-table columns named "
                f"in the agent's own prose: {[c[len(fact_prefix):] for c in found]}"
                + (
                    ""
                    if ok
                    else " — consistent with Phase 1's 'outgoing Column relations',"
                    " which finds nothing because columns are incoming"
                )
            ),
            metadata={
                "found": found,
                "expected_any_of": fact_columns,
            },
        )

    return score
