# Stage 08: Guardian sees the agent's activity (step 7 of 8)

Check that Collibra can read the agent's activity, and, after the deploy, that the running agent uses the latest rules file. This step is last on purpose: a missing connection never blocks the rest.

**Tools:** `get_agent_guardian_status`.

**Before** (by depth):
- New: "Step 7 of 8: let Guardian see your agent's activity. Guardian records what your agent does in Collibra, so you can see any rules it broke on the agent's **Guardian** tab. For that, Collibra needs to read your agent's activity in Databricks. A Collibra administrator connects the two once; I'll check whether that's done."
- Knows Collibra: the same, the first time; otherwise "Step 7 of 8: check that Collibra reads your agent's activity."
- Uses Guardian: nothing before the step.

**Before:** "Last, Guardian needs to read your agent's activity in Databricks. A Collibra administrator connects Collibra to your Databricks workspace once."

## Step 1: Read the connection status

`get_agent_guardian_status` with the AI Agent id. Read `collection.status`, `collection.reason` and `collection.lastCollectedOn`. If the block is missing (an older Collibra), say this part couldn't be checked.

| Collection | What it means | Do |
|---|---|---|
| `OK` | Collibra reads the agent's activity | Step 2 |
| `NOT_COLLECTED`, reason `NO_CONNECTION` | Collibra isn't connected to the Databricks workspace yet | The administrator message below; pause for "continue" |
| reason `SOURCE_NOT_CONNECTED` | Collibra is connected to a different workspace | The administrator message, naming the agent's workspace id (from its external id) |
| reason `AGENT_NOT_FOUND` | The agent wasn't found in the connected workspace | The checklist: is the agent's id right for how it's deployed (App or serving endpoint); is it deployed yet; does it have an MLflow experiment; do two endpoints serve the same model? |
| `LAGGING` | Collibra reads it, with a delay | Say so; nothing to do |
| `FAILING` | Reading the activity fails | `references/verify-and-diagnose.md`, then the support details |

The administrator message: "Collibra isn't connected to your Databricks workspace yet. Here's a message you can send to your Collibra administrator:" then, in a code block: "Hi, I'm setting up Collibra Guardian for our AI agent `<agent>` (`<agent link>`). Could you connect Collibra to our Databricks workspace `<workspace id>` so Guardian can read the agent's activity?" Add "Learn more:" and the setting-up page (`references/documentation.md`). Then: "When the connection is in place, type "continue"." On "continue", read the status again once.

## Step 2: After the deploy

The running agent reports which rules file it uses only after it has started with the file and handled a message, and Collibra has read its activity. Say: "Once your agent is deployed and has handled a message, I can check that it uses the new rules, once Collibra has read its activity."

When the customer comes back, or says "continue", read `get_agent_guardian_status` again:

- `enforcement.status` `UP_TO_DATE` and `observedBundle` matches `compilation.latestBundle`: the agent runs the latest rules file.
- `NOT_OBSERVED` or `STALE`: the agent isn't running the new rules file yet: restart it with the file (step 6's deploy steps).

The install-once way and the default way both fail silently when misplaced, so this check is the only proof Guardian is on. Never say Guardian is checking the agent before it passes.

## Exit

Never promise how long Collibra takes to read the activity: it changes. Say only what the status reports (`collection.lastCollectedOn`).

"Step 7 of 8 done: Guardian is reading your agent's activity. Collibra last read it at `<lastCollectedOn, in the customer's time>`. New issues appear on the agent's [Guardian tab](<instance>/asset/<aiAgentId>?tabbar=Guardian)."

Exit when activity is being read, or the customer has the administrator's message and knows when to come back.
