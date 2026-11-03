"""Delete the assets a data-product-create eval run created, so back-to-back
runs stay independent (create_asset gates on duplicate names, and the skill
itself stops when it finds an existing product over the same tables).

Walks outward from the fixture's expected data product name: product -> ports
-> contracts. Tables are never touched.

Dry-run by default:

    python scripts/cleanup.py            # list what would be deleted
    python scripts/cleanup.py --apply    # actually delete
"""

import argparse
import os
import sys
from pathlib import Path

import yaml

EVALS_DIR = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(EVALS_DIR))

from scorers.collibra import sync_client


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--apply", action="store_true", help="actually delete")
    parser.add_argument(
        "--fixture",
        default=os.environ.get(
            "EVAL_FIXTURE", EVALS_DIR / "fixtures/data_product_create.yaml"
        ),
    )
    args = parser.parse_args()

    expected = (yaml.safe_load(Path(args.fixture).read_text()) or {}).get(
        "expected", {}
    )
    product_name = expected.get("data_product_name")
    if not product_name:
        sys.exit(f"fixture {args.fixture} has no expected.data_product_name")

    with sync_client() as c:

        def type_name(asset_id: str) -> str:
            r = c.get(f"/assets/{asset_id}")
            r.raise_for_status()
            return (r.json().get("type") or {}).get("name", "")

        def related(**params: str) -> list[dict]:
            r = c.get("/relations", params=params)
            r.raise_for_status()
            return r.json().get("results", [])

        r = c.get("/assets", params={"name": product_name, "nameMatchMode": "EXACT"})
        r.raise_for_status()
        products = r.json().get("results", [])
        if not products:
            print(f"nothing to do: no asset named {product_name!r}")
            return

        # id -> name, in deletion order: contracts, then ports, then product
        to_delete: dict[str, str] = {}
        for product in products:
            ports = [
                rel["target"]
                for rel in related(sourceId=product["id"])
                if rel.get("target")
                and type_name(rel["target"]["id"]) == "Data Product Port"
            ]
            for port in ports:
                for rel in related(targetId=port["id"]):
                    src = rel.get("source")
                    if src and type_name(src["id"]) == "Data Contract":
                        to_delete[src["id"]] = src["name"]
            for port in ports:
                to_delete[port["id"]] = port["name"]
            to_delete[product["id"]] = product["name"]

        for asset_id, name in to_delete.items():
            if args.apply:
                c.delete(f"/assets/{asset_id}").raise_for_status()
                print(f"deleted  {name!r}  ({asset_id})")
            else:
                print(f"would delete  {name!r}  ({asset_id})")

        if not args.apply:
            print("\ndry run — rerun with --apply to delete")


if __name__ == "__main__":
    main()
