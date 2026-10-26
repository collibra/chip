# Agent scan fields

The exact shape of `extraction.json` and the labels to use. Anything not listed here is not written by you (the finalize program adds it).

## Top level

```json
{ "agents": [ ... ], "tools": [ ... ], "warnings": [ "..." ] }
```

All three are JSON arrays, never a JSON string that contains an array.

## Document-level optional fields (only when a platform file or the customer provides them)

| Field | Meaning |
|---|---|
| `platform` | `{ "name": "<UPPER_SNAKE_CASE>", "metadata": { ... } }`. `name` is the host from the platform check in upper case: `DATABRICKS` (metadata keys `ucModelName`, `workspaceId`, `mlflowRunId`), `AWS`, `AZURE`, `GCP`, `SNOWFLAKE`; leave the block out when there is no host. It is where the agent runs; the id's prefix may name a different route (an agent on AWS reporting to MLflow has `platform.name` `AWS` and an id starting `mlflow:`) |
| `source` | `{ "repoUrl": "...", "commitSha": "..." }`. Set it only when the scanned code is not the working copy (for example an extracted deployed commit); otherwise the finalize step finds it with git |

## Agent

| Field | Required | Meaning |
|---|---|---|
| `name` | yes | Identifier from the source, for example the variable or config name. 3 or more characters, unique. **Exception, the ROOT of an agent that runs on a platform:** the platform's own name for the agent (for Databricks, the last part of the model name), not a variable or class name; see the platform file. Put the code's name in a warning. |
| `role` | yes | `ROOT`, `SUBAGENT` or `ORPHAN` (Stage 03). Exactly one ROOT. |
| `pattern` | yes | How **this** agent is built (table below). Not the framework's style copied onto every agent. |
| `framework` | yes | `{ "name": "<lowercase>", "version": "<as found>" }`. Name matches the package or import name, for example `langgraph`, `langchain`, `deepagents`; `unknown` if you cannot tell. Version is exact if in a manifest, approximate (`0.2.x`) if inferred from imports; omit if unknown. |
| `crossSessionPersistence` | no | `true` if a checkpointer or backend keeps state across sessions (for example Postgres, SQLite, filesystem). `false` if only in-process (for example in-memory saver). **Omit** if you cannot tell. Never guess `false`. |
| `models` | yes | List of `{ "provider": "<lowercase>", "modelId": "<as in code>", "parameters": {…}, "isDefault": true/false }`. Exactly one `isDefault: true` if the list is not empty. `parameters` only keeps `temperature`, `top_p`, `max_tokens` that you actually found. |
| `systemPrompt` | no | `{ "text": "<verbatim>", "templatingVars": [..], "dynamic": true/false }`. `dynamic` is true if built by string formatting or branching at runtime. Omit the whole field if there is no system prompt. |
| `middleware` | yes | List of `{ "class": "<name>", "hook": "<lowercase snake_case>" }`. Known hooks: `wrap_tool_call`, `wrap_model_call`. |
| `tools` | yes | Names of tools this agent calls. Each must be an entry in top-level `tools`. |
| `subagents` | yes | Names of agents this agent delegates to. Each must be an entry in top-level `agents`. |
| `deployment` | no | **Root agent only.** Where and as what the root agent is deployed: `{ "version": "42", "endpoint": { "url": "https://...", "name": "..." } }`. `version` is the platform's own version string (for example a Unity Catalog model version), set only when the scanned code is the code of that version. `endpoint.url` is the address the root agent is served at, and an `endpoint` needs its `url`; it must not carry credentials. `endpoint.name` is the serving endpoint's own name where the platform has one (for Databricks the serving endpoint name). Only what was found, never guessed. The old flat fields `deploymentVersion` and `endpointUrl` are refused by the service. The `version` and the `url` are part of the document hash, so a redeploy or a new address is a new version in Collibra; the name is not
| `evidence` | no | Where you found this agent: a list of `"path:line"` strings, relative to the scan root. For review only; the finalize step removes it. Record it for every agent. |
| `modelsResolved`, `toolsResolved`, `subagentsResolved`, `middlewareResolved` | yes | `true` = list complete (empty means none). `false` = there may be more (empty means unknown). |

### Agent `pattern` values

| Value | When |
|---|---|
| `create_react_agent`, `StateGraph`, `AgentExecutor`, `create_deep_agent` | This agent **is** the direct result of that call. |
| `dict_config` | A plain dict or object literal such as `{"name": …, "system_prompt": …, "tools": […]}` passed into a `subagents=[…]` list. Common for sub-agents. A sub-agent defined this way is `dict_config` even when its parent was built with `create_deep_agent`. |
| `yaml_config` | Defined in a YAML or JSON config file. |
| `class_definition` | Defined as a class. |
| `ag2_ConversableAgent`, `ag2_UserProxyAgent`, `ag2_GroupChatManager` | AG2 (AutoGen) constructs. |
| any other value | Use the framework's own constructor name exactly as written and add a warning. |

## Tool

| Field | Required | Meaning |
|---|---|---|
| `name` | yes | As in the source. 3 or more characters, unique. |
| `definitionPattern` | yes | One of the values below. |
| `description` | no | Verbatim docstring or description. |
| `origin` | yes | `{ "type": "<same value as definitionPattern>", "reference": "<external id if any>", "dynamic": true/false }`. Use `dynamic: true` when the tool list is resolved at runtime. `reference` is required for a `uc_function` (the catalog path) and an `mcp_dynamic` source. |
| `args` | yes | List of `{ "name", "type", "required", "default", "description" }`. `name` and `required` always; the rest only when present in the source. Empty list if the tool takes no arguments. |
| `returns` | no | `{ "type": "<as declared>" }`. Omit if not declared. |
| `guardrails` | yes | In-code checks that compare an input against a rule the code defines and then reject, block, require approval or rewrite the input (Stage 05 says what counts). Empty list if you looked and found none. |
| `usedByAgents` | yes | Names of agents that call this tool. |
| `evidence` | no | Where the tool and its guardrails are defined: a list of `"path:line"` strings, relative to the scan root. For review only; the finalize step removes it. Record it for every tool. |

### Tool `definitionPattern` values

Use the strongest rule that applies. Never invent a label.

| Value | When |
|---|---|
| `decorator_tool` | A function with an explicit tool decorator (for example `@tool`). |
| `structured_tool` | A plain function, method or lambda put straight into a tools list with no decorator; the framework wraps it. |
| `tool_wrapper` | A function whose body calls another tool or function, wrapping it. |
| `uc_function` | A Unity Catalog function reference. |
| `mcp_dynamic` | The tool list is resolved at runtime from an MCP server. |
| `databricks_builtin_function` | Calls a Databricks built-in AI function. |
| `other` | None of the above fit. |

## Guardrail

| Field | Required | Meaning |
|---|---|---|
| `kind` | yes | Lowercase snake_case. Known: `threshold`, `allowlist`, `denylist`, `scope_restriction`, `size_limit`, `authentication`, `approval_required`, `rate_limit`, `format_validation`, `argument_combination`, `external_check`. Use the closest known value. |
| `description` | yes | One sentence for a human reviewer. |
| `argsInvolved` | yes | Which of the tool's arguments the check reads or constrains. |
| `behavior` | yes | Lowercase snake_case. Known: `raises`, `returns_error`, `silently_skips`, `requires_approval`, `logs_and_continues`, `modifies_args`. |
| `confidence` | yes | Number from 0 to 1. |

## Example (one root, one tool)

```json
{
  "agents": [{
    "name": "support_triage_agent",
    "role": "ROOT",
    "pattern": "create_react_agent",
    "framework": { "name": "langgraph", "version": "0.2.15" },
    "models": [{ "provider": "anthropic", "modelId": "claude-sonnet-4-5",
                 "parameters": { "temperature": 0 }, "isDefault": true }],
    "systemPrompt": { "text": "You triage support tickets for Acme.", "dynamic": false },
    "middleware": [],
    "tools": ["search_kb"],
    "subagents": [],
    "modelsResolved": true, "toolsResolved": true,
    "subagentsResolved": true, "middlewareResolved": true
  }],
  "tools": [{
    "name": "search_kb",
    "definitionPattern": "decorator_tool",
    "description": "Search the knowledge base.",
    "origin": { "type": "decorator_tool", "dynamic": false },
    "args": [{ "name": "query", "type": "str", "required": true },
             { "name": "limit", "type": "int", "required": false, "default": 5 }],
    "returns": { "type": "list[str]" },
    "guardrails": [{ "kind": "size_limit", "description": "Rejects queries over 500 characters.",
                     "argsInvolved": ["query"], "behavior": "raises", "confidence": 0.9 }],
    "usedByAgents": ["support_triage_agent"]
  }],
  "warnings": []
}
```
