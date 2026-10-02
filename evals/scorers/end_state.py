"""End-state scorer: ignores how the agent got there and asks the test
environment whether the expected asset graph now exists.

Checks (each contributes equally to the score):
  product_exists    - a Data Product with the expected name exists
  ports_linked      - it exposes the expected number of Data Product Ports
  tables_grouped    - some port groups every expected table
  contract_governs  - a Data Contract has a relation onto one of the ports
"""

from inspect_ai.scorer import Score, Target, accuracy, scorer, stderr
from inspect_ai.solver import TaskState

from . import collibra


@scorer(metrics=[accuracy(), stderr()])
def data_product_end_state():
    async def score(state: TaskState, target: Target) -> Score:
        expected = state.metadata["expected"]
        checks: dict[str, bool] = {}
        notes: list[str] = []

        async with collibra.client() as c:
            product = await collibra.find_asset_by_name(
                c, expected["data_product_name"]
            )
            checks["product_exists"] = product is not None
            if product is None:
                notes.append(
                    f"no asset named {expected['data_product_name']!r} found"
                )
                return _result(checks, notes)

            # Ports: relations off the product whose counterpart is a Port asset.
            ports: list[dict] = []
            for rel in await collibra.relations(c, source_id=product["id"]):
                tgt = rel.get("target") or {}
                if tgt and await collibra.asset_type_name(c, tgt["id"]) == (
                    "Data Product Port"
                ):
                    ports.append(tgt)
            checks["ports_linked"] = len(ports) >= int(expected.get("ports", 1))
            notes.append(f"ports found: {[p.get('name') for p in ports]}")

            # Grouped tables: at least one port must group every expected table.
            want = set(expected.get("grouped_tables", []))
            grouped_ok = False
            for port in ports:
                got = {
                    (rel.get("target") or {}).get("name", "")
                    for rel in await collibra.relations(c, source_id=port["id"])
                }
                missing = want - got
                if not missing:
                    grouped_ok = True
                    break
                notes.append(f"port {port.get('name')!r} missing tables: {missing}")
            checks["tables_grouped"] = bool(want) and grouped_ok

            # Contract: an incoming relation on some port from a Data Contract.
            contract_ok = False
            for port in ports:
                for rel in await collibra.relations(c, target_id=port["id"]):
                    src = rel.get("source") or {}
                    if src and await collibra.asset_type_name(c, src["id"]) == (
                        "Data Contract"
                    ):
                        contract_ok = True
                        notes.append(
                            f"contract {src.get('name')!r} governs "
                            f"port {port.get('name')!r}"
                        )
            checks["contract_governs"] = contract_ok

        return _result(checks, notes)

    return score


def _result(checks: dict[str, bool], notes: list[str]) -> Score:
    total = 4  # fixed check count so partial graphs score proportionally
    passed = sum(checks.values())
    lines = [f"{'PASS' if ok else 'FAIL'} {name}" for name, ok in checks.items()]
    return Score(
        value=passed / total,
        explanation="\n".join(lines + notes),
        metadata={"checks": checks},
    )
