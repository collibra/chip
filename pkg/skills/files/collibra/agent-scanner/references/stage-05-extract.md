# Stage 05: Extract

Fill in the facts for each agent and each tool, and write them to `extraction.json` in the output folder. Load `references/agent-scan-fields.md` with this stage: it holds the exact field list, allowed labels and an example.

If Stage 02 left a verdict, follow it: `unchanged` skips this stage, `changed` uses the changed-only extraction described in `references/stage-02-known-agent.md`. The rules below apply to everything you read again.

## What you write

One JSON file with three keys: `agents`, `tools`, `warnings`. You do not write hashes, `via`, `parser`, `generatedOn`, `fingerprintVersion` or `source`. The finalize program adds those (H6).

## Rules for every field

- **Quote, don't paraphrase.** System prompt text, tool names, argument names, argument types and default values are copied from the source exactly (H5).
- **Labels are lowercase and use the spellings in the reference** (framework name, model provider, middleware hook, guardrail kind and behavior). The same code scanned twice must give the same labels. If nothing in the reference fits, use `other` for a tool pattern and the nearest known value elsewhere, and add a warning.
- **Only report what you can see.** `crossSessionPersistence` is omitted unless the code shows a checkpointer or storage backend. A model that is chosen from an environment variable is not a model you know: list what you resolved and set `modelsResolved` to false. The exception is a value Stage 04 read from the deployed configuration: if `deployed-config.json` holds that variable, use its value as the model, keep the default in the warning, say in a warning where the value came from ("<NAME> is <value> in the configuration of <deployment> version <V>"), and set `modelsResolved` to true only if nothing else about the models is still open. The same applies to any other value the code takes from an environment variable that a field needs (an endpoint, a tool's target).
- **Four completeness flags per agent**, each set honestly:
  - `true` means the list beside it is complete, so an empty list means "confirmed none".
  - `false` means there may be more than you could list (tools added at runtime, a model picked by config, delegation decided at runtime, middleware you could not trace). An empty list then means "unknown".
  - Setting all four to `true` by default defeats their purpose. Check each one.
- **Redact secrets** (H7). Replace a credential-like value in a prompt or default with `[REDACTED]` and add a warning naming the file and field.

## Per tool: look inside the body

For each tool, read the implementation body, not only the signature and docstring, and list its guardrails in the tool's `guardrails` list.

**A guardrail is a check in the tool's code that compares an input against a rule the code itself defines, and then raises or returns an error, blocks the call, requires approval or sign-in, or rewrites the input to fit the rule.** The rule is a fixed value, a list, a pattern, a format or a policy that sits in the code or its configuration: a constant limit, an allowlist or denylist, a regular expression, an amount threshold, an approval or authentication step, a scope restriction.

- Report: `if len(query) > MAX_QUERY_CHARS: raise ...`; `if amount > LIMIT: raise PermissionError`; `if path not in ALLOWED_DIRS: return error`; `if not confirm: raise ...`.
- Do not report: slicing or truncating to a limit the caller passes in (`text[:limit]`), default values, ordinary type conversion, logging, retries, and branches that choose between normal behaviours.
- Give each one a `confidence` from 0 to 1 for how sure you are it is a deliberate guardrail. If you would give less than 0.5, leave it out and mention it in `warnings` instead.
- Report an empty list when you looked and found none. Do not skip the field.

## Tools and agents must match up

- Every name in an agent's `tools` appears in the top-level `tools`, and the other way round for `usedByAgents`.
- Every name in an agent's `subagents` is an entry in `agents`.
- Names are at least three characters and unique within agents and within tools. If a real name is shorter (`ls`) or two tools share a name, choose a distinct, descriptive name and add a warning that records the original.

## Dynamic tool sources

A tool list resolved at runtime (an MCP server, a toolkit that discovers functions) is one entry with `definitionPattern` `mcp_dynamic` and `origin.dynamic` true, and the agent's `toolsResolved` set to false. Do not invent the tools it might expose.

## Evidence

For every agent and tool, record where you found it in `evidence`: a list of `"path:line"` strings relative to the scan root. It makes review and verification quick, and the finalize step removes it.

## Large repositories

If sub-agents are available and discovery found more than 8 files that construct agents or tools, split the extraction as described in `references/subagents.md`. Otherwise extract yourself.

## Write, verify, summarise

1. Write `extraction.json` to the output folder.
2. Verify it as described in `references/subagents.md`: an independent check by a fresh sub-agent if sub-agents are available, otherwise the single-agent self-review. Fix `extraction.json` accordingly.
3. Give a short status line in plain words: how many agents and tools, which things are unresolved and why, and whether an independent check ran and what it changed.

## Exit criteria

- `extraction.json` written, valid JSON, every agent and tool from Stage 03 covered or explained in `warnings`.
- Each unresolved item has a warning.

End with the short summary and move straight to Stage 06. No customer question is needed unless something is ambiguous.
