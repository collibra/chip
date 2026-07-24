"""Thin Collibra DGC REST 2.0 client for eval scorers and cleanup.

Credentials resolve the same way chip's own config does, so a working local
chip setup needs no extra configuration: COLLIBRA_MCP_API_* env vars first,
then the repo-root mcp.yaml.
"""

import os
from pathlib import Path

import httpx
import yaml

REPO_ROOT = Path(__file__).resolve().parents[2]


def collibra_config() -> tuple[str, str, str]:
    url = os.environ.get("COLLIBRA_MCP_API_URL", "")
    usr = os.environ.get("COLLIBRA_MCP_API_USR", "")
    pwd = os.environ.get("COLLIBRA_MCP_API_PWD", "")
    if not (url and usr and pwd):
        mcp_yaml = REPO_ROOT / "mcp.yaml"
        if mcp_yaml.exists():
            api = (yaml.safe_load(mcp_yaml.read_text()) or {}).get("api", {})
            url = url or api.get("url", "")
            usr = usr or api.get("username", "")
            pwd = pwd or api.get("password", "")
    if not (url and usr and pwd):
        raise RuntimeError(
            "Collibra credentials not found: set COLLIBRA_MCP_API_URL/USR/PWD "
            "or provide an mcp.yaml at the repo root"
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
