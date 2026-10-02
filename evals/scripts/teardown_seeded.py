"""Delete everything a seeded execution run created.

Two stages, and the order matters:

  1. **Sweep** the assets the *model* created — Data Product, Port(s), Data
     Contract. Phase 4 of the skill has the model choose the product's target
     domain, so these normally land OUTSIDE the seeded community and the cascade
     in stage 2 will not touch them. They are only findable by navigating from
     the seeded fact table, so this must run FIRST — once the community is gone,
     the anchor is gone with it.
  2. **Cascade** — one `DELETE /communities/{id}` removes the domain and every
     seeded asset. Verified: 23/23 assets returned 404 afterwards.

Teardown is hygiene, not correctness. Because every run's names are unique, a
skipped teardown can no longer cause a false pass — only clutter. That is why
this is a script driven by the seed manifest rather than a post-run hook: a hook
does not run when a rollout crashes, which is exactly when mess is left behind.

    python scripts/teardown_seeded.py              # dry run, all runs
    python scripts/teardown_seeded.py --apply
    python scripts/teardown_seeded.py --tag probe1 --apply
"""

import argparse
import json
import sys
from pathlib import Path

EVALS_DIR = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(EVALS_DIR))

from scorers.collibra import sync_client
from scorers.seeded import (
    REL_CONTRACT_GOVERNS_PORT,
    REL_PORT_IMPLEMENTED_AS_TABLE,
    REL_PRODUCT_EXPOSES_PORT,
)
from solvers.seed import MANIFEST


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--apply", action="store_true", help="actually delete")
    parser.add_argument("--tag", help="only tear down this seed tag")
    parser.add_argument("--manifest", default=str(MANIFEST))
    args = parser.parse_args()

    manifest = Path(args.manifest)
    if not manifest.exists():
        print(f"nothing to do: no manifest at {manifest}")
        return

    seeds = [json.loads(line) for line in manifest.read_text().splitlines() if line.strip()]
    if args.tag:
        seeds = [s for s in seeds if s.get("tag") == args.tag]
    if not seeds:
        print("nothing to do: no matching seed records")
        return

    with sync_client() as client:

        def relations(**params) -> list[dict]:
            response = client.get("/relations", params={**params, "limit": 500})
            response.raise_for_status()
            return response.json().get("results", [])

        def sources(rels, type_id):
            return [
                r["source"]
                for r in rels
                if (r.get("type") or {}).get("id") == type_id and r.get("source")
            ]

        for seed in seeds:
            tag = seed.get("tag", "?")
            print(f"\n=== seed {tag} ({seed.get('community_name')})")

            # Stage 1 — model-created assets, reachable only via the fact table.
            swept: dict[str, str] = {}
            fact_id = seed.get("fact_table_id")
            if fact_id and client.get(f"/assets/{fact_id}").status_code == 200:
                ports = sources(
                    relations(targetId=fact_id), REL_PORT_IMPLEMENTED_AS_TABLE
                )
                for port in ports:
                    onto_port = relations(targetId=port["id"])
                    # Contracts first, then the product, then the port itself.
                    for contract in sources(onto_port, REL_CONTRACT_GOVERNS_PORT):
                        swept[contract["id"]] = f"Data Contract {contract.get('name')!r}"
                    for product in sources(onto_port, REL_PRODUCT_EXPOSES_PORT):
                        swept[product["id"]] = f"Data Product {product.get('name')!r}"
                    swept[port["id"]] = f"Port {port.get('name')!r}"
            elif fact_id:
                print("  (seeded fact table already gone — cannot sweep model assets)")

            for asset_id, label in swept.items():
                if args.apply:
                    response = client.delete(f"/assets/{asset_id}")
                    if response.status_code not in (200, 204, 404):
                        response.raise_for_status()
                    print(f"  deleted      {label}")
                else:
                    print(f"  would delete {label}")
            if not swept:
                print("  no model-created assets found")

            # Stage 2 — one cascading delete for the seeded subtree.
            community_id = seed.get("community_id")
            if not community_id:
                print("  no community_id recorded; skipping cascade")
                continue
            if client.get(f"/communities/{community_id}").status_code == 404:
                print(f"  community {seed.get('community_name')!r} already gone")
                continue
            if args.apply:
                response = client.delete(f"/communities/{community_id}")
                if response.status_code not in (200, 204, 404):
                    response.raise_for_status()
                print(f"  deleted      community {seed.get('community_name')!r} (cascades)")
            else:
                print(
                    f"  would delete community {seed.get('community_name')!r} "
                    "(cascades to domain + all seeded assets)"
                )

    if args.apply:
        # Only clear records we actually processed, so a --tag run does not drop
        # the rest of the manifest.
        done = {s.get("tag") for s in seeds}
        remaining = [
            line
            for line in manifest.read_text().splitlines()
            if line.strip() and json.loads(line).get("tag") not in done
        ]
        manifest.write_text("\n".join(remaining) + ("\n" if remaining else ""))
        print(f"\nmanifest: {len(remaining)} record(s) left")
    else:
        print("\ndry run — rerun with --apply to delete")


if __name__ == "__main__":
    main()
