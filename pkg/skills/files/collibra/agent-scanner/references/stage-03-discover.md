# Stage 03: Discover agents

Find every agent construction in the scan root and decide each agent's role. Load `references/agent-scan-fields.md` with this stage.

## Before you start

If Stage 02 found a known agent, `known.json` holds its verdict. `unchanged`: skip this whole stage (Stage 02, Step 6). `changed`: read only the changed files in `known.json` and search them for the names of the agents and tools in the known document, instead of listing and reading the whole scan root. Anything else: continue as written.

## Reading budget

Budget your reading. Aim to finish discovery and extraction within roughly 40 file reads and searches for a small agent, more for a large codebase. Say so in your status line if the code is large. If sub-agents are available, note how many files construct agents or tools: Stage 05 splits the extraction above 8.

1. List the top-level folders. Read the dependency manifest (`pyproject.toml`, `requirements*.txt`, `package.json`, `go.mod`, `pom.xml`) for the framework and its version.
2. Search for agent-shaped code before reading whole files: framework imports and constructor calls (`create_react_agent`, `create_deep_agent`, `StateGraph`, `AgentExecutor`, `Agent(`, `ConversableAgent`, `@tool` decorators, `tools=[`, `subagents=[`, `system_prompt`, `instructions=`), and config files that define agents (YAML or JSON with `model`, `tools`, `prompt`).
3. Read the files that construct agents, subagents and tools. Skip or only skim tests, evals, mocks, fixtures, seed scripts, notebooks and demo scaffolding unless nothing else resolves a required field. Reading those first is the usual way to run out of budget.
4. It is better to finish with fields marked unresolved and a warning than to run out of budget with nothing written.

Any language and any framework is in scope, not only Python and LangChain.

## Decide roles

| Role | When |
|---|---|
| ROOT | Not delegated to by any other agent, and has a clear entry point (a `main`, a server route, a graph that is compiled and invoked, a deployed handler). |
| SUBAGENT | Delegated to by another agent you found (listed in its subagents, called as a tool, a node in the parent's graph). |
| ORPHAN | An agent definition that nothing else references and that has no entry point. |

### Unused definitions must be reported

An agent or subagent definition (a dict literal, a constructor call, a class) that no orchestrator imports or references is **still included**, with role ORPHAN and a warning saying why it looks unused. Dead code left by a refactor is one of the most useful things a fingerprint can show. Never leave it out because nothing uses it.

### Same agent built in two places

If one logical agent is constructed in more than one place (for example a studio entry file and a command-line entry file both build the orchestrator), report it once. If the two differ in anything that matters (model, tools, persistence, middleware, system prompt), add a warning naming both locations. Which one is deployed changes what is true at runtime, so do not silently pick one.

### Several independent roots

A fingerprint has exactly one ROOT. If you find several independent systems, follow what Stage 00 stated: fingerprint each one in turn (one file each), unless the customer named only one. List what you found in your status line. Agents of a system that is not being fingerprinted are not included, except as warnings if they are connected.

### Factory patterns

If an agent is created by a factory or at runtime so that you cannot tell its configuration from the code, do not guess. Omit it and add a warning naming the factory and file.

## Where it runs

Not here: Stage 01 already checked for a platform, and Stage 04 describes the deployment and may change the folder to scan, in which case you run this stage again on that folder.

## Exit criteria

- Every agent construction found is listed with a role, or a warning explains why it is left out.
- Exactly one ROOT chosen (by the customer if there was a choice).
- The delegation links between agents are known, or marked unresolved.

End with a table (agent name, role, file, framework) in your status line and carry straight on to Stage 05. Do not wait for confirmation: the customer sees the same table in the final review and can send you back to look for anything missing.
