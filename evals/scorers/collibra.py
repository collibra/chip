"""Thin Collibra DGC REST 2.0 client for eval scorers and cleanup.

Credentials resolve in chip's own precedence order, so a working local chip
setup needs no extra configuration:

    1. COLLIBRA_MCP_API_URL / _USR / _PWD environment variables
    2. ./mcp.yaml            (repo root)
    3. ~/.config/collibra/mcp.yaml
    4. /etc/collibra/mcp.yaml

Steps 2-4 mirror the viper search path in cmd/chip/config.go. Earlier
versions stopped after the repo root, which meant a machine configured the
normal way — credentials in ~/.config/collibra — could not run any eval at
all, and the failure looked like missing credentials rather than a client
that never looked for them.

One deliberate divergence: viper stops at the first config file it finds,
while this fills each field from the first source that supplies it, so a
partial env var set or a partial mcp.yaml is topped up rather than discarded.
That is more forgiving than chip, which means these scorers can authenticate
in a split-config setup where chip itself would not — worth knowing if an eval
passes and the server does not.
"""

import os
from pathlib import Path

import httpx
import yaml

REPO_ROOT = Path(__file__).resolve().parents[2]

# Same order chip's viper config uses (cmd/chip/config.go).
CONFIG_PATHS = (
    REPO_ROOT / "mcp.yaml",
    Path.home() / ".config" / "collibra" / "mcp.yaml",
    Path("/etc/collibra/mcp.yaml"),
)


def collibra_config() -> tuple[str, str, str]:
    url = os.environ.get("COLLIBRA_MCP_API_URL", "")
    usr = os.environ.get("COLLIBRA_MCP_API_USR", "")
    pwd = os.environ.get("COLLIBRA_MCP_API_PWD", "")

    for config_path in CONFIG_PATHS:
        if url and usr and pwd:
            break
        if not config_path.exists():
            continue
        api = (yaml.safe_load(config_path.read_text()) or {}).get("api", {}) or {}
        # Per-field fallback, so a partial env var set is topped up rather
        # than discarded.
        url = url or api.get("url", "")
        usr = usr or api.get("username", "")
        pwd = pwd or api.get("password", "")

    if not (url and usr and pwd):
        searched = "\n  ".join(str(p) for p in CONFIG_PATHS)
        raise RuntimeError(
            "Collibra credentials not found: set COLLIBRA_MCP_API_URL/USR/PWD, "
            f"or provide an mcp.yaml at one of:\n  {searched}"
        )
    return url.rstrip("/"), usr, pwd


def client() -> httpx.AsyncClient:
    url, usr, pwd = collibra_config()
    return httpx.AsyncClient(
        base_url=f"{url}/rest/2.0", auth=(usr, pwd), timeout=30.0
    )


def sync_client() -> httpx.Client:
    url, usr, pwd = collibra_config()
    return httpx.Client(base_url=f"{url}/rest/2.0", auth=(usr, pwd), timeout=30.0)


async def find_asset_by_name(c: httpx.AsyncClient, name: str) -> dict | None:
    """Exact-name asset lookup; returns the first match or None."""
    r = await c.get("/assets", params={"name": name, "nameMatchMode": "EXACT"})
    r.raise_for_status()
    results = r.json().get("results", [])
    return results[0] if results else None


async def asset_type_name(c: httpx.AsyncClient, asset_id: str) -> str:
    r = await c.get(f"/assets/{asset_id}")
    r.raise_for_status()
    return (r.json().get("type") or {}).get("name", "")


async def relations(
    c: httpx.AsyncClient, *, source_id: str | None = None, target_id: str | None = None
) -> list[dict]:
    params: dict[str, str] = {}
    if source_id:
        params["sourceId"] = source_id
    if target_id:
        params["targetId"] = target_id
    r = await c.get("/relations", params=params)
    r.raise_for_status()
    return r.json().get("results", [])
