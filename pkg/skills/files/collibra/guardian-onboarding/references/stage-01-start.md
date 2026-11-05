# Stage 01: Start (look, ask once, recommend)

Find out what exists, ask one question about how much explanation the customer wants, and recommend one next step. The customer never has to choose a category. This stage has no step number for the customer.

## Step 1: Opening

Say, word for word:

"I'll help you set up Collibra Guardian for your AI agents. Guardian checks what an agent does against rules you write in plain language, called agent contracts, and records any issues in Collibra. I'll ask before I change anything in Collibra or in your files. Let me check a few things first."

## Step 2: Look, read-only, without asking

- **The Collibra connection** answers and offers the Guardian tools: check that `list_scanned_agents` and `get_agent_guardian_status` are among the tools you were given (don't call them to find out).
- **Collibra settings:** `list_agent_contracts` with no filter. `agentContractsFeatureDisabled` means AI Agent contracts is off. Whether Guardian is on can only be seen once a Guardian tool is called for an agent (`get_agent_guardian_status` below); see "Collibra settings" in `references/verify-and-diagnose.md`.
- **The client can run code and write files:** try a harmless command, such as printing the working directory.
- **Python 3.12 or later** is available (`python3 --version`), because the Guardian SDK needs it. Do **not** check the platform's command-line tool here; that happens only when a step needs it.
- **The current folder holds an AI agent:** framework imports such as LangChain or LangGraph, an `app.yaml`, a `databricks.yml`. See "Where is the agent's code?" below.
- **Collibra already knows that agent:** `list_scanned_agents` with `agentNameContains` set to the agent's name or the folder name (and, when you know the agent's external id, `sourceSystem` and `externalId`). For a known agent, `get_agent_guardian_status` with its id gives the overview: the agent scan, the contracts that govern it, whether its rules are up to date and why not (`staleBecause`), whether the running agent uses the latest rules file, and whether Collibra reads its activity.
- **Which agent contracts exist:** `list_agent_contracts`, the global one in particular (`layer` `GLOBAL`).

A state file, `collibra-guardian/setup-state.json`, in the folder or the project the customer names means an earlier run: go to "Resuming" in the main file.

## Step 3: One question, about explanation depth

Header `Experience`: "How familiar are you with Collibra AI governance?"

- **I'm new to it (default):** "I'll explain each step in a sentence before we do it."
- **I know Collibra, but Guardian is new to me:** "I'll explain Guardian's terms the first time they come up."
- **I already use Guardian:** "I'll keep it short: one line per step."

Keep the answer for the whole conversation. It changes how much each step explains, not which steps run.

Right after the answer, show the difference at once (Stage 00, "Depth of explanation"):
- New: "Here's the way ahead, eight short steps: 1 prepare your agent, 2 add it to Collibra, 3 write its rules (agent contracts), 4 say what it's for (optional), 5 apply the rules, 6 save the rules and deploy, 7 let Guardian see its activity, 8 a summary with links. I'll explain each one as we go."
- Knows Collibra: "Eight steps: prepare the agent, add it to Collibra, agent contracts, use case (optional), apply the rules, save and deploy, activity, summary."
- Uses Guardian: nothing extra; go straight to the recommendation.

## Where is the agent's code?

The customer may start anywhere: their home folder, an unrelated repository, or a client with no project. Never assume the current folder is the agent's project.

- **Check the current folder** (read-only, by listing files and opening only likely entry files and settings, never the whole repository): one agent project, several agents, or none.
- **Several agents** (header `Which agent`): "I found `<n>` AI agents here. Which one should we set up?" One option per agent, with the file it's in (at most four per question; more go in consecutive questions). One agent per run.
- **None here** (header `Agent code`): "I don't see an AI agent in this folder. Where is your agent's code?"
  - **In a folder on this computer (default):** "Tell me the path." Check it's an agent project, then use that absolute path for every file from now on. If you can't open it: "I can't open `<path>` from here. Start me in that folder, or add it to this session, and say "continue setting up Guardian"." (In Claude Code: `cd <path> && claude`, or add the folder to the session.)
  - **In a Git repository:** "Give me its address. I'll make a read-only copy." After a clear yes to cloning that address into a new folder, clone it and say the original can't be changed from here: the project changes in step 1 become a patch for its owner.
  - **Only in Databricks:** "I'll copy it from your workspace, read-only." This needs the Databricks command-line tool: follow `shared/platform-prerequisites.md`, then, after a clear yes, export it with the customer's chosen profile (`databricks workspace export-dir <workspace path> <new local folder> -p <profile>`).
  - **I don't have the code:** "We can still work on its agent contracts and rules, and I'll write a note for your developer." Continue with what needs no code (steps 3 to 5 for an agent Collibra already knows), and at the end write a hand-off note for the developer covering preparing the agent and the scan.
- **Use what Collibra knows:** when the customer names an agent Collibra knows, `get_latest_agent_scan` gives the repository and commit of its last scan: "Collibra last scanned `<agent>` from `<repository>` at commit `<short>`. Is the code there?"
- **Anchor to the project:** the state file, the rules file, the SDK settings and the environment the SDK is installed into all live under the agent's project path, never the current folder.
- **Coming back from another folder** with no state file here: "I don't see your Guardian setup in this folder. Which project were we working on?"

## Step 4: Recommend one next step

From what step 2 found, with the time it takes:

| Found | Say |
|---|---|
| An agent here, not known to Collibra | "This folder has an AI agent, `<agent>`. Collibra doesn't know it yet. `<Your company has a company-wide agent contract.` \| `No agent contract exists yet.>` I suggest we set up Guardian for this agent. Most of the time goes on waiting for Collibra and for the deploy." |
| A known agent whose code, contracts or rules changed (`STALE`, `staleBecause`, or a newer commit than the last scan) | "Collibra knows [`<agent>`](<instance>/asset/<aiAgentId>) (version `<n>`), and `<its code` \| `its agent contracts>` changed since its rules were applied. I suggest we update it." |
| No agent here, and the customer doesn't point to one | "I don't see an AI agent in this folder. I suggest we start with your agent contracts. You can add an agent later from its project folder." |

Then (header `Next step`): **Start with this (default)** / **Show me what else I can do**.

## Step 5: The menu, only if asked

Header `Get started`: "Here's what I can help with:"

- **Set up Guardian for an agent:** "Prepare your agent, add it to Collibra, apply its rules and switch Guardian on." Stages 02 to 09.
- **Write or change agent contracts:** "Start with the rules. You can add an agent later." Stage 04, then, if an agent is known, offer stages 06 to 09.
- **Update an agent Collibra already knows:** "Your agent's code or its rules changed, and you want Guardian to use the latest." Name the cause from `staleBecause` in plain words: `SCAN_CHANGED`, or new code since the last scan, means stages 02 (only what's missing) and 03, then 06 to 09; `CONTRACT_VERSION_CHANGED` or `GOVERNING_SET_CHANGED` means stages 06 to 09.
- **Check an agent's setup:** "See what's in place, what's missing, and any issues." Read only: show the summary from Stage 09 with what is missing, then recommend the next step.

## Step 6: The plan

"Here's the plan: `<the steps that apply, numbered, with their customer step numbers>`. Each step ends with a one-line update and links to what I found or changed. You can ask "what is ...?" about anything at any time."

Then "Learn more:" and the "About Guardian" page from `references/documentation.md`.

## Blocks found at the start

| Found | What to do |
|---|---|
| AI Agent contracts or Guardian turned off | "Collibra settings" in `references/verify-and-diagnose.md`. Only the switch the chosen path needs blocks it. |
| The Guardian tools missing | "Your Collibra connection doesn't include the Guardian tools yet. Ask whoever set up your Collibra connection for the private preview version, then start a new conversation." Stop. |
| The client can't run code | "In this assistant I can work with your agent contracts, AI use cases and rules in Collibra, but I can't install packages, change your code or save files. To prepare your agent, open this conversation in an assistant that can work with your project folder." Then offer only steps 3, 4 and 5. No state file. |
| Python older than 3.12, or none | Not a block: step 1 offers to set up or install Python 3.12. Say it there. |

## Exit criteria

The customer chose a path; the agent and its absolute project path are known, or the customer has no code; the explanation depth is set; the plan is shown.
