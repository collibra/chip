# Stage 08: Guardian sees the agent's activity (step 7 of 8)

Check that Collibra can read the agent's activity, and, after the deploy, that the running agent uses the latest rules file. This step is last on purpose: a missing connection never blocks the rest.

**Tools:** `get_agent_guardian_status`; for the optional step 3, `get_agent_rules_report` and a read-only look where the agent's records go.

**Before** (by depth):
- New: "Step 7 of 8: let Guardian see your agent's activity. Guardian records what your agent does in Collibra, so you can see any rules it broke on the agent's **Guardian** tab. For that, Collibra needs to read your agent's activity in Databricks. A Collibra administrator adds one Databricks connection on Collibra Edge, once; I'll check whether that's done."
- Knows Collibra: the same, the first time; otherwise "Step 7 of 8: check that Collibra reads your agent's activity."
- Uses Guardian: nothing before the step.

**Before:** "Last, Guardian needs to read your agent's activity in Databricks. A Collibra administrator adds one Databricks connection on a running Collibra Edge site, once. Guardian sets up the rest itself."

## Step 1: Read the connection status

`get_agent_guardian_status` with the AI Agent id. Read `collection.status`, `collection.reason` and `collection.lastCollectedOn`. If the block is missing (an older Collibra), say this part couldn't be checked.

| Collection | What it means | Do |
|---|---|---|
| `OK` | Collibra reads the agent's activity | Step 2 |
| `NOT_COLLECTED`, no reason | Collibra hasn't checked for this agent's activity yet, as just after the rules are first applied | "Collibra hasn't checked yet. It checks about every 15 minutes." Offer to check again later |
| `NOT_COLLECTED`, reason `NO_CONNECTION` | Collibra Edge has no Databricks connection yet | The administrator message below; pause for "continue" |
| reason `SOURCE_NOT_CONNECTED` | Collibra has a Databricks connection, but hasn't read this workspace through it yet | In the half hour after the administrator added the connection, this is expected: Collibra first sees the connection at a check, then reads the workspace by the check after. Say so, and don't send a message. If it stays: a message for the administrator to check that the connection is for workspace `<workspace id>` (from the agent's external id) and that its Edge site is running |
| reason `AGENT_NOT_FOUND` | The agent wasn't found in the connected workspace | The checklist: is the agent's id right for how it's deployed (App or serving endpoint); is it deployed yet; does it have an MLflow experiment; do two endpoints serve the same model? |
| `LAGGING` | Collibra reads it, with a delay | Say so; nothing to do |
| `FAILING` | Reading the activity fails | `references/verify-and-diagnose.md`, then the support details |

The administrator message: "Collibra isn't connected to your Databricks workspace yet. Here's a message you can send to your Collibra administrator:" then, in a code block: "Hi, I'm setting up Collibra Guardian for our AI agent `<agent>` (`<agent link>`). Could you add a Databricks connection for our workspace `<workspace id>` on a running Collibra Edge site, so Guardian can read the agent's activity? Only the connection is needed: Guardian sets up the rest on that Edge site itself." Never ask the administrator to set up anything else on Edge, because Guardian adds what reads the activity for each Databricks connection it finds. Add "Learn more:" and the setting-up page (`references/documentation.md`). Then: "When the connection is in place, type "continue"." On "continue", read the status again once. Collibra looks for new connections about every 15 minutes, so if it still says `NO_CONNECTION` or now says `SOURCE_NOT_CONNECTED`, don't send a message: "Collibra checks for new connections regularly, about every 15 minutes, so it can take up to half an hour after the connection is added before it reads your agent's activity. I can check again later, or you can come back with "continue setting up Guardian"."

## Step 2: After the deploy

The running agent reports which rules file it uses only after it has started with the file and handled a message, and Collibra has read its activity. Say: "Once your agent is deployed and has handled a message, I can check that it uses the new rules, once Collibra has read its activity."

When the customer comes back, or says "continue", read `get_agent_guardian_status` again:

- `enforcement.status` `UP_TO_DATE` and `observedBundle` matches `compilation.latestBundle`: the agent runs the latest rules file.
- `NOT_OBSERVED` or `STALE`: the agent isn't running the new rules file yet: restart it with the file (step 6's deploy steps).

The install-once way and the default way both fail silently when misplaced, so this check is the only proof Guardian is on. Never say Guardian is checking the agent before it passes.

## Step 3 (optional): What Guardian did so far

Offer this on any platform when the agent has handled messages since the deploy, the rules file is in the project, and you can read where the agent's Guardian decisions go with the customer's own sign-in. It's useful when Collibra hasn't read the activity yet, when Collibra can't read this platform, or when the customer asks whether the rules work as meant. Collibra's Guardian tab stays the record; this is a quick look.

The Guardian SDK records each decision in the places the agent's settings send it. Find which from the project (its SDK settings, `MLFLOW_TRACKING_URI`, an OpenTelemetry set-up, how it logs), or ask once:

| Where the decisions go | How to read them, read-only |
|---|---|
| MLflow traces, on Databricks or any MLflow server | The project's Python with `mlflow` and the agent's tracking address and experiment: `mlflow.search_traces` for the latest runs. On Databricks, the signed-in `databricks` tool's profile (`shared/platform-prerequisites.md`) |
| OpenTelemetry, to an observability service | That service's own command-line tool or API, only when the customer has one signed in and names it |
| The agent's logs (the SDK's `collibra.guardian.decisions` records) | Where the logs are: the local process output, or the platform's log command, such as `databricks apps logs <app>` |

If none of these can be read, don't offer it: point to the Guardian tab.

Ask: "Shall I look at your agent's recent runs in `<where>` to see what Guardian stopped or flagged? I'll read only Guardian's decisions, never the conversations, and change nothing."

After a yes, in one read-only command run from the project with the project's Python:

- Read at most the latest 20 runs.
- Keep only Guardian's decision: whether it stopped or flagged, where (a tool call or a reply), and the id of the check that decided. A decision whose `collibra.guardian.purpose` is `summarization` is about the step that summarises a long conversation, so name that step as where. Count per check and decision.
- **Print only the counts.** Never print, save or quote a run's inputs, outputs or messages: they hold real conversations. Never write a run or a log to a file, not even a temporary one.

Match each check id to its rule with the project's rules file (`spec.yaml`) and `get_agent_rules_report` (the rule's statement and contract). Then show one table:

| Stopped or flagged | Times | Rule | Looks right? |
|---|---|---|---|
| `<what was stopped, in plain words>` | `<n>` | `<contract name>`, rule `<n>` | ✅, or ⚠️ with one plain reason |

"Looks right?" is your own reading, so say so in one line: "This column is my reading of each rule, not a Collibra result." Mark ⚠️ only when the check stops more than the rule means: for example a required phrase that almost no normal answer contains, or one common word such as "key" in a word list. Never show check conditions or patterns; describe them in plain words.

For a ⚠️ on the customer's own contract: offer the `collibra/agent-contract` skill to make the rule more specific, then apply the rules again (step 5). For a ⚠️ on a contract that governs other agents too (a company-wide one): say that changing it affects every agent it governs, so it's a decision for its owner, and offer a note for them. Never change a contract from this step.


## Exit

Never promise how long Collibra takes to read the activity: it changes. Say only what the status reports (`collection.lastCollectedOn`).

"Step 7 of 8 done: Guardian is reading your agent's activity. Collibra last read it at `<lastCollectedOn, in the customer's time>`. New issues appear on the agent's [Guardian tab](<instance>/asset/<aiAgentId>?tabbar=Guardian)."

Exit when activity is being read, or the customer has the administrator's message and knows when to come back.
