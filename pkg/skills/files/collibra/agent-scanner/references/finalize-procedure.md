# Finalize procedure

What the finalize step does, written as a specification. You write a small program from it (Python 3 with the standard library, or bash with `jq` and `shasum -a 256` or `sha256sum`), run it on `extraction.json`, and keep it in the output folder as `finalize-agent-scan.py` or `.sh`. Use Python if both are available: the hashing rules below are easier to get right there.

**You must pass the self-check at the end before you run it on the customer's extraction.** A program that does not reproduce the test hashes is wrong, whatever it looks like.

## Input and output

- Input: `extraction.json` with `agents`, `tools`, `warnings`, and optionally `platform` and `source` (below), and on the root agent a `deployment` object, `{ "version": "...", "endpoint": { "url": "...", "name": "..." } }`, every part optional except that an `endpoint` needs its `url`.
- Output: `agent_scan.json`, the complete document, written in the output folder next to `extraction.json`.
- Print to the terminal (format is free): counts of agents, tools and warnings, the document `contentHash`, and a suggested agent id (see step 8).
- Exit with an error and a list of problems, writing no output file, if step 2 finds any.

## Step 1: Normalise

Wherever the objects exist (never create an absent system prompt or guardrail):

- Delete the `evidence` field from every agent and tool. It is for review only and is not part of the document or any hash.
- Set `via` to `"LLM_FALLBACK"` on all six of these: every agent; every tool; every entry of an agent's `models`; an agent's `systemPrompt`; every entry of an agent's `middleware`; every entry of a tool's `guardrails`. The schema requires `via` on each of them, and a document missing it on any one is rejected.
- Lowercase, skipping any that is absent (these must be strings; reject the extraction if one is present and not a string): `framework.name`, `models[].provider`, `middleware[].hook`, `guardrails[].kind`, `guardrails[].behavior`.
- Make `models`, `middleware`, `tools`, `subagents`, `args`, `guardrails`, `usedByAgents` and `argsInvolved` arrays when missing (never null).

## Step 2: Reject if any of these hold

List every problem found, not only the first.

- `agents` or `tools` is not an array.
- An agent or tool name is shorter than 3 characters. A missing name counts as empty, so it is rejected here too.
- Two agents share a name, or two tools share a name.
- The number of agents with role `ROOT` is not exactly 1.
- An agent role is not `ROOT`, `SUBAGENT` or `ORPHAN`.
- An agent has a non-empty `models` list without exactly one `isDefault: true`.
- An agent lacks a boolean `modelsResolved`, `toolsResolved`, `subagentsResolved` or `middlewareResolved`.
- `platform` is present but is not an object with only `name` and `metadata`, its `name` is not UPPER_SNAKE_CASE (capital letters, digits and underscores, starting with a letter, for example `DATABRICKS`), or its `metadata` is present and not an object.
- An agent carries the old fields `deploymentVersion` or `endpointUrl`: the service refuses them, so the extraction must use `deployment` instead.
- An agent that is not the ROOT carries `deployment`.
- `deployment` is not an object, `deployment.version` is not a string, or `deployment.endpoint.url` is not a string or carries credentials (a user part before the `@` of the address, such as `https://user:password@host/path`).

Before these checks a blank `version` or `name` counts as absent and is removed, and an `endpoint` with no `url` is dropped with the warning "deployment.endpoint had no url and was dropped". A `deployment` left empty is removed.

## Step 3: Repair references (warn, do not reject)

Remove any name in an agent's `tools` that is not a tool name, any name in an agent's `subagents` or a tool's `usedByAgents` that is not an agent name. For each removal, append to `warnings`: `<owner>.<field> referenced '<name>', which matches no known name; dropped`.

For a guardrail without `confidence`, set it to `0.5` and append the warning `guardrail '<kind>' on '<tool>' had no confidence; set to 0.5`.

## Hashing conventions (apply to steps 4 to 6)

- Every "concatenate" joins the pieces directly: **no separator** of any kind.
- Every "sorted ascending" is plain code-point order, case-sensitive (`Z` sorts before `a`). No locale-aware or case-insensitive sorting.
- A hash is the SHA-256 of the UTF-8 bytes of the string, written as 64 lowercase hex characters.

## Step 4: Tool hash

For each tool whose `definitionPattern` is not `mcp_dynamic`, concatenate the following into one string, in this order, then take the SHA-256 of its UTF-8 bytes as 64 lowercase hex characters:

1. For each argument, sorted by `name` (ascending): `name`; `type` (empty string if absent or null); `true` or `false` for `required`; the `default` formatted per "Value formatting", only if present and not null.
2. `returns.type`, if present and non-empty.
3. For each guardrail, sorted by the key `kind + "|" + behavior + "|" + argsInvolved joined with ","` (ascending): `kind`; every `argsInvolved` entry in its listed order, joined directly; `behavior`. Guardrails with equal keys produce identical text, so their order among themselves does not matter.

Not included: descriptions, `confidence`, `origin`, `definitionPattern`, `usedByAgents`, `via`. A tool with `definitionPattern` `mcp_dynamic` gets `contentHash: null`, which contributes nothing anywhere.

## Step 5: Agent hash

For each agent, build a text from these parts, in this order:

1. `pattern` (empty if absent), `framework.name`, `framework.version` (empty if absent).
2. If the agent has a model with `isDefault: true`: its `provider`, `modelId`, then for each of `temperature`, `top_p`, `max_tokens` in that order, if present in its `parameters`: the key, then the value formatted per "Value formatting". Other parameter keys and models that are not the default are ignored.
3. `systemPrompt.text`, if there is a system prompt.
4. The `contentHash` of each tool the agent lists in `tools` (skip tools with null hash), sorted ascending as strings.
5. The **subagent term** (below), a list of hash strings sorted ascending.
6. For each middleware entry sorted by `class + hook` (ascending): `class`, `hook`.
7. `true` or `false` for `crossSessionPersistence`, only if it is present and not null.

Not included: `name`, `role`, `via`, `*Resolved` flags, `systemPrompt.dynamic` and `templatingVars`.

The agent's **own-content hash** is the SHA-256 of that text with the subagent term left empty. Its `contentHash` is the SHA-256 of the text with the subagent term in place.

### The subagent term, and delegation loops

A subagent's own hash depends on its subagents, so a change deep in the chain reaches every agent above it. Delegation can loop, so loops are hashed as one unit:

1. **Loop groups.** Agents that can all reach each other through `subagents` form a loop group. An agent in no loop is a group of one.
2. **Group hash.** A group of more than one has a group hash: the SHA-256 of its members' own-content hashes, sorted ascending and joined directly.
3. **The term for an agent.** Take the `contentHash` of every subagent the agent lists that is **outside** its group (one entry per listed name), and add the group hash **once** if the group has more than one member. Sort the entries ascending. A listed subagent inside the group contributes nothing of its own, including an agent that lists itself. A group of one adds nothing: it hashes exactly as the formula reads.
4. **Order of work.** Hash the agents outside a group before the group's members. If agents form no loops, this is a plain bottom-up calculation.

A change to any member of a loop group moves the hash of every member.

## Step 6: Document hash

Concatenate every agent's `contentHash` sorted ascending, then every non-null tool `contentHash` sorted ascending, then a newline, the ROOT agent's `deployment.version`, a newline, and the ROOT agent's `deployment.endpoint.url` exactly as it is written, and take the SHA-256. Use an empty string for a value that is missing, so a document with no deployment hashes the sorted hashes followed by two newlines. Names and order of the arrays do not matter. `deployment.endpoint.name` is not hashed.

So a redeploy (a new version) or a move to a new endpoint address is a new document hash, and with it a new version in Collibra, while the agents' and tools' own hashes do not move.

## Step 7: Assemble the document

```
fingerprintVersion: "1.0"
parser: { version: "skill-0.1", llmProvider: "CUSTOMER" }
generatedOn: current UTC time, whole seconds only (no fractions), "YYYY-MM-DDTHH:MM:SSZ"
contentHash: the Step 6 hash
platform: { name, metadata }     only if the extraction has one; metadata only if non-empty
source: { repoUrl, commitSha }   the extraction's `source` if it has one, else found by step 8; each key only if non-empty
agents: the normalised agents, each with its contentHash
tools: the normalised tools, each with its contentHash (null for mcp_dynamic)
warnings: your warnings plus those added above
```

`generatedOn`, `source` and `platform` are not hashed. `deployment` is in no agent or tool hash; its `version` and `endpoint.url` are in the document hash (Step 6). Write with two-space indentation and UTF-8, without escaping non-ASCII characters.

**Shape check, before writing the file.** The schema rejects missing required keys and unknown keys, so confirm each object has exactly these (extraction-only fields such as `evidence` are already removed):

- document: `fingerprintVersion`, `parser`, `generatedOn`, `contentHash`, `agents`, `tools`, `warnings`, and `source` and `platform` only if present.
- platform: `name`, and `metadata` when given.
- agent: `name`, `contentHash`, `role`, `pattern`, `via`, `modelsResolved`, `models`, `toolsResolved`, `tools`, `subagentsResolved`, `subagents`, `middlewareResolved`, `middleware`, plus `framework`, `systemPrompt` and `crossSessionPersistence` when they exist, and on the ROOT agent only, `deployment` (`version`, and `endpoint` with `url` and `name`) when given.
- model: `provider`, `modelId`, `isDefault`, `via`, and `parameters` when found.
- system prompt: `text`, `via`, and `templatingVars` and `dynamic` when given.
- middleware entry: `class`, `hook`, `via`.
- tool: `name`, `contentHash`, `definitionPattern`, `origin`, `args`, `guardrails`, `usedByAgents`, `via`, plus `description` and `returns` when they exist.
- guardrail: `kind`, `description`, `argsInvolved`, `behavior`, `confidence`, `via`.

The expected document for V1 below shows the whole shape.

## Step 8: Source and agent id

- Source: if the extraction carries a `source` (for example when the code scanned is an extracted deployed commit and not the working copy), use it as is and skip git. Otherwise run `git -C <scan root> remote get-url origin` and `git -C <scan root> rev-parse HEAD`, where `<scan root>` is the customer's repository folder agreed in Stage 00 (not the output folder; pass it to the program as an argument). Ignore a failure. Never fail the run because git is missing.
- Suggested agent id: `<repoUrl>#<ROOT agent name>`, or just the ROOT agent's name if there is no repo URL. The program only prints this; the id the customer sees is the one Stage 00 decided, which has the route from the platform check in front of it (`custom:` when no route was found), so the skill adds that prefix to this suggestion and never sends the bare form.

## Value formatting (for `default` and model parameters)

A JSON value is written as text like this:

| JSON value | Text |
|---|---|
| string | as is |
| `true` / `false` | `true` / `false` |
| number with no fraction (`5`, `5.0`, `0`) | digits only: `5`, `5`, `0` |
| other number (`0.7`) | shortest decimal that reads back to the same number: `0.7`. A number that would need exponent notation (below `0.0001` or from `1e21` up) is written exactly as it appears in the source. This is rare; if your tooling cannot keep the source text, add a warning naming the value and use your language's shortest form. |
| `null` | not written at all (a null `default` is skipped, as step 4 says) |
| list | `[` + items joined by one space + `]` |
| object | `map[` + `key:value` pairs sorted by key, joined by one space + `]` |

## Self-check (mandatory)

Before the first real run, build each of the extractions below, run your steps 1 to 6 on it, and compare. For V1, also assemble the whole document and compare it with the expected document that follows V1 (leave out `generatedOn` when comparing). They are already normalised and valid.

- If any hash differs, find the difference against the steps above and fix the program. Do not adjust a test value.
- Run the real extraction only when all of them match.
- If you cannot make them match after three attempts, stop and tell the customer the document cannot be completed here. Do not send or hand off a document whose hashes failed the check.

### V1: one agent, one tool with a default, a guardrail, a model and middleware

Extraction (already lowercase; only the fields shown):

```json
{"agents":[{"name":"triage_agent","role":"ROOT","pattern":"create_react_agent","framework":{"name":"langgraph","version":"0.2.15"},"models":[{"provider":"anthropic","modelId":"claude-x","parameters":{"temperature":0,"max_tokens":1024},"isDefault":true}],"middleware":[{"class":"AuditMiddleware","hook":"wrap_tool_call"}],"systemPrompt":{"text":"You triage tickets."},"tools":["search_kb"],"subagents":[],"modelsResolved":true,"toolsResolved":true,"subagentsResolved":true,"middlewareResolved":true}],"tools":[{"name":"search_kb","definitionPattern":"decorator_tool","origin":{"type":"decorator_tool","dynamic":false},"args":[{"name":"query","type":"str","required":true},{"name":"limit","type":"int","required":false,"default":5}],"guardrails":[{"kind":"size_limit","description":"d","argsInvolved":["query"],"behavior":"raises","confidence":0.9}],"usedByAgents":["triage_agent"],"returns":{"type":"list[str]"}}],"warnings":[]}
```

Expected hashes:

| Item | SHA-256 |
|---|---|
| agent `triage_agent` | `006d726e0127bdfec3eecf519e3f17872720c20b6cdc69f6bad9ac960cb871c1` |
| tool `search_kb` | `a5803c8312657afe9d0aab91c502ce8573e4d90b2ab2af9661e8f25e16c430a4` |
| document `contentHash` | `4453028e53aab59c475ed7ea85f6bfddd6c5fd7b0c9cf06f65e2e5fea5803eb4` |

### Expected document for V1

The whole document the steps produce for V1, with `generatedOn` left out (it is the current time). Key order is free; keys and values must match.

```json
{
  "fingerprintVersion": "1.0",
  "parser": {
    "version": "skill-0.1",
    "llmProvider": "CUSTOMER"
  },
  "contentHash": "4453028e53aab59c475ed7ea85f6bfddd6c5fd7b0c9cf06f65e2e5fea5803eb4",
  "agents": [
    {
      "name": "triage_agent",
      "role": "ROOT",
      "pattern": "create_react_agent",
      "framework": {
        "name": "langgraph",
        "version": "0.2.15"
      },
      "models": [
        {
          "provider": "anthropic",
          "modelId": "claude-x",
          "parameters": {
            "temperature": 0,
            "max_tokens": 1024
          },
          "isDefault": true,
          "via": "LLM_FALLBACK"
        }
      ],
      "middleware": [
        {
          "class": "AuditMiddleware",
          "hook": "wrap_tool_call",
          "via": "LLM_FALLBACK"
        }
      ],
      "systemPrompt": {
        "text": "You triage tickets.",
        "via": "LLM_FALLBACK"
      },
      "tools": [
        "search_kb"
      ],
      "subagents": [],
      "modelsResolved": true,
      "toolsResolved": true,
      "subagentsResolved": true,
      "middlewareResolved": true,
      "via": "LLM_FALLBACK",
      "contentHash": "006d726e0127bdfec3eecf519e3f17872720c20b6cdc69f6bad9ac960cb871c1"
    }
  ],
  "tools": [
    {
      "name": "search_kb",
      "definitionPattern": "decorator_tool",
      "origin": {
        "type": "decorator_tool",
        "dynamic": false
      },
      "args": [
        {
          "name": "query",
          "type": "str",
          "required": true
        },
        {
          "name": "limit",
          "type": "int",
          "required": false,
          "default": 5
        }
      ],
      "guardrails": [
        {
          "kind": "size_limit",
          "description": "d",
          "argsInvolved": [
            "query"
          ],
          "behavior": "raises",
          "confidence": 0.9,
          "via": "LLM_FALLBACK"
        }
      ],
      "usedByAgents": [
        "triage_agent"
      ],
      "returns": {
        "type": "list[str]"
      },
      "via": "LLM_FALLBACK",
      "contentHash": "a5803c8312657afe9d0aab91c502ce8573e4d90b2ab2af9661e8f25e16c430a4"
    }
  ],
  "warnings": []
}
```

### V2: a parent and one subagent

Extraction (already lowercase; only the fields shown):

```json
{"agents":[{"name":"planner_agent","role":"ROOT","pattern":"create_react_agent","framework":{"name":"langgraph","version":"0.2.15"},"models":[],"middleware":[],"systemPrompt":{"text":"Plan."},"tools":[],"subagents":["worker_agent"],"modelsResolved":true,"toolsResolved":true,"subagentsResolved":true,"middlewareResolved":true},{"name":"worker_agent","role":"SUBAGENT","pattern":"create_react_agent","framework":{"name":"langgraph","version":"0.2.15"},"models":[],"middleware":[],"systemPrompt":{"text":"Work."},"tools":[],"subagents":[],"modelsResolved":true,"toolsResolved":true,"subagentsResolved":true,"middlewareResolved":true}],"tools":[],"warnings":[]}
```

Expected hashes:

| Item | SHA-256 |
|---|---|
| agent `planner_agent` | `74f91f4114364655ec52fd41e159b5753d0f65d38685b65b79b01a743a1e7370` |
| agent `worker_agent` | `6e8d9febeb18d4fece218ba2d53c7b5e558ffb4862f1b5bcb2dd381723120908` |
| document `contentHash` | `d5058d00172ea6182f49f812adbcc66431a78ada30942f4ab582e1c5444fdcde` |

### V3: two agents that delegate to each other (a loop group)

The group hash stands in for both members.

Extraction (already lowercase; only the fields shown):

```json
{"agents":[{"name":"alpha_agent","role":"ROOT","pattern":"create_react_agent","framework":{"name":"langgraph","version":"0.2.15"},"models":[],"middleware":[],"systemPrompt":{"text":"A."},"tools":[],"subagents":["beta_agent"],"modelsResolved":true,"toolsResolved":true,"subagentsResolved":true,"middlewareResolved":true},{"name":"beta_agent","role":"SUBAGENT","pattern":"create_react_agent","framework":{"name":"langgraph","version":"0.2.15"},"models":[],"middleware":[],"systemPrompt":{"text":"B."},"tools":[],"subagents":["alpha_agent"],"modelsResolved":true,"toolsResolved":true,"subagentsResolved":true,"middlewareResolved":true}],"tools":[],"warnings":[]}
```

Expected hashes:

| Item | SHA-256 |
|---|---|
| agent `alpha_agent` | `45ee6259bb29b28527eb45ec4cf40329c5a207a9f6a316c942fa7947198a3111` |
| agent `beta_agent` | `f1eb32bdedf7862b974732f13084e78d0ebe6ba917d965aa8fe1abb26e1d4d5c` |
| document `contentHash` | `171178c09fa2f9413350464cb1e94df27fbb0962f810fe3d4327f9350fb6be64` |

### V4: rules the first three do not exercise

Covers: out-of-order middleware and guardrails, a non-default model (ignored), parameters that are and are not functional (`seed` is ignored), a number that formats as an integer (`top_p: 1`), `crossSessionPersistence: true`, defaults that are a list, a string and `false`, an argument with no `type`, an `mcp_dynamic` tool with a null hash, and `modelsResolved: false` (not hashed).

Extraction (already lowercase; only the fields shown):

```json
{"agents":[{"name":"ops_agent","role":"ROOT","pattern":"StateGraph","framework":{"name":"langgraph"},"models":[{"provider":"openai","modelId":"gpt-x","parameters":{"temperature":0.7,"top_p":1,"seed":42},"isDefault":true},{"provider":"anthropic","modelId":"aux","isDefault":false}],"middleware":[{"class":"ZMiddleware","hook":"wrap_model_call"},{"class":"AMiddleware","hook":"wrap_tool_call"}],"crossSessionPersistence":true,"tools":["zip_files","list_files"],"subagents":[],"modelsResolved":false,"toolsResolved":true,"subagentsResolved":true,"middlewareResolved":true}],"tools":[{"name":"zip_files","definitionPattern":"structured_tool","origin":{"type":"structured_tool","dynamic":false},"args":[{"name":"paths","type":"list","required":true,"default":["a","b"]},{"name":"level","required":false,"default":"max"},{"name":"dry_run","type":"bool","required":false,"default":false}],"guardrails":[{"kind":"threshold","description":"x","argsInvolved":["level","paths"],"behavior":"raises","confidence":0.8},{"kind":"allowlist","description":"y","argsInvolved":["paths"],"behavior":"returns_error","confidence":0.6}],"usedByAgents":["ops_agent"]},{"name":"list_files","definitionPattern":"mcp_dynamic","origin":{"type":"mcp_dynamic","reference":"srv","dynamic":true},"args":[],"guardrails":[],"usedByAgents":["ops_agent"]}],"warnings":[]}
```

Expected hashes:

| Item | SHA-256 |
|---|---|
| agent `ops_agent` | `0e578797afe65a1ad3f1ff253bd1eaf60f05a9d0ece710bbed7b49c16f08fdc2` |
| tool `zip_files` | `86fb14bbc79f488080641790e47b186a10d32758fd1c7799bbd46ff79a02b407` |
| tool `list_files` | `null` (no hash) |
| document `contentHash` | `c978f550b9892199d07d51fc14a323bc2d8c08ccb69d94314d7c264f9bfbf32f` |

### V5: a subagent term next to middleware and persistence

Checks that the subagent hash sits before the middleware terms.

Extraction (already lowercase; only the fields shown):

```json
{"agents":[{"name":"lead_agent","role":"ROOT","pattern":"create_react_agent","framework":{"name":"langgraph","version":"0.2.15"},"models":[],"middleware":[{"class":"Audit","hook":"wrap_tool_call"}],"tools":[],"subagents":["aide_agent"],"modelsResolved":true,"toolsResolved":true,"subagentsResolved":true,"middlewareResolved":true,"systemPrompt":{"text":"Lead."},"crossSessionPersistence":true},{"name":"aide_agent","role":"SUBAGENT","pattern":"create_react_agent","framework":{"name":"langgraph","version":"0.2.15"},"models":[],"middleware":[],"tools":[],"subagents":[],"modelsResolved":true,"toolsResolved":true,"subagentsResolved":true,"middlewareResolved":true,"systemPrompt":{"text":"Aide."}}],"tools":[],"warnings":[]}
```

Expected hashes:

| Item | SHA-256 |
|---|---|
| agent `lead_agent` | `63c39683a704ab0ff8feec4e54b90a99e9742e44aceac37010934f7444be9a90` |
| agent `aide_agent` | `657bdbcaf94453375a19579ffc2e29a480d3bb85ce1c050ade29cd08bcddc1d6` |
| document `contentHash` | `7fcc9a5ac48310ba9940f4d410a0bd1a22e0e7736442c2b62c9c2806e64127f8` |

### V6: a loop with a subagent outside it

`beta_agent` lists `alpha_agent` (inside the loop) and `gamma_agent` (outside it); `gamma_agent` lists itself, which adds nothing.

Extraction (already lowercase; only the fields shown):

```json
{"agents":[{"name":"alpha_agent","role":"ROOT","pattern":"create_react_agent","framework":{"name":"langgraph","version":"0.2.15"},"models":[],"middleware":[],"tools":[],"subagents":["beta_agent"],"modelsResolved":true,"toolsResolved":true,"subagentsResolved":true,"middlewareResolved":true,"systemPrompt":{"text":"A."}},{"name":"beta_agent","role":"SUBAGENT","pattern":"create_react_agent","framework":{"name":"langgraph","version":"0.2.15"},"models":[],"middleware":[],"tools":[],"subagents":["alpha_agent","gamma_agent"],"modelsResolved":true,"toolsResolved":true,"subagentsResolved":true,"middlewareResolved":true,"systemPrompt":{"text":"B."}},{"name":"gamma_agent","role":"SUBAGENT","pattern":"create_react_agent","framework":{"name":"langgraph","version":"0.2.15"},"models":[],"middleware":[],"tools":[],"subagents":["gamma_agent"],"modelsResolved":true,"toolsResolved":true,"subagentsResolved":true,"middlewareResolved":true,"systemPrompt":{"text":"G."}}],"tools":[],"warnings":[]}
```

Expected hashes:

| Item | SHA-256 |
|---|---|
| agent `alpha_agent` | `45ee6259bb29b28527eb45ec4cf40329c5a207a9f6a316c942fa7947198a3111` |
| agent `beta_agent` | `32621094d46710cc5fc1ed8e169f99ea85ba5ba91740c53a7cce051022d83460` |
| agent `gamma_agent` | `dfc12ef4aa1d8f6bc82b23fe0341da989c3b05d1df9dab98adddeaa515fa48b2` |
| document `contentHash` | `def5cc418a76377cd54c80f3da324bc2bcea05cd60c7b3da4ec404592b419928` |

### V7: platform fields, a deployment and an explicit source (V1 plus these)

Checks that `platform`, `deployment` and `source` appear in the document, that `platform`, `source` and the endpoint's `name` change no hash, and that the deployment's `version` and `endpoint.url` change only the document hash: the agent and tool hashes equal V1's.

Extraction:

```json
{"agents":[{"name":"triage_agent","role":"ROOT","pattern":"create_react_agent","framework":{"name":"langgraph","version":"0.2.15"},"models":[{"provider":"anthropic","modelId":"claude-x","parameters":{"temperature":0,"max_tokens":1024},"isDefault":true}],"middleware":[{"class":"AuditMiddleware","hook":"wrap_tool_call"}],"systemPrompt":{"text":"You triage tickets."},"tools":["search_kb"],"subagents":[],"modelsResolved":true,"toolsResolved":true,"subagentsResolved":true,"middlewareResolved":true,"deployment":{"version":"42","endpoint":{"url":"https://example.cloud.databricks.com/serving-endpoints/agents_demo-example_schema-example_agent/invocations","name":"agents_demo-example_schema-example_agent"}}}],"tools":[{"name":"search_kb","definitionPattern":"decorator_tool","origin":{"type":"decorator_tool","dynamic":false},"args":[{"name":"query","type":"str","required":true},{"name":"limit","type":"int","required":false,"default":5}],"guardrails":[{"kind":"size_limit","description":"d","argsInvolved":["query"],"behavior":"raises","confidence":0.9}],"usedByAgents":["triage_agent"],"returns":{"type":"list[str]"}}],"warnings":[],"platform":{"name":"DATABRICKS","metadata":{"ucModelName":"example_catalog.example_schema.example_agent"}},"source":{"repoUrl":"git@example.com:acme/agents.git","commitSha":"1111111111111111111111111111111111111111"}}
```

Expected hashes:

| Item | SHA-256 |
|---|---|
| agent `triage_agent` | `006d726e0127bdfec3eecf519e3f17872720c20b6cdc69f6bad9ac960cb871c1` |
| tool `search_kb` | `a5803c8312657afe9d0aab91c502ce8573e4d90b2ab2af9661e8f25e16c430a4` |
| document `contentHash` | `a53720296887565a12cdeb786f2867e11f9eb2fff07908c8adf6892209c5defc` |

### Expected document for V7

```json
{
  "fingerprintVersion": "1.0",
  "parser": {
    "version": "skill-0.1",
    "llmProvider": "CUSTOMER"
  },
  "contentHash": "a53720296887565a12cdeb786f2867e11f9eb2fff07908c8adf6892209c5defc",
  "agents": [
    {
      "name": "triage_agent",
      "role": "ROOT",
      "pattern": "create_react_agent",
      "framework": {
        "name": "langgraph",
        "version": "0.2.15"
      },
      "models": [
        {
          "provider": "anthropic",
          "modelId": "claude-x",
          "parameters": {
            "temperature": 0,
            "max_tokens": 1024
          },
          "isDefault": true,
          "via": "LLM_FALLBACK"
        }
      ],
      "middleware": [
        {
          "class": "AuditMiddleware",
          "hook": "wrap_tool_call",
          "via": "LLM_FALLBACK"
        }
      ],
      "systemPrompt": {
        "text": "You triage tickets.",
        "via": "LLM_FALLBACK"
      },
      "tools": [
        "search_kb"
      ],
      "subagents": [],
      "modelsResolved": true,
      "toolsResolved": true,
      "subagentsResolved": true,
      "middlewareResolved": true,
      "deployment": {
        "version": "42",
        "endpoint": {
          "url": "https://example.cloud.databricks.com/serving-endpoints/agents_demo-example_schema-example_agent/invocations",
          "name": "agents_demo-example_schema-example_agent"
        }
      },
      "via": "LLM_FALLBACK",
      "contentHash": "006d726e0127bdfec3eecf519e3f17872720c20b6cdc69f6bad9ac960cb871c1"
    }
  ],
  "tools": [
    {
      "name": "search_kb",
      "definitionPattern": "decorator_tool",
      "origin": {
        "type": "decorator_tool",
        "dynamic": false
      },
      "args": [
        {
          "name": "query",
          "type": "str",
          "required": true
        },
        {
          "name": "limit",
          "type": "int",
          "required": false,
          "default": 5
        }
      ],
      "guardrails": [
        {
          "kind": "size_limit",
          "description": "d",
          "argsInvolved": [
            "query"
          ],
          "behavior": "raises",
          "confidence": 0.9,
          "via": "LLM_FALLBACK"
        }
      ],
      "usedByAgents": [
        "triage_agent"
      ],
      "returns": {
        "type": "list[str]"
      },
      "via": "LLM_FALLBACK",
      "contentHash": "a5803c8312657afe9d0aab91c502ce8573e4d90b2ab2af9661e8f25e16c430a4"
    }
  ],
  "warnings": [],
  "source": {
    "repoUrl": "git@example.com:acme/agents.git",
    "commitSha": "1111111111111111111111111111111111111111"
  },
  "platform": {
    "name": "DATABRICKS",
    "metadata": {
      "ucModelName": "example_catalog.example_schema.example_agent"
    }
  }
}
```
