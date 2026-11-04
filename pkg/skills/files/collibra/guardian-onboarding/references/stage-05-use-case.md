# Stage 05: AI use case, optional (step 4 of 8)

Link the agent to an AI use case, if the customer wants one. It is optional: Guardian applies the rules without it.

**Tools:** `get_asset_details`, `search_asset_keyword`, `prepare_create_asset`, `create_asset`, `edit_asset` (operation `add_relation` with `relationTypeId`).

**Before** (by depth):
- New: "Step 4 of 8, optional: what your agent is for. An AI use case describes what your agent is for and its risks, for example "answer customers' questions about their orders". Guardian applies your rules more accurately with one, and it's how your company keeps track of why the agent exists. You can skip it and add it later."
- Knows Collibra: "Step 4 of 8, optional: link an AI use case. Guardian applies your rules more accurately with one."
- Uses Guardian: nothing before the step.

**Before:** "An AI use case describes what your agent is for and its risks. Guardian applies your rules more accurately with one."

## Step 1: Does the agent have one?

`get_asset_details` on the AI Agent; look in its incoming relations for an AI Use Case through "AI Use Case uses AI Agent". If there is one, show it (name, link) and go to the exit.

## Step 2: Look for ones that could fit

`search_asset_keyword` with `assetTypeFilter` "AI Use Case", once for each of a few words from the scan: the agent's name, the purpose in its instructions, the subject of its tools. For each match, `get_asset_details` gives one line of its description and how many agents already use it.

Show each match first, before any question: "I found an AI use case that may fit: **`<name>`**, `<one line of its description>`. It's already used by `<n>` agents. `<link>`"

Then (header `Use case`): "`<agent>` isn't linked to an AI use case. What would you like to do?"

- **Use `<name>` (default, when one is found)**
- **Choose another existing AI use case** (the customer names it; find it, show it the same way, then ask again)
- **Create one from your agent**
- **Skip for now**

An existing use case is never linked without being shown.

## Step 3: Link an existing one

"I'll link `<use case>` to `<agent>`. Shall I go ahead?" After the customer's clear yes: `edit_asset` on the **AI Use
Case** with one `add_relation` operation, `relationTypeId` `00000000-0000-0000-0000-000000007109` (AI Use Case uses AI
Agent), and the AI Agent's id as `targetAssetId`.

Always pass this id, never only the role name: an AI Use Case has several relation types named "uses" (to AI Agent, AI
Base Model, AI Project and AI Model Version), and by name the tool would pick another one. If the link fails (for
example no permission, never retried, `refusal-scripts.md › R-PERMISSION`), or the customer prefers to do it: "Open
[`<use case>`](`<its link from get_asset_details>`), add a relation **uses**, choose the AI Agent `<agent>`, then type
"continue" and I'll check it's linked." The use case is optional, so carry on either way.

## Step 4: Create one from the agent

1. Draft a **name and a description** from the scan: what the agent is for, who uses it, in plain words. Add other attributes only where the code shows them; never guess a risk or an owner.
2. Show the draft and let the customer edit it. Show it again after every change.
3. `prepare_create_asset` for an AI Use Case in the AI Use Cases domain (the customer confirms the domain if there is more than one).
4. Ask, word for word: "Shall I create this AI use case and link it to `<agent>`?"
5. After the customer's clear yes: `create_asset` with exactly the name and description shown, then link it as in step 3 (the same yes covers the link, because it was part of the question).

## Step 5: Check

`get_asset_details` on the AI Agent: its incoming relations now include the use case through "AI Use Case uses AI Agent". If not, say what you found, never success.

If the rules were already applied to this agent: linking a use case doesn't mark them out of date, so say "I'll apply your rules again in the next step, so they take the use case into account."

## Exit

- Linked: "Step 4 of 8 done: `<agent>` is linked to the AI use case [`<name>`](<instance>/asset/<useCaseId>)."
- Skipped: "Step 4 of 8 skipped: you can link an AI use case later from [the agent's page](<instance>/asset/<aiAgentId>)."

Exit when the use case is linked, or skipped. Keep its id for the summary and the support details; on resuming, read it again from the agent's relations.
