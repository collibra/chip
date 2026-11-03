# Stage 03: The agent in Collibra (step 2 of 8)

Make sure Collibra knows the agent's current version, and get its AI Agent id and AI Agent Version id.

**Tools:** `list_scanned_agents`, `get_latest_agent_scan`, `get_agent_guardian_status`, `get_asset_details`; the hand-off to `collibra/agent-scanner` with `load_collibra_skill`.

**Before** (by depth):
- New agent: "Next, I'll read your agent's code and describe it to Collibra: its models, instructions and tools. Collibra calls this description a fingerprint."
- Known agent: "Collibra already knows `<agent>` (version `<n>`). I'll check whether your code changed since then."

## Step 1: Decide whether a scan is needed

| Situation | Do |
|---|---|
| Collibra doesn't know the agent | Hand off to the scanner (step 2) |
| Collibra knows it and the code changed since its last scan (a different commit from `get_latest_agent_scan`, or changes in the working copy, including the ones made in step 1) | Hand off to the scanner |
| Collibra knows it, the code is unchanged, and only the deployment changed (for example a redeploy) | Don't send a new scan: a scan with the same code and a new deployment makes a new agent version with no rules. Say so, and go to step 3 |
| The customer has no code | No scan from here. Use the agent Collibra knows; the developer's hand-off note covers the scan. If Collibra doesn't know the agent, steps 3 to 5 can't run for it: offer to work on agent contracts instead |

## Step 2: Hand off to the scanner

Tell the customer: "I'll now describe your agent to Collibra. You'll see exactly what will be sent before anything goes." Load `collibra/agent-scanner` with `load_collibra_skill` and follow it from its own start, with its own rules and questions; it asks for consent in its own way. Point it at the agent's project path from Stage 01. The scan must include the step 1 changes: if they aren't committed, tell the customer, and when the scanner asks about uncommitted changes, recommend including them.

If the scanner reports that only the deployment changed and the code didn't, recommend not sending, for the reason in step 1. The customer still decides at the scanner's own question.

When the scanner ends, reload `references/stage-00-security-and-scope.md` if it is no longer in your context, and come back here. Keep what the scan returned: the external id it was sent with, the scan id and the fingerprint id. If nothing was sent, say so, and continue only if Collibra already knows the agent.

## Step 3: Wait for Collibra

1. Check `list_scanned_agents` with `sourceSystem` and `externalId` from the id the scan was sent with, until the agent has an `aiAgentId`. Between checks, if you can run commands, wait in the shell (`sleep 60`) rather than calling again straight away. "Collibra is still adding your agent. This usually takes under a minute. I'll keep checking."
2. Then check `get_agent_guardian_status` with that id, the same way, until `fingerprint.status` is `COMPLETED` or `FAILED`. `FAILED` carries `failureMessage`: say it plainly (`references/verify-and-diagnose.md`).
3. At most one short update a minute. After five minutes without an agent id, follow the diagnosis in `references/verify-and-diagnose.md`.

## Step 4: Check

- The AI Agent id is known.
- `fingerprint.aiAgentVersion` names the newest version, and its `fingerprint.id` equals the fingerprint id the scan returned. If no scan was sent, the newest version is the one Collibra holds.
- Two versions in a row with the same code (a redeploy): explain it as in `references/verify-and-diagnose.md`.

## Step 5: Links

- The agent: `<instance>/asset/<aiAgentId>`
- Its version: `<instance>/asset/<aiAgentVersionId>`
- When the scan had a deployment: its AI Endpoint, read from the version's relations with `get_asset_details` on the version id: `<instance>/asset/<endpoint id>`.

## Exit

Write the agent id and version id to the state file. Then:

- "Step 2 of 8 done: Collibra knows `<agent>`, version `<n>`. Links: [`<agent>`](<instance>/asset/<aiAgentId>), [version `<n>`](<instance>/asset/<aiAgentVersionId>)`<, and where it runs: [<endpoint name>](<instance>/asset/<endpoint id>)>`."
- Unchanged: "Step 2 of 8 done: your code hasn't changed, so Collibra already has the latest version. Links: [`<agent>`](<instance>/asset/<aiAgentId>), [version `<n>`](<instance>/asset/<aiAgentVersionId>)."

Exit when the AI Agent id and AI Agent Version id are known and stored in the state file.
