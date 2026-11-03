"""Seed a throwaway star schema per rollout, in one API request.

Replaces the fixture's hardcoded source-table UUID. Each rollout gets its own
community → domain → schema → tables → columns, named `TEST_<tag>_<static>`, so:

  * the Data Product name the skill derives from the table is unique, which
    removes `create_asset`'s duplicate gating and lets rollouts run in parallel;
  * the scorer can anchor on a known table UUID and walk the graph outward
    instead of matching a product by name — so a leftover product from another
    run is unreachable and cannot be mistaken for success;
  * teardown is one cascading DELETE of the community.

Everything is created by a single `POST /import/json-job`: the import command
format carries assets, attributes *and* relations inline. Verified live —
1 community, 1 domain, 23 assets, 1 attribute, 22 relations, 0 errors.
"""

import asyncio
import json
from pathlib import Path

import httpx
from inspect_ai.solver import Generate, Solver, TaskState, solver

from scorers import collibra

EVALS_DIR = Path(__file__).resolve().parents[1]
MANIFEST = EVALS_DIR / "logs" / "seed-manifest.jsonl"

# System UUIDs — out-of-the-box types, stable across instances. Probed on a live
# instance rather than assumed; see docs/skill_adherence.md.
DOMAIN_TYPE_PHYSICAL = "Physical Data Dictionary"
# Data Product / Data Product Port / Data Contract are only allowed in a domain
# of this type, so without one the skill cannot reach Phase 7 at all: every
# prepare_create_asset returns "domain is required … Filtered to Data Product
# Catalog domains" with an empty option list. Seeding one per rollout is what
# makes this arm independent of how the instance happens to be set up.
DOMAIN_TYPE_DP_CATALOG = "Data Product Catalog"
REL_SCHEMA_CONTAINS_TABLE = "00000000-0000-0000-0000-000000007043"
REL_COLUMN_PART_OF_TABLE = "00000000-0000-0000-0000-000000007042"
ATTR_DESCRIPTION = "00000000-0000-0000-0000-000000003114"

FACT_TABLE = "ORDERS"
FACT_DESCRIPTION = "Fact table of individual sales order lines."

# Dimension names and the shared key columns are the whole point: Phase 3 of the
# skill picks related tables by plain-language reasoning over *names*, not by
# foreign keys or lineage. ORDERS carrying CUSTOMER_ID/PRODUCT_ID/STORE_ID is
# what gives that reasoning something to latch onto.
TABLE_COLUMNS = {
    FACT_TABLE: [
        "ORDER_ID",
        "CUSTOMER_ID",
        "PRODUCT_ID",
        "STORE_ID",
        "ORDER_TOTAL",
        "ORDER_DATE",
    ],
    "CUSTOMER": ["CUSTOMER_ID", "CUSTOMER_NAME", "EMAIL", "CITY"],
    "PRODUCT": ["PRODUCT_ID", "PRODUCT_NAME", "CATEGORY", "UNIT_PRICE"],
    "STORE": ["STORE_ID", "STORE_NAME", "REGION", "COUNTRY"],
}


def names(tag: str) -> dict:
    """Every name this seed will create, derived from one dynamic tag."""
    return {
        "community": f"TEST_{tag}_COMMUNITY",
        "domain": f"TEST_{tag}_SALES_DOMAIN",
        # Where the agent writes. Per-rollout and inside the same community, so
        # concurrent rollouts cannot write into each other's catalog and the
        # community cascade still removes everything in one request.
        "dp_domain": f"TEST_{tag}_DP_CATALOG",
        "schema": f"TEST_{tag}_SALES",
        "fact_table": f"TEST_{tag}_{FACT_TABLE}",
        "dimension_tables": [
            f"TEST_{tag}_{t}" for t in TABLE_COLUMNS if t != FACT_TABLE
        ],
    }


def build_commands(tag: str) -> list[dict]:
    """The import document: community, domain, and every asset with its relations.

    Relations are declared on the *source* asset via a `<relationTypeId>:TARGET`
    key listing its targets. That format is undocumented in the OpenAPI schema —
    it was confirmed against a live instance (the import summary reported
    `RELATION added=22`).
    """
    n = names(tag)
    domain_ref = {"name": n["domain"], "community": {"name": n["community"]}}

    def ident(name: str) -> dict:
        return {"name": name, "domain": domain_ref}

    def asset(name: str, type_name: str, relations=None, description=None) -> dict:
        cmd = {
            "resourceType": "Asset",
            "identifier": ident(name),
            "name": name,
            "type": {"name": type_name},
        }
        if relations:
            cmd["relations"] = relations
        if description:
            cmd["attributes"] = {ATTR_DESCRIPTION: [{"value": description}]}
        return cmd

    table_names = [f"TEST_{tag}_{t}" for t in TABLE_COLUMNS]
    commands = [
        {"resourceType": "Community", "identifier": {"name": n["community"]}},
        {
            "resourceType": "Domain",
            "identifier": domain_ref,
            "type": {"name": DOMAIN_TYPE_PHYSICAL},
        },
        # Empty on purpose: the agent fills it in Phase 7. Its only job is to
        # exist, so prepare_create_asset has a legal home to offer for the
        # Data Product, its Port and the Data Contract.
        {
            "resourceType": "Domain",
            "identifier": {
                "name": n["dp_domain"],
                "community": {"name": n["community"]},
            },
            "type": {"name": DOMAIN_TYPE_DP_CATALOG},
        },
        # The schema owns the tables: "Schema contains Table", schema as source.
        asset(
            n["schema"],
            "Schema",
            relations={
                f"{REL_SCHEMA_CONTAINS_TABLE}:TARGET": [ident(t) for t in table_names]
            },
        ),
    ]

    for table, columns in TABLE_COLUMNS.items():
        table_name = f"TEST_{tag}_{table}"
        commands.append(
            asset(
                table_name,
                "Table",
                description=FACT_DESCRIPTION if table == FACT_TABLE else None,
            )
        )
        for column in columns:
            # Note the direction: the *column* is the source of
            # "Column is part of Table". There is no Table->Column relation type
            # at all, so columns necessarily arrive as incoming relations on the
            # table. (The skill's Phase 1 says "outgoing Column relations", which
            # is why this eval exists — see docs/skill_adherence.md.)
            commands.append(
                asset(
                    f"{table_name}_{column}",
                    "Column",
                    relations={
                        f"{REL_COLUMN_PART_OF_TABLE}:TARGET": [ident(table_name)]
                    },
                )
            )

    return commands


async def _request(client: httpx.AsyncClient, method: str, url: str, **kwargs):
    """One request, retrying transient gateway failures.

    The dev instance returned `503 no healthy upstream` for a few seconds
    mid-session. Losing a rollout — and its seeded assets — to a blip that clears
    in 5s is not worth it.
    """
    last = None
    for attempt in range(4):
        response = await client.request(method, url, **kwargs)
        if response.status_code < 500:
            return response
        last = response
        await asyncio.sleep(2 * (attempt + 1))
    return last


async def _await_job(client: httpx.AsyncClient, job_id: str, timeout_s: int = 180):
    """Poll an import job to a terminal state."""
    deadline = timeout_s / 1.5
    for _ in range(int(deadline)):
        job = (await _request(client, "GET", f"/jobs/{job_id}")).json()
        if job.get("state") in ("COMPLETED", "ERROR", "CANCELED"):
            return job
        await asyncio.sleep(1.5)
    raise RuntimeError(f"import job {job_id} did not finish within {timeout_s}s")


async def seed(tag: str) -> dict:
    """Create the star schema for `tag` and return its identifiers.

    Fails loudly: a partially-seeded graph would be scored as a skill failure,
    which is a far more expensive kind of wrong than a crashed setup.
    """
    n = names(tag)
    commands = build_commands(tag)
    expected_assets = sum(1 for c in commands if c["resourceType"] == "Asset")

    async with collibra.client() as client:
        response = await _request(
            client,
            "POST",
            "/import/json-job",
            files={"file": ("seed.json", json.dumps(commands), "application/json")},
            data={"fileName": "seed.json", "continueOnError": "false"},
        )
        if response.status_code != 200:
            raise RuntimeError(f"import job rejected: {response.status_code} {response.text}")

        job_id = response.json()["id"]
        job = await _await_job(client, job_id)
        errors = (
            await _request(client, "GET", f"/import/results/{job_id}/errors")
        ).json()
        if job.get("result") != "SUCCESS" or errors.get("total"):
            raise RuntimeError(
                f"seed import failed (state={job.get('state')} "
                f"result={job.get('result')} errors={errors.get('total')}): "
                f"{json.dumps(errors.get('results', [])[:3])}"
            )

        # One call resolves every seeded UUID. This is a name lookup, but scoped
        # to a domain we just created, so it cannot collide with another run.
        domains = (
            await _request(client, "GET", "/domains", params={"name": n["domain"]})
        ).json()["results"]
        if not domains:
            raise RuntimeError(f"seeded domain {n['domain']!r} not found after import")
        domain_id = domains[0]["id"]

        # Checked separately rather than assumed: if the instance's Data Product
        # Catalog type is missing or renamed, the import still succeeds and the
        # failure would only surface as the agent finding nowhere to write —
        # scored as a skill failure, which is the wrong diagnosis.
        dp_domains = (
            await _request(client, "GET", "/domains", params={"name": n["dp_domain"]})
        ).json()["results"]
        if not dp_domains:
            raise RuntimeError(
                f"seeded {DOMAIN_TYPE_DP_CATALOG} domain {n['dp_domain']!r} not found "
                "after import — the agent would have nowhere to create the Data "
                "Product, and the arm would score that as a SKILL.md failure"
            )
        dp_domain_id = dp_domains[0]["id"]

        assets = (
            await _request(
                client,
                "GET",
                "/assets",
                params={"domainId": domain_id, "limit": 500},
            )
        ).json()["results"]
        if len(assets) != expected_assets:
            raise RuntimeError(
                f"expected {expected_assets} seeded assets, found {len(assets)}"
            )
        by_name = {a["name"]: a["id"] for a in assets}

        communities = (
            await _request(
                client, "GET", "/communities", params={"name": n["community"]}
            )
        ).json()["results"]

        result = {
            "tag": tag,
            "community_id": communities[0]["id"] if communities else None,
            "community_name": n["community"],
            "domain_id": domain_id,
            "dp_domain_id": dp_domain_id,
            "dp_domain_name": n["dp_domain"],
            "schema_id": by_name[n["schema"]],
            "schema_name": n["schema"],
            "fact_table_id": by_name[n["fact_table"]],
            "fact_table_name": n["fact_table"],
            "dimension_table_ids": [by_name[t] for t in n["dimension_tables"]],
            "dimension_table_names": n["dimension_tables"],
            "column_names": [
                f"TEST_{tag}_{t}_{c}" for t, cols in TABLE_COLUMNS.items() for c in cols
            ],
        }

    _record(result)
    return result


def _record(result: dict) -> None:
    """Append to the teardown manifest.

    A file rather than a post-run hook: a hook does not run when a rollout
    crashes, which is exactly the case that leaves assets behind.
    """
    MANIFEST.parent.mkdir(parents=True, exist_ok=True)
    with MANIFEST.open("a") as handle:
        handle.write(json.dumps(result) + "\n")


def _tag(state: TaskState) -> str:
    """A short, filesystem- and Collibra-safe unique tag for this rollout.

    Uses the per-rollout uuid so parallel epochs cannot collide. Not
    random/time-based, which would make a resumed run seed a different graph than
    the one its cached results describe.
    """
    raw = getattr(state, "uuid", None) or f"{state.sample_id}{state.epoch}"
    return str(raw).replace("-", "")[:10].lower()


@solver
def seed_star_schema(
    placeholder: str = "{source_table}",
    domain_placeholder: str = "{target_domain}",
) -> Solver:
    """Setup solver: seed a fresh star schema, then point the prompt at it.

    The prompt has to be filled in here rather than at dataset construction:
    the seeded assets do not exist until this runs, and each epoch of the same
    sample needs its own. So the sample's `input` carries both placeholders
    verbatim and we substitute the real names once they are known.

    Naming the target domain is what makes concurrent rollouts safe. Phase 4 of
    the skill has the *user* pick a domain from every one that accepts a Data
    Product, so with several rollouts in flight the agent would be offered every
    other rollout's catalog too and could write into one of them. Supplying it is
    the same substitution this arm already makes for the user's sign-off, and it
    keeps every asset inside the community the cascade deletes.
    """

    async def solve(state: TaskState, generate: Generate) -> TaskState:
        result = await seed(_tag(state))
        state.metadata["seed"] = result

        prompt = state.user_prompt
        if prompt is not None:
            prompt.text = prompt.text.replace(
                placeholder, result["fact_table_name"]
            ).replace(domain_placeholder, result["dp_domain_name"])
        return state

    return solve
