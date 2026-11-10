# What the customer reads

Communication standards. These follow the agent-contract skill, chip's most complete example of customer-facing
wording (its `stage-00-security-and-scope.md`, "Behaviours that keep the customer in control", and `statement-guide.md`,
"Words to avoid in your own messages" and "Offering choices"):

- **Simple and meaningful.** Second person, sentence case, one idea per sentence. Every explanation says what the thing
  is **and why it matters to the customer**, in one or two sentences. If a sentence doesn't help the customer decide or act,
  leave it out.
- **The names customers see in Collibra:** agent contract, AI use case, AI Command Center, the Contract and Guardian tabs,
  Apply latest rules. A product term the customer needs appears once with a short explanation; the glossary has them.
- **Ask, don't assume.** When something can be read two ways, ask, and offer the best reading as the default.
- **Show what was found before offering to reuse it.** When the skill finds an existing AI use case or agent contract that
  could fit, it first shows it: the name, one line of its description, what it's already linked to (for example "used by
  2 other agents"), and its link. Only then it asks, and the customer decides: use it, create a new one, or skip. Never
  link an existing asset the customer hasn't seen.
- **Summarise before moving on.** Each step ends with a short status line (one to five lines) and the next question.
  Never paste whole tool results, the agent scan, the rules report or code: summarise, details only on request.
- **Links in every status line.** Each status line ends with clickable markdown links (`[name](url)`) to what the step
  found, created or deployed, built from the instance URL and the ids the tools returned (skill-spec.md, "Links"). A
  link is never invented.
- **Questions end naturally.** A consent question ends with something like "Shall I go ahead?", never with an exact reply
  to type. Any clear yes to that question counts (yes, go ahead, do it, continue, sure, ok, please do); a question back,
  a condition ("yes, but..."), silence, an unclear reply or a yes to a different question doesn't.
- **Choices inside a sentence are lettered** (a), (b), so a reply like "ok" can't be mistaken for a choice. Menus shown as
  their own list keep numbers or the client's question form.
- **Show your working on wording, never on mechanism.** Never explain how Guardian works inside; the only explanation of
  enforcement is the agent-contract skill's fixed sentence.
- **Words to avoid in your own messages:** this skill's own labels and numbers (hard rule ids, "stage", "this skill", "per
  my instructions"), any status Collibra didn't return in this conversation, and internal terms: bundle, compile, spec,
  binding, guard, collector, job, API, endpoint path, internal system names. Say instead: rules file, apply the rules,
  check, connection, Collibra support. The description of the agent sent to Collibra is the "agent scan".
- **Depth follows the customer's answer** to "How familiar are you with Collibra AI governance?", visibly from the first
  message after it. New: the eight steps in a line each, the full "Before" text at every step, and a sentence on each
  option. Knows Collibra: the step names, and a Guardian term explained only the first time. Uses Guardian: no
  overview, no explanations, only the question or action and the one-line status. Every stage file carries its own
  "Before" text for each depth.

Placeholders are in angle brackets. Customer-facing steps are numbered 1 to 8; they map to stages 02 to 09.

## Start

| When | Wording |
|---|---|
| Opening | "I'll help you set up Collibra Guardian for your AI agents. Guardian checks what an agent does against rules you write in plain language, called agent contracts, and records any issues in Collibra. I'll ask before I change anything in Collibra or in your files. Let me check a few things first." |
| One question (header `Experience`) | "How familiar are you with Collibra AI governance?" Options: **I'm new to it (default):** "I'll explain each step in a sentence before we do it." **I know Collibra, but Guardian is new to me:** "I'll explain Guardian's terms the first time they come up." **I already use Guardian:** "I'll keep it short: one line per step." |
| Recommendation, agent not known | "This folder has an AI agent, `<agent>`. Collibra doesn't know it yet. `<Your company has a company-wide agent contract.` \| `No agent contract exists yet.>` I suggest we set up Guardian for this agent. Most of the time goes on waiting for Collibra and for the deploy." |
| Recommendation, agent known and changed | "Collibra knows [`<agent>`](`<link>`) (version `<n>`), and `<its code` \| `its agent contracts>` changed since its rules were applied. I suggest we update it." |
| Recommendation, no agent here | "I don't see an AI agent in this folder. I suggest we start with your agent contracts. You can add an agent later from its project folder." |
| Then (header `Next step`) | **Start with this (default)** / **Show me what else I can do** |
| The menu (header `Get started`) | "Here's what I can help with:" **Set up Guardian for an agent:** "Prepare your agent, add it to Collibra, apply its rules and switch Guardian on." **Write or change agent contracts:** "Start with the rules. You can add an agent later." **Update an agent Collibra already knows:** "Your agent's code or its rules changed, and you want Guardian to use the latest." **Check an agent's setup:** "See what's in place, what's missing, and any issues." |
| The plan | "Here's the plan: `<the steps that apply, numbered>`. Each step ends with a one-line update and links to what I found or changed. You can ask "what is ...?" about anything at any time." |

## Where is the agent's code?

| When | Wording |
|---|---|
| No agent in the current folder (header `Agent code`) | "I don't see an AI agent in this folder. Where is your agent's code?" **In a folder on this computer (default):** "Tell me the path." **In a Git repository:** "Give me its address. I'll make a read-only copy." **Only in Databricks:** "I'll copy it from your workspace, read-only." **I don't have the code:** "We can still work on its agent contracts and rules, and I'll write a note for your developer." |
| The folder isn't reachable here | "I can't open `<path>` from here. Start me in that folder, or add it to this session, and say "continue setting up Guardian"." |
| Several agents in the folder (header `Which agent`) | "I found `<n>` AI agents here. Which one should we set up?" One option per agent, with the file it's in. |
| Collibra knows the agent | "Collibra last scanned `<agent>` from `<repository>` at commit `<short>`. Is the code there?" |
| Coming back from another folder | "I don't see your Guardian setup in this folder. Which project were we working on?" |

## Collibra settings

Two switches, and one option, on **Settings > AI Command Center > Guardian and contract**
(`<instance>/settings/ai-governance/guardian`). Only a Collibra administrator can change them.

| Setting | What it means for the customer | Needed for |
|---|---|---|
| **AI Agent contracts** (on/off) | Lets your company write agent contracts in Collibra. | Writing or changing agent contracts |
| **Guardian** (on/off) | Lets Guardian check your agents against their agent contracts and record issues in Collibra. Turning it on also turns on AI Agent contracts. Turning it off only hides Guardian in Collibra: agents that have the SDK keep being checked until it's removed from their code. | Applying the rules and seeing issues |
| **Allow collecting payloads to enable showing details of the violations** (option under Guardian) | Lets Collibra keep what was said in a flagged conversation (the user's request and the agent's reply), so you can see why something was flagged. Without it you still see that a rule was broken, but not the conversation. Your company decides this for privacy reasons. | Seeing the conversation behind an issue |

| When | Wording |
|---|---|
| A switch is off | "`<AI Agent contracts \| Guardian>` is turned off in your Collibra, so `<what it blocks, in plain words>`. A Collibra administrator turns it on in Settings > AI Command Center > Guardian and contract: `<settings link>`." Then (header `Admin`): "Are you a Collibra administrator?" **(default) Yes, show me how** / **No, write a message for my administrator** |
| Yes, show me how | "1. Open `<settings link>`. 2. Turn on `<AI Agent contracts \| Guardian>`. 3. Come back here and type "continue". I'll check it worked." |
| A message for the administrator | "Hi, I'm setting up Collibra Guardian for our AI agent `<agent>`. Could you turn on `<AI Agent contracts \| Guardian>` in Collibra: Settings > AI Command Center > Guardian and contract (`<settings link>`)? `<If Guardian: Please also decide whether to allow collecting payloads, which shows the conversation behind each issue.>` Thank you." Then: "When it's on, type "continue"." |
| Payload collection is off (only mentioned when the customer opens an issue or asks why there's no detail) | "Your company has chosen not to keep conversation details, so I can show that a rule was broken but not what was said. An administrator can change this in the same settings page." |

## Blocks found at the start

| Found | Wording |
|---|---|
| Agent contracts or Guardian turned off | The "Collibra settings" wording above. |
| Guardian tools missing | "Your Collibra connection doesn't include the Guardian tools yet. `<How to get the preview version of the connection>`, then start a new conversation." |
| Can't run code here | "In this assistant I can work with your agent contracts, AI use cases and rules in Collibra, but I can't install packages, change your code or save files. To prepare your agent, open this conversation in an assistant that can work with your project folder." |

## Step by step

| Step | Before | Question or consent | Done | If something's missing |
|---|---|---|---|---|
| 1 Prepare your agent | "First, I'll prepare your agent for Guardian. That means installing the Guardian SDK, the Python package that lets Guardian check your agent, and adding three lines and a small settings file to your project. I'll show you every change before I make it." | "Where did you save the Guardian SDK files from the private preview portal?" Then the mode (header `Mode`): "When Guardian finds your agent about to break a rule, what should it do?" **Monitor: record it only (offered first):** "Your agent carries on as normal, and Collibra shows what Guardian would have stopped. A safe way to start and to check your rules." **Enforce: stop it:** "Guardian applies your rules: it blocks the action, or changes it (for example, hides sensitive data), as your rules say. Your users see the effect straight away." Then: "You can change this later by asking me, or in the settings file." After showing the changes: "Shall I install the Guardian SDK and make these changes to your project? I'll change nothing until you say yes." | "Step 1 of 8 done: the Guardian SDK `<version>` is installed. Guardian has no rules yet, so for now it lets everything through. Once your rules file is in place (step 6), when your agent breaks a rule, Guardian will `<only record it` \| `stop it>`. Changed: [`collibra-guardian.toml`](`<path>`)`<, and each other changed file>`." | "I need the Guardian SDK files to continue. Download them from the private preview portal, tell me the folder, and type "continue"." / "Your project uses Python `<version>`. The Guardian SDK needs Python 3.12 or later." |
| 2 Add your agent to Collibra | New: "Next, I'll read your agent's code and describe it to Collibra: its models, instructions and tools. Collibra calls this an agent scan." Known: "Collibra already knows `<agent>` (version `<n>`). I'll check whether your code changed since then." | The scanner skill's own questions; it asks for consent in its own way. | "Step 2 of 8 done: Collibra knows `<agent>`, version `<n>`. Links: [`<agent>`](`<link>`), [version `<n>`](`<link>`)`<, and where it runs: [<endpoint name>](<link>), when the agent is deployed>`." Unchanged: "Step 2 of 8 done: your code hasn't changed, so Collibra already has the latest version. Links: [`<agent>`](`<link>`), [version `<n>`](`<link>`)." | "Collibra is still adding your agent. This usually takes under a minute. I'll keep checking." |
| 3 Add agent contracts | "An agent contract is a set of rules for your agents, written in plain language. One company-wide contract can apply to every agent, and an agent can also have its own." Before the hand-off to the contract skill: "Step 3 of 8: your agent contract. I'll draft rules from what your agent does, ask you for values I can't know, go through the rules with you, and show you the whole contract before anything is saved. After it's saved, I'll link it and we go on to step 4." During the hand-off, every message that waits for the customer ends with one line: where they are and what happens after they answer. | None apply (header `Contracts`): "No agent contract applies to `<agent>` yet. What would you like to do?" **Write a contract for this agent (default)** / **Write the company-wide contract** / **Stop here for now**. When an existing specialised contract could fit, show it first ("I found an agent contract that may fit: **`<name>`**, `<n>` rules, for example `<one rule>`. It applies to `<n>` agents. `<link>`") and add **Use `<name>`** as the first option | "Step 3 of 8 done: `<n>` agent contracts apply to `<agent>`: [`<name>`](`<link>`) (`<rules>` rules), one per contract`<. The new link shows on the agent's [Contract tab](<link>)>`." | (header `Link`) "Your contract `<name>` isn't linked to `<agent>` yet, so its rules don't apply to it. Shall I link it to `<agent>` now?" **Yes, link it** / **I'll do it myself in Collibra** / **Not now**, none labelled the default. After the customer's yes, the skill links it, checks it and gives the link. Only if the customer prefers it, or the link fails: "Open [`<name>`](`<contract link>`), link it to `<agent>`, then type "continue"." |
| 4 Say what your agent is for (optional) | "An AI use case describes what your agent is for and its risks. Guardian applies your rules more accurately with one." | (header `Use case`) When one or more existing use cases could fit, show each first: "I found an AI use case that may fit: **`<name>`**, `<one line of its description>`. It's already used by `<n>` agents. `<link>`" Then: "`<agent>` isn't linked to an AI use case. What would you like to do?" **Use `<name>` (default, when one is found)** / **Choose another existing AI use case** / **Create one from your agent** / **Skip for now**. Existing one: "I'll link `<use case>` to `<agent>`. Shall I go ahead?" New one, after showing the proposed name and description: "Shall I create this AI use case and link it to `<agent>`?" If the link fails or the customer prefers to do it: "Open [`<use case>`](`<link>`), add a relation **uses**, choose the AI Agent `<agent>`, then type "continue" and I'll check it's linked." | "Step 4 of 8 done: `<agent>` is linked to the AI use case [`<name>`](`<link>`)." | Skipped: "Step 4 of 8 skipped: you can link an AI use case later from [the agent's page](`<link>`)." |
| 5 Apply the rules | "Now I'll apply your agent contracts to `<agent>`. Collibra turns each rule into checks Guardian can run. This takes about five minutes. It's the same as **Apply latest rules** on the agent's **Contract** tab." | "Shall I apply the latest rules to `<agent>` now? I'll change nothing until you say yes." | "Step 5 of 8 done: `<n>` of your `<total>` rules will be checked while your agent runs. `<m>` are policy only, and `<k>` don't apply to this agent. Details on the agent's [Contract tab](`<link>`)." While waiting: "Collibra has started. While it works, I'll get the next step ready." | "None of your rules could be turned into checks for `<agent>`: `<plain reason>`. Would you like help making them more specific?" |
| 6 Save the rules and deploy | "I'll save your agent's rules file to `<path>`, where the Guardian SDK will find it, and check it's the file Collibra made." | "Shall I save the rules file and add the setting that points to it? I'll change nothing until you say yes." Then: "To start checking, deploy your agent again: `<steps for the platform>`. Guardian reads the rules file when your agent starts." | "Step 6 of 8 done: the rules file is saved in [`<path>`](`<path>`) and checked. Deploy your agent to start checking." After the deploy, where it runs, only when known and never guessed: the App [`<app name>`](https://`<workspace host>`/apps/`<app name>`) or the serving endpoint [`<endpoint name>`](https://`<workspace host>`/ml/endpoints/`<endpoint name>`), and its AI Endpoint in Collibra [`<name>`](`<link>`). | "The download didn't complete, so I'll try once more." / "The file doesn't match what Collibra made, so I won't use it. Please share the details below with Collibra support." |
| 7 Let Guardian see your agent's activity | "Last, Guardian needs to read your agent's activity in Databricks. A Collibra administrator adds one Databricks connection on a running Collibra Edge site, once. Guardian sets up the rest itself." | Missing: the instructions and a documentation link, then "When the connection is in place, type "continue"." Optional: "Shall I look at your agent's recent runs in `<where>` to see what Guardian stopped or flagged? I'll read only Guardian's decisions, never the conversations, and change nothing." | "Step 7 of 8 done: Guardian is reading your agent's activity. Collibra last read it at `<lastCollectedOn, in the customer's time>`. New issues appear on the agent's [Guardian tab](`<link>`)." | "Collibra isn't connected to your Databricks workspace yet. Here's a message you can send to your Collibra administrator: `<message>`" |
| 8 Summary | "Guardian is set up for `<agent>`." The summary as a list, every item with a real clickable link taken from the tools (never a placeholder), including "Endpoint: `<the App or the serving endpoint in Databricks, linked>`, where you can check the deployed agent" when the skill found it, then: "Come back to this conversation, or say "update Guardian for my agent", when your agent's code or its agent contracts change, or when you want to change what Guardian does when a rule is broken." | | | |

## Across all steps

| Situation | Wording |
|---|---|
| A step only the customer can do | "`<what to do>`. When it's done, type "continue" and I'll pick up from here." |
| Links don't open | When the customer says a link doesn't open, or asks for the full links: from then on, each item's full address as plain text after it, because some terminals can't open markdown links. |
| Only the deployment changed (header `Send scan`) | "Your agent's code hasn't changed since Collibra last saw it; only its deployment changed. Collibra keeps the same agent version and its rules either way. Sending a new scan records this deployment in Collibra's history, but nothing new appears on the agent's pages. What would you like to do?" **Don't send it** / **Send it**, with no default. |
| Waiting for the rules to apply | One line per check: "Applying your `<n>` rules: `<minutes>` of about `<typical>` minutes so far." No rule count or percentage: Collibra reports none until the run ends. |
| A quiet retry | Nothing, if it works. After a second failure: "Collibra didn't respond, so I've stopped here. Nothing was changed." |
| No permission | "Your Collibra account doesn't have permission to `<action>`. Here's a message for your Collibra administrator: `<message>`" The message names the role from the documentation's roles table that covers the action: AI Producer for agents, AI Business User for writing agent contracts, AI Governance Admin for settings. |
| Framework version older than the SDK needs | "Your agent uses `<LangChain 1.3>`, and Guardian needs `<1.4.5>` or later. I'll move your project to `<1.4.5>` as part of the changes." |
| The summarising step isn't checked | "Guardian checks your agent's own steps, but not the step that summarises a long conversation, because `<the cause in plain words>`. Fixing that changes your agent's code, so Collibra will see a new version of your agent and its rules need applying again. Would you like me to?" |
| Framework version newer than the tested range | "Your agent uses `<LangChain 2.1>`, and Guardian is tested with `<1.4 up to 2.0>`. As it is, your agent wouldn't start with Guardian. I can (a) set your project to a tested version, or (b) let it start anyway and only log a warning. Which would you like?" |
| Collibra hasn't checked the activity yet | "Collibra hasn't checked yet. It checks about every 15 minutes." Then an offer to check again later |
| The connection is added but not read yet | "Collibra checks for new connections regularly, about every 15 minutes, so it can take up to half an hour after the connection is added before it reads your agent's activity. I can check again later, or you can come back with "continue setting up Guardian"." |
| Support details | "If you contact Collibra support, share these details:" then instance, time (UTC) and the ids, labelled Agent, Agent version, Scan, Rules file, Use case, Contracts. |
| Coming back | "Welcome back. Last time we finished step `<n>` for `<agent>`. I'll check what's changed in Collibra since then." |
| A consent question | Show the change, then end naturally: "Shall I go ahead?" Any clear yes to that question counts; no exact phrase. |
| No clear answer to a consent question | The question again, word for word. After the second time: "If you'd rather not do this now, say so and I'll leave everything as it is." |
| Fewer or more checks than last time | "This time `<n>` of your `<total>` rules became checks, compared with `<previous>` last time. Collibra reads plain-language rules a little differently each time, so the number can move slightly. The rules file in use is always the newest one." Say it only when the count changed and the contracts didn't. |
| A check after an action failed | "I `<did X>`, but when I checked, `<what I found>`. `<What I'll do or what you can do>`." |

## Glossary ("what is ...?")

| Term | Answer |
|---|---|
| Collibra | "Collibra is where your company keeps track of its data and AI, and the rules for using them." |
| AI Command Center | "The part of Collibra for AI: your AI agents, models, AI use cases and their rules." |
| Guardian | "Guardian watches what an AI agent does while it runs, checks it against your agent contracts, and records any issues in Collibra." |
| Agent contract | "A list of rules for AI agents, written in plain language, such as "never share a customer's card number". One can apply to every agent; an agent can also have its own." |
| AI use case | "A description of what an AI agent is for, who uses it, and its risks." |
| Rules file | "The file Collibra makes from your agent contracts. Your agent reads it when it starts, so Guardian knows what to check." |
| Monitor (record only) | "Guardian records what it would have stopped, without stopping anything. You can switch to stopping it at any time." |
| Collecting payloads | "Keeping the conversation behind a flagged action, so you can see why it was flagged. Your company decides whether Collibra keeps it." |
| Guardian SDK | "The Python package that lets Guardian check your agent. It comes from the private preview portal." |
