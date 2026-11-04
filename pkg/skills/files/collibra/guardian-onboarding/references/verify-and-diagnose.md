# Verify and diagnose

Load this only when a check fails, something fails, or a Collibra setting looks switched off. Each step lists its own checks; the table below is the full list. Load `references/support-details.md` only when you stop on a failure you can't explain or fix.

## Check your own work after every action

Before you tell the customer an action worked, check the result yourself. The customer never has to open Collibra to find out whether something happened.

| After | Check | With |
|---|---|---|
| A platform sign-in | The identity call answers, and it is the profile the customer chose | the platform tool (`shared/platform-prerequisites.md`) |
| The SDK install and project changes (step 1) | The packages import in the project's environment; the settings file parses and has `enforcement_mode` as chosen and no rules file setting yet; the entry module still parses (compiled in memory, never run); the changes on disk are exactly the changes shown | local commands |
| Sending the scan (step 2) | The agent appears with an id; its newest version is the one just sent; the fingerprint status is `COMPLETED` | `list_scanned_agents`, `get_agent_guardian_status` |
| A contract saved by the contract skill, or a contract linked by you or the customer (step 3) | It is in the list of contracts governing the agent, with the expected rule count | `list_agent_contracts` with `agentId`, `get_agent_contract` |
| Creating or linking a use case (step 4) | The agent's incoming relations include the use case, through "AI Use Case uses AI Agent" | `get_asset_details` on the agent |
| Applying the rules (step 5) | The status is `UP_TO_DATE`; the report's rule count equals the rules in the governing contracts; the report names the newest agent version | `get_agent_guardian_status`, `get_agent_rules_report` |
| Saving the rules file (step 6) | SHA-256 equals the reported hash; the archive holds `spec.yaml` whose `spec_bundle_id` equals the file's id; the SDK's own loader accepts the file; the file is at the path the setting names | local commands, `get_agent_rules_file` |
| Deploying, after the agent ran (step 7) | Enforcement shows the latest rules file (`observedBundle` matches `latestBundle`); collection `OK` | `get_agent_guardian_status` |

When a check fails, say what you did, what you found and what happens next, and never report success:

"I `<did X>`, but when I checked, `<what I found>`. `<What I'll do, or what you can do>`."

For example: "I applied the rules, but when I checked, Collibra still shows them as in progress after 15 minutes."

## When something fails: find the cause first

Read every signal the tools give you, name the most likely cause in plain words, and say what to do. Only when the cause can't be found or fixed from here, show the support details. Every failure message has four parts:

```text
What happened:   <plain words>
What I checked:  <the signals read>
Most likely:     <the cause, and how sure you are>
What to do:      <the fix, or the hand-off>; if that doesn't help, share the details below with Collibra support.
```

| Step | You see | Check next | Most likely cause, and what the customer hears |
|---|---|---|---|
| 1 | The SDK won't install | Python version; the downloaded folder; a missing Collibra dependency wheel | Wrong Python, an incomplete download, or a missing wheel: say which |
| 1 | The project's LangChain or LangGraph is outside the tested range | The installed versions | Offer (a) a tested version or (b) `framework_version_policy = "permissive"`, as the customer chooses |
| 1, 6 | Guardian says it found no rules file and allows everything | The path the setting names, and whether the file is there | Nothing looks broken, so warn about it explicitly: the agent runs unchecked until the rules file is in place |
| 2 | The scan is refused with an error code | The error's message names the field | The description has a mistake: the scanner fixes it and asks again |
| 2 | No agent id after five minutes | Whether `list_scanned_agents` answers; the fingerprint status | Collibra is busy: try again later. Otherwise the support details |
| 2 | Fingerprint `FAILED` | Its `failureMessage` | Say it plainly. A message about a domain or a permission goes to the Collibra administrator |
| 3 | No contract governs the agent | Is there a specialised contract the customer just wrote? | None: offer the contract skill. Not linked: offer to link it yourself, after the customer's yes; the customer links it in Collibra only if they prefer or your link fails |
| 3 | A contract can't be viewed | Which contracts could be read | The list may be incomplete: a message to the administrator about view rights |
| 5 | Agent not found (404) | The fingerprint status | Collibra hasn't finished adding the agent: wait, then try again |
| 5 | 403 | none | No permission: a message for the administrator; never retried |
| 5 | Compilation `FAILED` | `latestJob.failureMessage` | If it says to try again, ask once more whether to apply the rules; otherwise the support details |
| 5 | `NOT_COMPILED` after the run finished, or no rules file and an empty report | In order: no contract governs the agent; the governing contracts have no rules | Each points back to step 3. With neither, the support details |
| 5 | `UP_TO_DATE`, but few rules became checks | Each rule's outcome and reason in the report | Group them in plain words ("3 rules mention tools this agent doesn't have"); offer the contract skill to make them more specific |
| 5 | Still in progress after 15 minutes | The status and `latestJob.state` | Say so; offer to keep checking or to come back later with "continue setting up Guardian" |
| 6 | No rules file yet | The compilation status | Back to step 5 |
| 6 | The download fails | The link is short-lived | Get a fresh one with `get_agent_rules_file`, once |
| 6 | The hash doesn't match, twice | none | Don't use the file; the support details |
| 6 | No `spec.yaml` in the file | Its `compiledAt` | An old rules file: apply the rules again |
| 6 | The agent doesn't start: `ConfigurationError` naming `policy_bundle` | Does the named path exist where the agent runs? | The file wasn't shipped with the setting: ship them together, or remove the setting |
| 6 | The agent doesn't start: `BundleError` | The file's hash | A truncated or wrong file: download it again |
| 7 | Collection `NOT_COLLECTED`, reason `NO_CONNECTION` | none | The administrator connects Collibra to the Databricks workspace; "continue" |
| 7 | Reason `SOURCE_NOT_CONNECTED` | The agent's workspace, from its external id | Collibra is connected to a different workspace: a message for the administrator naming the workspace id |
| 7 | Reason `AGENT_NOT_FOUND` | The agent id's form against how it's deployed (App or serving endpoint) | A checklist: wrong id, not deployed yet, no MLflow experiment, or two endpoints serving the same model |
| 7 | Collection `FAILING` | `lastCollectedOn` | Collecting itself fails: the support details |
| 7 | Enforcement `NOT_OBSERVED` or `STALE` after the deploy | `observedBundle` against `latestBundle` | The agent isn't running the new rules file yet: restart it with the file |
| any | `agentContractsFeatureDisabled`, or Guardian looks switched off | Which switch | Not a permission problem: "Collibra settings" below; no retry until "continue" |
| any | A block missing from `get_agent_guardian_status` | The tool's note on absent blocks | An older Collibra: say which part couldn't be checked, never guess it |

## Collibra settings

Two switches and one option on **Settings > AI Command Center > Guardian and contract** (`<instance>/settings/ai-governance/guardian`). Only a Collibra administrator can change them.

| Setting | What it means for the customer | Needed for |
|---|---|---|
| **AI Agent contracts** | Lets your company write agent contracts in Collibra. | Writing or changing agent contracts |
| **Guardian** | Lets Guardian check your agents against their agent contracts and record issues in Collibra. Turning it on also turns on AI Agent contracts. Turning it off only hides Guardian in Collibra: agents that have the SDK keep being checked until it's removed from their code. | Applying the rules and seeing issues |
| **Allow collecting payloads to enable showing details of the violations** (under Guardian) | Lets Collibra keep what was said in a flagged conversation, so you can see why something was flagged. Your company decides this for privacy reasons. | Seeing the conversation behind an issue |

**Detect.** The contract tools return `agentContractsFeatureDisabled` when AI Agent contracts is off. The Guardian tools have no "switched off" error, so when a Guardian tool fails with an error that is not a permission problem (401, 403) and not a missing agent (404), say Guardian may be turned off and give the guidance below, rather than treating it as a permission problem.

**Only the switch a chosen path needs blocks it:** writing or changing agent contracts needs AI Agent contracts; setting up an agent needs both.

**Guide.** Say what's off and what it blocks, in one sentence: "`<AI Agent contracts | Guardian>` is turned off in your Collibra, so `<what it blocks, in plain words>`. A Collibra administrator turns it on in Settings > AI Command Center > Guardian and contract: `<settings link>`." Then (header `Admin`): "Are you a Collibra administrator?" **Yes, show me how (default)** / **No, write a message for my administrator**.

- Yes: "1. Open `<settings link>`. 2. Turn on `<AI Agent contracts | Guardian>`. 3. Come back here and type "continue". I'll check it worked."
- No: "Hi, I'm setting up Collibra Guardian for our AI agent `<agent>`. Could you turn on `<AI Agent contracts | Guardian>` in Collibra: Settings > AI Command Center > Guardian and contract (`<settings link>`)? `<If Guardian: Please also decide whether to allow collecting payloads, which shows the conversation behind each issue.>` Thank you." Then: "When it's on, type "continue"."

Never retry the blocked action before the customer says "continue"; then check again once.

**Payload collection** is the company's privacy choice: never push for it. Mention it only when the customer looks at an issue without detail, or asks why: "Your company has chosen not to keep conversation details, so I can show that a rule was broken but not what was said. An administrator can change this in the same settings page."

## Permissions

A 401 or 403 is not transient: never retry it on your own (`refusal-scripts.md › R-PERMISSION`). "Your Collibra account doesn't have permission to `<action>`. Here's a message for your Collibra administrator: `<message>`" The message names the role that covers the action, from the setting-up page's roles table: AI Producer for agents, AI Business User for writing agent contracts, AI Governance Admin for settings.

## Retries

- A read, a time-out or a server error: try once more, quietly. If it works, say nothing. After a second failure: "Collibra didn't respond, so I've stopped here. Nothing was changed." Then the support details if the customer wants them.
- A write that failed: retry only when you can confirm it didn't happen (for example, the status shows the rules didn't start applying). Otherwise read back first and report what is there.
- Never after 401 or 403.

## When the number of checks moves

When the contracts didn't change but the number of rules that became checks did: "This time `<n>` of your `<total>` rules became checks, compared with `<previous>` last time. Collibra reads plain-language rules a little differently each time, so the number can move slightly. The rules file in use is always the newest one." Say it only in that case.
