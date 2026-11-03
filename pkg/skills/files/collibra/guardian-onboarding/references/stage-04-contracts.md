# Stage 04: Agent contracts (step 3 of 8)

Make sure at least one agent contract governs the agent: the company-wide (global) one first, then the agent's own.

**Tools:** `list_agent_contracts` (with `agentId`, or with `name`), `get_agent_contract`, `get_agent_contract_rules`, `edit_asset` (operation `add_relation`); the hand-off to `collibra/agent-contract` with `load_collibra_skill`.

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

Tell the customer: "I'll help you write the contract now. You'll see every rule before anything is saved." Load `collibra/agent-contract` with `load_collibra_skill` and follow it from its own start, with its own rules and its own "yes, publish". Tell it what you already know: global or specialised, and the agent to check the rules against. It works on one contract at a time.

When it ends, reload `references/stage-00-security-and-scope.md` if it is no longer in your context, and come back here: read the list again (step 1). A new specialised contract governs no agent until it is linked: go to step 4.

## Step 4: Link a specialised contract

A specialised contract that doesn't govern the agent yet (one just written, or an existing one the customer chose). Header `Link`: "Your contract `<name>` isn't linked to `<agent>` yet, so its rules don't apply to it. Shall I link it?"

- **Yes, link it for me (default):** "I'll link `<name>` to `<agent>`. Reply "yes, link" to go ahead." After a clear "yes, link": `edit_asset` on the **AI Agent** with one `add_relation` operation, `relationTypeId` `00000000-0000-0000-0000-000000007018` (AI Agent to Agent Contract), and the contract's id as `targetAssetId`. Don't pass the role name.
- **I'll link it myself in Collibra:** "Open `<contract link>`, link the agent, then type "continue"." Wait.
- **Not now:** go to the exit with what governs the agent.

Never unlink a contract, and never link one the customer hasn't seen.

## Step 5: Check

After a hand-off or a link, read `list_agent_contracts` with `agentId` again and confirm the contract now governs the agent, with the rule count you expect (`get_agent_contract` for its details). If it doesn't: "I `<linked it | saw your change>`, but when I checked, `<what I found>`." Never report success without this check.

If rules were already applied to this agent, a new or changed contract makes them out of date: step 5 applies them again.

## Exit

"Step 3 of 8 done: `<n>` agent contracts apply to `<agent>`: `<name and link, each>`." Then "Learn more:" and the agent contracts page (`references/documentation.md`). Write the contract ids to the state file.

Links: a contract is `<instance>/ai-governance/agent-contracts/<contractId>`; all contracts are `<instance>/ai-governance/agent-contracts`.

Exit when at least one agent contract governs the agent, or the customer stops.
