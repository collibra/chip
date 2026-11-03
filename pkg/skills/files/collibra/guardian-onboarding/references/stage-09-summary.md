# Stage 09: Summary (step 8 of 8)

Say "Guardian is set up for `<agent>`." when it is, or what is still missing when it isn't, then the summary. Read everything again from Collibra first (`get_agent_guardian_status`); show only what a tool returned in this conversation.

Every item has a link. Leave out an item you don't have; never invent one.

```text
Agent:           <name>, <link>
Version:         <name>, <link>
Use case:        <name>, <link> | none
Agent contracts: <name> (<n> rules), <link>, one line each
Rules:           <n> of <total> checked while the agent runs; details on the Contract tab: <instance>/asset/<aiAgentId>?tabbar=Contract
Rules file:      <path in the project>, checked
Mode:            Monitor (record only) | Enforce (stop it), from enforcement_mode in <settings file>
Activity:        <read by Collibra | waiting for the connection | waiting for the deploy>
Issues:          the Guardian tab: <instance>/asset/<aiAgentId>?tabbar=Guardian
All agents:      <instance>/ai-governance/guardian-agents
Endpoint:        <the App or the serving endpoint in Databricks, linked>, where you can check the deployed agent
```

The endpoint line appears only when the workspace address and the name are both known from the code, the scan or a read-only look-up; never guess one. A Databricks App: `https://<workspace host>/apps/<app name>`. Model Serving: `https://<workspace host>/ml/endpoints/<endpoint name>`.

Links: any asset (AI Agent, AI Agent Version, AI Use Case, AI Endpoint) is `<instance>/asset/<id>`; a contract is `<instance>/ai-governance/agent-contracts/<contractId>`.

If the rules file isn't in place yet, or the agent hasn't been deployed with it: "Until your agent starts with the rules file, Guardian lets everything through."

Then: "Come back to this conversation, or say "update Guardian for my agent", when your agent's code or its agent contracts change, or when you want to change what Guardian does when a rule is broken."

Then "Learn more:" and the page on switching Guardian on for an agent (`references/documentation.md`).

Update the state file with the current stage. The run is complete. Another agent starts again from Stage 01 (one agent per run).
