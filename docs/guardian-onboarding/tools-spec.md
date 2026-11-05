# Tools

Four new tools, and one additive change to an existing tool (`edit_asset`). Follow `docs/TOOL_CONTRIBUTION_STANDARDS.md` and use the existing
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
  - `scan {status: IN_PROGRESS | COMPLETED | FAILED, id, createdOn, lastReceivedOn, aiAgentVersion {id, name}, failureMessage}`;
  - `contracts [{id, name, layer: GLOBAL | SPECIALIZED, activeVersionId}]`, global first, empty when none governs the agent;
  - `compilation {status: NOT_COMPILED | IN_PROGRESS | FAILED | STALE | UP_TO_DATE, staleBecause [CONTRACT_VERSION_CHANGED | GOVERNING_SET_CHANGED | SCAN_CHANGED], latestJob {state, createdOn, failureMessage}, latestBundle {id, version, compiledAt, agentVersionId, agentScanStateId, contractVersionIds}}`;
  - `enforcement {status: NOT_OBSERVED | UP_TO_DATE | STALE, observedBundle}`;
  - `collection {status: OK | NOT_COLLECTED | FAILING | LAGGING, reason: NO_CONNECTION | SOURCE_NOT_CONNECTED | AGENT_NOT_FOUND, lastCollectedOn}`.
  The job's id is left out, as in `apply_latest_rules`. Plus `links`: ready-made links to the agent, its newest version, its Contract and Guardian tabs, each governing
  contract, all contracts and all Guardian agents, built from the connection's Collibra address (absent when the address
  isn't known). The skill takes links from here and from `get_asset_details`' `link`, never building its own.
  Any block may be missing on an older instance; return what's there and say which blocks were absent.
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
- **Returns:** `bundleId` (absent when no rules file was made), `agentVersionId`, `startedAt`, `finishedAt`, and for
  each rule its statement, its contract, an outcome and a reason, with a count per outcome. The skill groups the
  outcomes into what the customer sees: checked, partly checked, for the record only, not about this agent, or couldn't
  be turned into a check as written. Nothing about how a rule is checked is returned.
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

## Change: `edit_asset add_relation` takes an optional relation type id

`add_relation` finds a relation type by its name on the edited asset and keeps the last match, without looking at the
other asset's type. So a name only works when it is unique on the edited asset:

- **Agent contract to AI Agent:** edit the AI Agent with `relationType` "complies to". On an AI Agent that name belongs
  to one relation type only, so the existing behaviour is enough.
- **AI use case to AI Agent:** no name is unique from either side. On an AI Use Case, "uses" belongs to four relation
  types (to AI Agent, AI Base Model, AI Project and AI Model Version); on an AI Agent, "is used by" belongs to two. A test
  on a live instance confirmed both are refused by Collibra (`relationTgtIncompatible`, `relationSrcIncompatible`).

The change, additive only:

- **Add** an optional `relationTypeId` (UUID) to `add_relation`. When it's set, use it instead of the name: check it is
  a relation type assigned to the edited asset's type (at either end) before any write, and set the direction from
  that. When both are given, `relationTypeId` wins.
- **Keep** the name lookup exactly as it is when `relationTypeId` isn't given.
- **Bulk adds** keep the direction of a relation where the edited asset is the tail (they used to always send the
  edited asset as the source).
- **The use case link:** edit the AI Use Case with `relationTypeId` `00000000-0000-0000-0000-000000007109`
  (`AIUseCaseUsesAIAgent`) and the agent as `targetAssetId`.
- **Tests:** the id picks the right one of two types sharing a name; the id wins over the name; the edited asset as the
  tail flips source and target; an id not assigned is refused before any write; an invalid id is refused; a name alone
  resolves as before; a bulk add keeps a tail relation's direction.

## Tests for every tool

- Unit tests with an HTTP test server: success, each documented error, and input validation before any call.
- `apply_latest_rules`: `confirm=false` sends nothing; `confirm=true` sends exactly one request.
- `get_agent_rules_file`: the redirect isn't followed, and the link is returned but never logged.
- Registration tests as in `pkg/tools/register_test.go`, and the skill check `TestServedSkillsOnlyNameRegisteredTools`.
