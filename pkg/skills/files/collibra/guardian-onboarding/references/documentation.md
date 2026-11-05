# Product documentation

The private preview documentation is the customer's reference, and this skill follows it. It is still changing, so link to it and never copy its text. Each step ends with "Learn more:" and the page for that step below. When the documentation and these files disagree, the documentation wins, except for the known gaps listed at the end of this file.

**Every documentation link lives in this file.** Stages name a page by its topic ("the SDK settings page"), never by its address. At general availability the addresses change; only this file changes with them.

**If a link doesn't work,** don't show a broken link. Give the customer the topic's name and the "About Guardian" page (the first row below) as the place to start.

## Pages

Base address (private preview): `https://productresources.collibra.com/docs/collibra/private-preview/AICommandCenter/Content/AICommandCenter/GuardianAgents/`

| Topic | Used at | Link |
|---|---|---|
| About Guardian | Start; "what is Guardian?" | https://productresources.collibra.com/docs/collibra/private-preview/AICommandCenter/Content/AICommandCenter/GuardianAgents/co_about-guardian-agents.htm |
| Setting up Guardian: Collibra settings and roles | The Collibra settings; a missing permission; the activity connection | https://productresources.collibra.com/docs/collibra/private-preview/AICommandCenter/Content/AICommandCenter/GuardianAgents/ta_aicc-set-up-guardian-private-preview.htm |
| Agent contracts | Step 3 | https://productresources.collibra.com/docs/collibra/private-preview/AICommandCenter/Content/AICommandCenter/GuardianAgents/co_agent-contracts.htm |
| How Guardian works, and the modes Enforce and Monitor | Step 1 (the mode); step 8; the verdicts | https://productresources.collibra.com/docs/collibra/private-preview/AICommandCenter/Content/AICommandCenter/GuardianAgents/co_guardian-agent.htm |
| Switching Guardian on for an agent | Steps 1 and 8 | https://productresources.collibra.com/docs/collibra/private-preview/AICommandCenter/Content/AICommandCenter/GuardianAgents/co_guardian-agent.htm#enabling |
| SDK settings | Steps 1 and 6 | https://productresources.collibra.com/docs/collibra/private-preview/AICommandCenter/Content/AICommandCenter/GuardianAgents/ref_guardian-sdk-settings.htm |
| Supported frameworks and tested versions | Step 1 (the framework version check) | https://productresources.collibra.com/docs/collibra/private-preview/AICommandCenter/Content/AICommandCenter/GuardianAgents/ref_guardian-sdk-settings.htm#frameworks |
| Where the SDK finds the rules file | Step 6 | https://productresources.collibra.com/docs/collibra/private-preview/AICommandCenter/Content/AICommandCenter/GuardianAgents/ref_guardian-sdk-settings.htm#bundle |
| Running the agent with Guardian without code changes | Step 1, only when the customer asks for the launcher | https://productresources.collibra.com/docs/collibra/private-preview/AICommandCenter/Content/AICommandCenter/GuardianAgents/ta_guardian-cli-run.htm |
| Downloading the latest rules file with the Collibra CLI | Step 6, for an agent that runs where the customer has the Collibra CLI | https://productresources.collibra.com/docs/collibra/private-preview/AICommandCenter/Content/AICommandCenter/GuardianAgents/ta_guardian-cli-sync.htm |
| Collibra CLI error codes | When the customer reports an error from the Collibra CLI | https://productresources.collibra.com/docs/collibra/private-preview/AICommandCenter/Content/AICommandCenter/GuardianAgents/ref_guardian-cli-errors.htm |
| Installing the Collibra CLI | Step 6, with the CLI mention | https://productresources.collibra.com/docs/collibra/private-preview/AICommandCenter/Content/CLI/ta_set-up-cli.htm |

Checked against the documentation on 2026-11-03.

## Known gaps in the documentation

These are not covered, or are covered differently, in the documentation today. Keep this skill's own steps for them, and don't tell the customer the documentation is wrong:

- Scanning the agent isn't documented; the documentation says only that Guardian must recognise the agent.
- The "Allow collecting payloads" option isn't described.
- Connecting Collibra to the agent's activity in Databricks isn't documented. Use the setting-up page as the nearest topic.
- Databricks Apps and Model Serving aren't covered; the documentation points managed platforms to the install-once way.
- No permission is named for Apply latest rules.
- The documentation says applying the rules needs an AI use case linked to the agent; it doesn't. The use case stays optional.
- The rules file is documented only through the Collibra CLI's download command; this skill gets it from Collibra directly.
