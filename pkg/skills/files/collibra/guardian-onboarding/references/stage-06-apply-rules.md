# Stage 06: Apply the rules (step 5 of 8)

Turn the agent contracts that govern the agent into a rules file for its newest version, and summarise the result in plain words. It is the same action as **Apply latest rules** on the agent's **Contract** tab; the customer never has to look for it.

**Tools:** `apply_latest_rules`, `get_agent_guardian_status`, `get_agent_rules_report`.

**Before:** "Now I'll apply your agent contracts to `<agent>`. Collibra turns each rule into checks Guardian can run. This takes about five minutes. It's the same as **Apply latest rules** on the agent's **Contract** tab."

Run this step again after a contract change, after a new agent version, and after linking a use case.

## Step 1: Preview and ask

Call `apply_latest_rules` with the AI Agent id and `confirm` false. It changes nothing and returns the agent, its newest version, the contracts that govern it with their active versions, and the current status. Show that in plain words (agent, version, each contract with its rule count), then ask, word for word:

"Shall I apply the latest rules to `<agent>` now? I'll change nothing until you say yes."

## Step 2: Apply

After the customer's clear yes for what was shown, call `apply_latest_rules` once with `confirm` true. Say: "Collibra has started. While it works, I'll get the next step ready."

- **403:** no permission. A message for the administrator (`references/verify-and-diagnose.md`); never retried, even after "try again".
- **404 agent not found:** Collibra may not have finished adding the agent. Check the fingerprint status, wait, and ask again before the next try.
- **A time-out or server error:** read `get_agent_guardian_status` first. Call again only if it shows the rules didn't start applying (the latest run is older than your call), and only once.

The result has a state and a time, never an id. Don't mention a job or any id to the customer.

## Step 3: Wait

Check `get_agent_guardian_status` about once a minute until `compilation.status` is `UP_TO_DATE`, `NOT_COMPILED` or `FAILED`. The state of the latest run alone isn't enough: a run with no usable rules also ends as completed. Typical time: about five minutes for 20 rules.

Between checks, if you can run commands, wait in the shell (`sleep 60`) rather than calling again straight away. While waiting, prepare the next step (read the project's settings file, work out where the rules file goes) and give at most one short update a minute. After 15 minutes still in progress, follow `references/verify-and-diagnose.md`.

## Step 4: Check

- `compilation.status` is `UP_TO_DATE`.
- `get_agent_rules_report`: its rule count equals the sum of `ruleCount` over the governing contracts, and its `agentVersionId` is the agent's newest version.
- If either check fails, say what you found, never success.

`FAILED` or `NOT_COMPILED`: follow `references/verify-and-diagnose.md` (in order: no contract governs the agent, then contracts with no rules; a failure message that says to try again means asking once more whether to apply the rules).

## Step 5: The result, in plain words

Group the report's outcomes with the contract skill's marks:

| Outcomes | Mark | Say |
|---|---|---|
| `bound` | ✅ | checked while the agent runs |
| `partially_bound` | 🟡 | partly checked while the agent runs |
| `no_enforcement_required` | 📋 | policy only: recorded, not checked while the agent runs |
| `not_applicable` | ➖ | not about this agent |
| `unbindable`, `ambiguous`, `inexpressible`, `unsupported_pattern`, `needs_classifier`, `unbound`, `no_model_answer` | ⚠️ | couldn't be turned into a check as written |

Give the counts per mark, and at most one plain line per ⚠️ group, from the rules' `reason` ("3 rules mention tools this agent doesn't have"). Never paste the report: show rule by rule only when the customer asks. Never name the outcome codes to the customer, and never explain them beyond the reason.

"Step 5 of 8 done: `<n>` of your `<total>` rules will be checked while your agent runs. `<m>` are policy only, and `<k>` don't apply to this agent. Details on the agent's [Contract tab](<instance>/asset/<aiAgentId>?tabbar=Contract)."

If some rules need a change: offer the contract skill to make them more specific (Stage 04, step 3). If none became checks: "None of your rules could be turned into checks for `<agent>`: `<plain reason>`. Would you like help making them more specific?"

If the contracts didn't change but the number of checks did since last time, use the wording in "When the number of checks moves" in `references/verify-and-diagnose.md`.

## Exit

Write the rules file id (`compilation.latestBundle.id`) to the state file. Exit when a rules file exists, or the customer knows why not and what to do.
