# Skill evals

See [../EVALS.md](../EVALS.md) for the why and the overall plan. This directory is the
harness ([Inspect AI](https://inspect.aisi.org.uk)).

## Setup (once)

```bash
# from the repo root
go build -o .build/chip ./cmd/chip

cd evals
python3 -m venv .venv
.venv/bin/pip install -r requirements.txt
```

Credentials — same resolution as chip itself: `COLLIBRA_MCP_API_URL/USR/PWD` env vars,
falling back to the repo-root `mcp.yaml`. If chip works locally, the scorers work too.

## Run the happy path

1. Fill in `fixtures/data_product_create.yaml` (source table + expected outcome).
2. ```bash
   export ANTHROPIC_API_KEY=<key>
   cd evals && source .venv/bin/activate
   inspect eval tasks/data_product_create.py --model anthropic/claude-sonnet-5
   ```
3. Browse the result:
   ```bash
   inspect view
   ```
4. Clean up before the next run (duplicate names break reruns):
   ```bash
   python scripts/cleanup.py          # dry run
   python scripts/cleanup.py --apply
   ```

Useful flags: `--epochs 3` (repeat for stability), `--limit 1`, `--log-dir logs/`.

### Hitting 429 rate limits?

Every model call carries all 28 chip tool schemas (~20k input tokens), which can
exceed a low-tier key's per-minute input-token budget outright — the run then 429s
forever, no matter how long Inspect backs off. Options, in order of preference:

1. Raise the key's tier / check workspace limits in the Anthropic console.
2. Trim chip's tool surface to what the skill needs (~halves the request size;
   slightly less faithful to real clients, which see every tool):
   ```bash
   export EVAL_ENABLED_TOOLS="list_collibra_skills,load_collibra_skill,search_asset_keyword,get_asset_details,get_table_semantics,prepare_create_asset,create_asset,edit_asset,list_data_contract,pull_data_contract_manifest,push_data_contract_manifest"
   ```

## Layout

```
tasks/       Inspect task definitions (one file per skill)
scorers/     end_state.py (REST assertions), trajectory.py (transcript assertions)
fixtures/    per-environment expected values — the only file you edit per env
scripts/     cleanup.py — delete eval-created assets between runs
```
