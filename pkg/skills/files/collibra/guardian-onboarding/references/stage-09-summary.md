# Stage 09: Summary (step 8 of 8)

Say "Guardian is set up for `<agent>`." when it is, or what is still missing when it isn't, then the summary. Read everything again from Collibra first (`get_agent_guardian_status`); show only what a tool returned in this conversation.

Every item is a clickable markdown link (`[name](url)`) with a real address. Take the addresses from the tools, never write `<instance>` or build one yourself: the `links` that `get_agent_guardian_status` just returned (agent, agentVersion, each contract, contractTab, guardianTab, guardianAgents), and `get_asset_details`' `link` for the use case (call it with the use case's id). Leave out an item you don't have a link for; never invent one.

Write it as a list, so the links can be clicked (not inside a code block):

- **Agent:** [`<name>`](`links.agent`)
- **Version:** [`<name>`](`links.agentVersion`)
- **Use case:** [`<name>`](`<get_asset_details link>`), or none
- **Agent contracts:** [`<name>`](`links.contracts[].url`) (`<n>` rules), one per contract
- **Rules:** `<n>` of `<total>` checked while the agent runs, [see them on the Contract tab](`links.contractTab`)
- **Rules file:** `<path in the project>`, checked
- **Mode:** Monitor (record only) or Enforce (stop it), from `enforcement_mode` in `<settings file>`
- **Activity:** read by Collibra, waiting for the connection, or waiting for the deploy
- **Issues:** [the Guardian tab](`links.guardianTab`)
- **All agents:** [Guardian agents](`links.guardianAgents`)
- **Endpoint:** [`<the App or the serving endpoint>`](`<platform link>`), where you can check the deployed agent

The endpoint line appears only when the workspace address and the name are both known from the code, the scan or a read-only look-up; never guess one. A Databricks App: `https://<workspace host>/apps/<app name>`. Model Serving: `https://<workspace host>/ml/endpoints/<endpoint name>`.

Never show `<instance>` or any other placeholder to the customer: an item without a real link is written without one.


If the rules file isn't in place yet, or the agent hasn't been deployed with it: "Until your agent starts with the rules file, Guardian lets everything through."

Then: "Come back to this conversation, or say "update Guardian for my agent", when your agent's code or its agent contracts change, or when you want to change what Guardian does when a rule is broken."

Then "Learn more:" and the page on switching Guardian on for an agent (`references/documentation.md`).

Update the state file with the current stage. The run is complete. Another agent starts again from Stage 01 (one agent per run).
