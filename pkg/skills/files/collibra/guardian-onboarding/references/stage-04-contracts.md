# Stage 04: Agent contracts (step 3 of 8)

Make sure at least one agent contract governs the agent: the company-wide (global) one first, then the agent's own.

**Tools:** `list_agent_contracts` (with `agentId`, or with `name`), `get_agent_contract`, `get_agent_contract_rules`, `edit_asset` (operation `add_relation`); the hand-off to `collibra/agent-contract` with `load_collibra_skill`.

**Before** (by depth; see Stage 00, "Depth of explanation"):
- New: "Step 3 of 8: your agent contract. An agent contract is a set of rules for your agents, written in plain language, such as "never share a customer's card number". One company-wide contract can apply to every agent, and an agent can also have its own. Guardian checks your agent against these rules, so this is where you decide what it may and may not do."
- Knows Collibra: the same, the first time "agent contract" comes up; otherwise "Step 3 of 8: your agent contract."
- Uses Guardian: nothing before the step.

**Before:** "An agent contract is a set of rules for your agents, written in plain language. One company-wide contract can apply to every agent, and an agent can also have its own."

## Step 1: What governs the agent now

1. `list_agent_contracts` with `agentId` set to the AI Agent id. The global contract, if there is one, comes first; then the specialised contracts linked to this agent. Note each contract's `ruleCount` and `activeVersionId`.
2. If a contract can't be read (a permission error on one of them), say the list may be incomplete and offer a message to the administrator about view rights.
3. `agentContractsFeatureDisabled`: "Collibra settings" in `references/verify-and-diagnose.md`.

If at least one contract governs the agent and has rules, show them (name, rule count, link) and go to the exit, unless the customer wants to write or change one.

## Step 2: None applies: look for one that could fit

Search for existing specialised contracts that aren't linked to this agent: `list_agent_contracts` with `name` set to words from the scan (the agent's name, the purpose in its instructions, the subject of its tools), a few searches at most. For each candidate, read one rule with `get_agent_contract_rules` and how many agents it governs (`governedAgentCount`).

Show each match first, before any question: "I found an agent contract that may fit: **`<name>`**, `<n>` rules, for example `<one rule>`. It applies to `<n>` agents. `<link>`"

Then (header `Contracts`): "No agent contract applies to `<agent>` yet. What would you like to do?"

- **Use `<name>`** (first, only when a match was shown)
- **Write a contract for this agent (default when nothing was found)**
- **Write the company-wide contract**
- **Stop here for now**

The default is the first option. "Use" goes to step 4 (linking). "Write" goes to step 3.

## Step 3: Hand off to the contract skill

Tell the customer what step 3 involves before the hand-off: "Step 3 of 8: your agent contract. I'll draft rules for `<agent>` from what it does, ask you for any values I can't know (such as limits), go through the rules with you, and show you the whole contract before anything is saved. After it's saved, I'll link it to `<agent>` and we go on to step 4, what your agent is for." Load `collibra/agent-contract` with `load_collibra_skill` and follow it from its own start, with its own rules; it asks for consent in its own way. Tell it what you already know: global or specialised, and the agent to check the rules against. It works on one contract at a time, and may offer to link a new specialised contract to the agent itself.

While the contract skill runs, end every message that waits for the customer with one plain line saying where they are and what happens after they answer, for example "Step 3 of 8, your agent contract. After your answers, I'll go through the rules with you, then show the whole contract to save." Keep it to one line, and don't repeat the contract skill's own summary.

When it ends, reload `references/stage-00-security-and-scope.md` if it is no longer in your context, and come back here: read the list again (step 1). If a new specialised contract doesn't govern the agent yet, go to step 4.

## Step 4: Link a specialised contract

A specialised contract that doesn't govern the agent yet (one just written, or an existing one the customer chose). You link it yourself, after the customer's yes. Ask (header `Link`): "Your contract `<name>` isn't linked to `<agent>` yet, so its rules don't apply to it. Shall I link it to `<agent>` now?" Options: **Yes, link it** / **I'll do it myself in Collibra** / **Not now**, none of them labelled the default.

- **Yes:** `edit_asset` on the **AI Agent** with one `add_relation` operation, `relationType` "complies to", and the contract's id as `targetAssetId`. On an AI Agent that name belongs to one relation type only (an asset complies to a governance asset, which an agent contract is), so it links the right way. Then check it (step 5).
- **The customer prefers to do it in Collibra, or the link fails** (for example no permission: never retried, `refusal-scripts.md › R-PERMISSION`): "Open [`<name>`](<instance>/ai-governance/agent-contracts/<contractId>), link it to `<agent>`, then type "continue"." Wait, then check it (step 5).
- **Not now:** go to the exit with what governs the agent.

Never unlink a contract, and never link one the customer hasn't seen.

## Step 5: Check

After a hand-off or a link, read `list_agent_contracts` with `agentId` again and confirm the contract now governs the agent, with the rule count you expect (`get_agent_contract` for its details). If it doesn't: "I `<linked it | saw your change>`, but when I checked, `<what I found>`." Never report success without this check.

If rules were already applied to this agent, a new or changed contract makes them out of date: step 5 applies them again.

## Exit

"Step 3 of 8 done: `<n>` agent contracts apply to `<agent>`: [`<name>`](<instance>/ai-governance/agent-contracts/<contractId>) (`<rules>` rules), one per contract`<. The new link shows on the agent's [Contract tab](<instance>/asset/<aiAgentId>?tabbar=Contract)>`." Then "Learn more:" and the agent contracts page (`references/documentation.md`). Write the contract ids to the state file.

Links: a contract is `<instance>/ai-governance/agent-contracts/<contractId>`; all contracts are `<instance>/ai-governance/agent-contracts`.

Exit when at least one agent contract governs the agent, or the customer stops.
