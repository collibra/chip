# Verification and diagnosis

## The skill checks its own work

After every action, the skill verifies the result itself before it tells the customer the action worked. The customer
never has to open Collibra to find out whether something happened. This extends the contract skill's rule "after every
write, read the contract back and report what Collibra holds" to every stage (hard rule H6).

| After | The skill checks | With |
|---|---|---|
| Platform login | The identity call answers, and it's the profile the customer chose | the platform tool |
| SDK install and project changes | The packages import in the project's environment; the settings file parses and doesn't name a rules file yet; the entry module still parses (compiled in memory, never run); the diff on disk is exactly the diff shown | local commands |
| Sending the scan | The agent appears with an id, its newest version is the one just sent, and the fingerprint status is `COMPLETED` | `list_scanned_agents`, `get_agent_guardian_status` |
| A contract saved by the contract skill | It's in the list of contracts governing the agent, with the expected rule count | `list_agent_contracts`, `get_agent_contract` |
| Creating or linking a use case | The agent's incoming relations include the use case, through "AI Use Case uses AI Agent" | `get_asset_details` |
| Applying the rules | Compilation `UP_TO_DATE`; the report's rule count equals the governing contracts' rules; the report names the newest agent version | `get_agent_guardian_status`, `get_agent_rules_report` |
| Saving the rules file | SHA-256 equals the reported hash; the archive holds `spec.yaml` whose `spec_bundle_id` is the file's id; the SDK's own loader accepts the file; the file is at the path the settings name | local commands, `get_agent_rules_file` |
| Deploying, after the agent ran | Enforcement shows the latest rules file (`observedBundle` equals `latestBundle`); collection `OK` | `get_agent_guardian_status` |

When a check fails, the skill says what it did, what it found and what happens next, and never reports success:
"I applied the rules, but when I checked, Collibra still shows them as in progress after 15 minutes. ..."

## When something fails: find the cause first

Read every signal the tools expose, name the most likely cause in plain words, and say what to do. Only when the cause
can't be found or fixed from here, show the support details. Every failure message has four parts:

```text
What happened:   <plain words>
What I checked:  <the signals read>
Most likely:     <the cause, and how sure>
What to do:      <the fix, or the hand-off>; if that doesn't help, share the details below with Collibra support.
```

| Stage | The skill sees | It checks next | Most likely cause, and what the customer hears |
|---|---|---|---|
| 02 | The SDK won't install | Python version; the downloaded folder; a missing Collibra dependency wheel | Wrong Python, an incomplete download, or a missing wheel: say which |

| 02, 07 | Guardian says it found no rules file and allows everything | The same path check | Nothing looks broken, so warn about it explicitly |
| 03 | The scan is refused with an error code | The error's message names the field | The description has a mistake: the scanner fixes it and asks again |
| 03 | No agent id after five minutes | `list_scanned_agents` unavailable; fingerprint status | Collibra is busy: try later. Otherwise support details |
| 03 | Fingerprint `FAILED` | Its message | Say it plainly; a domain or permission message goes to the Collibra administrator |
| 04 | No contract governs the agent | Is there a specialised contract the customer just wrote? | None: the contract skill. Not linked: propose linking it (the skill after "yes, link", or the customer on the contract's page) |
| 04 | A contract can't be viewed | Which contracts could be read | The list may be incomplete: a message to the administrator about view rights |
| 06 | 404 agent not found | Fingerprint status | Collibra hasn't finished adding the agent: wait, then retry |
| 06 | 403 | none | No permission: a message for the administrator; never retried |
| any | `agentContractsFeatureDisabled`, or Guardian looks turned off | Which switch: AI Agent contracts or Guardian | Not a permission problem: the "Collibra settings" wording in wording.md; steps for an administrator or a message for one; no retry until "continue" |
| 06 | `FAILED` | The job's failure message | If it says to try again, ask "yes, apply" once more; otherwise support details |
| 06 | `NOT_COMPILED` after the run finished, empty report | In order: no contract governs the agent; the contracts have no rules | Each points to stage 04. With none of these, support details |
| 06 | `UP_TO_DATE`, few rules became checks | Each rule's outcome and reason | Group them in plain words ("3 rules mention tools this agent doesn't have"); offer the contract skill to make them more specific |
| 07 | No rules file yet | Compilation status | Back to stage 06 |
| 07 | The download link fails | It's short-lived | Get a fresh one once |
| 07 | The hash doesn't match, twice | none | Don't use the file; support details |
| 07 | No `spec.yaml` in the file | Its compile date | An old rules file: apply the rules again |
| 07 | The agent doesn't start: `ConfigurationError` naming `policy_bundle` | Does the named path exist on the platform? | The file wasn't shipped with the setting: ship them together, or remove the setting |
| 07 | The agent doesn't start: `BundleError` | The file's hash | A truncated or wrong file: download it again |
| 02 | The agent doesn't start: framework version outside the tested range | The installed LangChain and LangGraph versions | A tested version, or `framework_version_policy = "permissive"`, as the customer chooses |
| 08 | Collection `NO_CONNECTION` | none from chip | The administrator connects Collibra to the Databricks workspace; documentation link; "continue" |
| 08 | `SOURCE_NOT_CONNECTED` | The agent's workspace from its id | Connected to a different workspace: message for the administrator |
| 08 | `AGENT_NOT_FOUND` | The agent id's form against how it's deployed (App or endpoint) | Wrong id, not deployed yet, no MLflow experiment, or two endpoints serving the same model: a checklist |
| 08 | `FAILING` | Last collected time | Collection itself fails: support details |
| 08 | Enforcement `NOT_OBSERVED` after deploy | `observedBundle` against `latestBundle` | The agent isn't running the new rules file yet: restart it with the file |
| 08 | A newer agent version shows no rules, but the agent enforces the older one | The agent's version list: two versions with the same code | Created by a redeploy with unchanged code: see [decisions.md](decisions.md#a-redeploy-can-look-like-a-new-agent-version) |

## Support details

What the customer can share; never internal system names, queries or links:

```text
If you contact Collibra support, share these details:
Instance:       <instance URL>
When (UTC):     <start> to <end>
Step:           <n>, <name>
What happened:  <plain words>
Agent:          <id>
Agent version:  <id>
Scan:           <id>

Rules file:     <id>
Use case:       <id>
Contracts:      <ids>
```

Support teams keep their own guide to finding these ids in their tools; it isn't part of this repository.
