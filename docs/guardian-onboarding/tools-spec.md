# Tools

Four new tools and one change to an existing tool. Follow `docs/TOOL_CONTRIBUTION_STANDARDS.md` and use the existing
Guardian tools (`publish_agent_scan`, `list_scanned_agents`, `get_latest_agent_scan`) as the pattern: one tool per
capability, inputs validated before any network call, the shared HTTP helper, explicit MCP annotations, a description
written as a full paragraph with example questions, errors that tell the assistant how to recover, and writes behind a
confirm checkpoint that echoes every field.

The tools are platform-neutral: nothing in them is specific to Databricks. Like the other Guardian tools on this branch,
they're registered without a capability flag for the private preview; gating is decided with the chip owners before
merging to `main`.

Authentication is the connection's, as for the existing Guardian tools. The Guardian APIs accept the instance's basic
authentication and OAuth bearer tokens; the scopes are named below for OAuth clients.

## `get_agent_guardian_status` (read)

One answer to "where is this agent in its Guardian setup?".

- **Calls:** `GET /rest/guardianOverview/v1/aiAgents/{aiAgentId}` (scope `aigovernance.guardian.overview-read`).
- **Input:** `aiAgentId` (UUID, required).
- **Returns**, passed through with the field names unchanged:
  - `aiAgent {id, name}`;
  - `fingerprint {status: IN_PROGRESS | COMPLETED | FAILED, id, createdOn, lastReceivedOn, aiAgentVersion {id, name}, failureMessage}`;
  - `contracts [{id, name, layer: GLOBAL | SPECIALIZED, activeVersionId}]`, global first, empty when none governs the agent;
  - `compilation {status: NOT_COMPILED | IN_PROGRESS | FAILED | STALE | UP_TO_DATE, staleBecause [CONTRACT_VERSION_CHANGED | GOVERNING_SET_CHANGED | FINGERPRINT_CHANGED], latestJob {state, createdOn, failureMessage}, latestBundle {id, version, compiledAt, agentVersionId, agentFingerprintId, contractVersionIds}}`;
  - `enforcement {status: NOT_OBSERVED | UP_TO_DATE | STALE, observedBundle}`;
  - `collection {status: OK | NOT_COLLECTED | FAILING | LAGGING, reason: NO_CONNECTION | SOURCE_NOT_CONNECTED | AGENT_NOT_FOUND, lastCollectedOn}`.
  The job's id is left out, as in `apply_latest_rules`. Any block may be missing on an older instance; return what's there and say which blocks were absent.
- **Errors:** 403 `accessDenied`, 404 `aiAgentNotFound` (say: the agent may still be importing, or the id is wrong),
  503 `agentContractsUnavailable` (transient).
- **Annotations:** read-only, idempotent.
- **Example questions:** "Is Guardian set up for my agent?", "Did the rules finish applying?", "Is Collibra reading my
  agent's activity?".

## `apply_latest_rules` (write)

The same action as **Apply latest rules** on the agent's Contract tab: turns the agent contracts that govern the agent
into a rules file for its newest version, with a report.

- **Calls:** `POST /rest/guardianEnforcement/v1/bundles/by/aiAgent/{aiAgentId}/compile` with no body (scope
  `aigovernance.guardian.enforcement-write`). Answers 202 with a job and a `Location` header.
- **Input:** `aiAgentId` (UUID, required), `confirm` (boolean, required).
- **Confirm checkpoint:** with `confirm=false`, call nothing; read `get_agent_guardian_status` and return a preview: the
  agent, its newest version, the governing contracts with their active versions, and the current compilation status.
  With `confirm=true`, send the request.
- **Returns:** `state` and `createdOn`. Not the job id: it describes how Collibra works inside, and the skill follows
  progress through `get_agent_guardian_status`.
- **Errors:** 404 `aiAgentNotFound` (never scanned, not imported yet, or unknown; the three look the same), 403 (a
  permission problem, never retry).
- **Don't** check for contracts inside the tool: a run with no usable rules completes without a rules file, and the
  skill explains that from the status.
- **Annotations:** not read-only, not idempotent (each call starts a run), not destructive.

## `get_agent_rules_report` (read)

What happened to each rule in the latest run.

- **Calls:** `GET /rest/guardianEnforcement/v1/compilationReports/by/aiAgent/{aiAgentId}` (scope
  `aigovernance.guardian.enforcement-read`).
- **Input:** `aiAgentId`.
- **Returns:** `bundleId` (absent when no rules file was made), `agentVersionId`, `startedAt`, `finishedAt`,
  `rules [{ruleId, agentContractId, agentContractVersionId, agentContractLayer, statement, outcome, reason}]` and a count
  per outcome. Outcomes: `bound`, `partially_bound`, `not_applicable`, `no_enforcement_required`, `unbindable`,
  `ambiguous`, `inexpressible`, `unsupported_pattern`, `needs_classifier`, `unbound`, `no_model_answer`. Leave out the
  guard detail: the skill never shows it.
- **Errors:** 404 `compilationReportNotFound`.
- **Annotations:** read-only, idempotent.

## `get_agent_rules_file` (read)

What the skill needs to download the agent's latest rules file and check it.

- **Calls:** `GET /rest/guardianEnforcement/v1/bundles/by/aiAgent/{aiAgentId}`, then
  `GET /rest/guardianEnforcement/v1/bundles/{bundleId}/file` **without following the redirect** (scope
  `aigovernance.guardian.enforcement-read`).
- **Input:** `aiAgentId`.
- **Returns:** `bundleId`, `version`, `agentVersionId`, `sizeInBytes`, `contentHash` (SHA-256, hex), `compiledAt`, and
  `downloadUrl`, the short-lived link from the redirect's `Location` header, marked sensitive: the description tells the
  assistant to use it once, never show it to the customer and never write it to a file.
- **Errors:** 404 `noBundleAvailable` (no rules file yet: point to `get_agent_guardian_status`), 404 `aiAgentNotFound`, 403.
- **Annotations:** read-only, idempotent.
- The tool doesn't download the file: chip may run remotely, and the file belongs on the customer's machine.

## Change: `edit_asset add_relation` takes a relation type id

Today `add_relation` resolves the relation type from its **role name** only. Several relation types can share a role on
the same asset type (an AI Use Case has several "uses" relations), and the last one wins, so a use case can be linked to
an agent through the wrong relation type.

- **Add** an optional `relationTypeId` (UUID) to the `add_relation` operation. When it's set, use it and skip the role
  lookup; check that the relation type is assigned to the edited asset's type (source or target side) and set the
  direction from that.
- **Keep** `relationType` (role name) for compatibility; when both are set, `relationTypeId` wins.
- **Also fix the ambiguity:** when a role or co-role name matches more than one relation type in the assignment, return an
  error that lists the candidates with their ids, instead of picking one.
- `prepare_create_asset` already returns `relationTypeId` for each relation slot, so the assistant can pass it straight on.
- **The cases that need it:** linking a specialised agent contract to an agent (relation type `...7018`, AI Agent to
  Agent Contract), and linking an AI use case to an AI agent uses `AIUseCaseUsesAIAgent`,
  `00000000-0000-0000-0000-000000007109`. AI Use Case's assignment has four relation types with the role "uses" (to AI
  Agent `...7109`, AI Base Model `...77002`, AI Project `...7112`, AI Model Version `...7098`), so `relationType: "uses"`
  resolves to whichever comes last and fails or links the wrong type.
- **Tests:** an assignment with two relation types sharing a role: `relationTypeId` picks the right one; the role name
  alone returns the ambiguity error with both candidates; a `relationTypeId` not in the assignment is refused before any
  call.

## Tests for every tool

- Unit tests with an HTTP test server: success, each documented error, and input validation before any call.
- `apply_latest_rules`: `confirm=false` sends nothing; `confirm=true` sends exactly one request.
- `get_agent_rules_file`: the redirect isn't followed, and the link is returned but never logged.
- Registration tests as in `pkg/tools/register_test.go`, and the skill check `TestServedSkillsOnlyNameRegisteredTools`.
