# Decisions, open questions and risks

## Decisions

From the design review on 2026-11-03, unless dated otherwise.

| Topic | Decision |
|---|---|
| Name | Guardian Onboarding, `collibra/guardian-onboarding` (2026-11-03). "Guardian Setup" was too close to Guardian Agents and the Guardian settings page. |
| Which skill | A new skill drives the whole journey. The scanner and contract skills stay as they are and are called from it. |
| Start | Look first, ask one question about explanation depth, recommend one next step; the menu only on request. Works for new and existing customers, governance and technical. |
| Mode | The customer chooses what Guardian does when a rule is broken: record only, or stop the action, each explained simply. "Record only" is offered first as the safer start; the skill always writes the choice, because the SDK's default is to stop. |
| Collibra settings | The skill checks the AI Agent contracts and Guardian switches, explains what an off switch blocks, and guides an administrator or writes them a message, then continues. Payload collection is the company's choice and never pushed. |
| Wording | Simple and meaningful, following the agent-contract skill's communication rules (wording.md). |
| Order | Prepare the agent (SDK and wiring) first, then the scan, contracts (global first), the optional use case, apply the rules, save the rules file where the SDK reads it, deploy once, and the activity connection last. |
| SDK | Mandatory, checked first, installed only with consent from the files the customer downloaded from the private preview portal. Versions are read from the files, never hard-coded. |
| Reusing existing assets | When the skill finds an existing AI use case or agent contract that could fit, it shows it first (name, description, what it's linked to, link); the customer decides to use it or create a new one. |
| Use case | Optional. Suggest similar existing ones by keyword search, or draft a name and description from the scan for the customer to edit; create and link only after "yes, create". |
| Linking | By relation type id, through a change to `edit_asset add_relation`. A specialised contract that isn't linked to the agent: the skill proposes linking it and the customer decides (the skill after "yes, link", or the customer on the contract's page). |
| Applying rules | The customer never looks for a compile button: the skill asks "yes, apply" and runs it. |
| Rules file | Through the API, never a command-line tool, and saved where the SDK reads it. |
| No Guardian API changes | The Guardian APIs are released, so the skill works with them as they are (2026-11-03). When no rules file is made, it checks the likely causes in order: no contract, then contracts with no rules. After linking a use case it offers to apply the rules again itself. When collecting activity fails, or Guardian may be turned off, it gives the customer the settings guidance or the ids for support. |
| Product documentation | The skill follows the private preview documentation and links to it rather than copying it, because it's still changing (skill-spec.md, "Product documentation"). All links live in one file, `references/documentation.md`, because the addresses will change at general availability. |
| Rules-file path | No setting names the rules file before the file is there, because the SDK refuses to start when a named file is missing. The path and the file are written together in stage 07. |
| Platform command-line tools | Checked only when a step needs them (the deployment look-up, the deploy), never at the start. Installing, signing in and choosing the profile are optional; without the tool the setup continues and the deploy moves to the customer or the note for whoever deploys. |
| Tools | Four platform-neutral tools. An Edge read tool was considered and dropped: the status's collection reason says what's missing. |
| A step only the customer can do | Instructions and a documentation link, then "continue". The activity connection is last, so it never blocks the rest. |
| Errors | No internal system names. The customer gets ids to share with support. |
| Retries | Automatic only where safe; never after 401 or 403. |
| Verification | After every action the skill checks its own work; the customer doesn't have to. |
| Evals | The team's engineering evals, the same kind as the contract skill's, in `evals/guardian-onboarding/`. |
| Public repository | Nothing internal in the skill, tools, evals or these docs. |
| Branches | Work on a branch from `aicc-prpr/guardian-agents`; test with the team there; one PR back into `aicc-prpr/guardian-agents`. |

## A redeploy can look like a new agent version

**The issue.** A root agent's version is identified by its code's hash together with its deployment version. Rules are
made for one version, and issues are filed under the version the rules file was made for. So:

- if switching Guardian on changes the agent's code as the scanner sees it (explicit middleware, a guarded constructor),
  the next scan is a new version while the agent runs the previous version's rules file;
- on Databricks Model Serving, shipping a rules file means a new registered model version, so the next scan that reads
  the deployment is a new version too, with the same code;
- the newest version then shows no rules applied while the agent is in fact enforcing the previous one's, and it repeats
  with every rules update.

**What the skill does:**

- prepares the agent (SDK and wiring) before the scan, with the default way that doesn't change the scan (way 4);
- doesn't send a new scan when the code is unchanged and only the deployment changed;
- detects it after the fact: two consecutive versions with the same code, and explains it to the customer.

**Still to decide** with the Guardian service owners:

- the version identity on the server: the code hash only, or carrying the rules forward to a new version with identical
  code (either covers every source of scans, not only this skill);
- whether the scanner should leave Guardian's own wiring out of the scan;
- for Model Serving, keeping the rules file outside the registered model (for example a volume the endpoint reads), so a
  rules update doesn't create a model version; not yet tested.

## Known gaps in the documentation

Checked on 2026-11-03. The skill keeps its own steps for these until the documentation covers them:

- Scanning the agent (the fingerprint) isn't documented; the documentation says only that Guardian must "recognise" the
  agent.
- The "Allow collecting payloads" option isn't described, though the Guardian tab topic refers to it.
- Connecting Collibra to the agent's activity (Edge, the Databricks connection) isn't documented.
- Databricks Apps and Model Serving aren't covered; the documentation points managed platforms to `guardian-auto`.
- No permission is given for Apply latest rules.
- The documentation says applying the rules needs an AI use case linked to the agent; Guardian doesn't need one. To
  confirm with the Guardian team, and the documentation updated.
- The rules file is documented only through the CLI's `sync`; the skill downloads it through the API.

## Open questions

- The documentation addresses at general availability: when they're known, update `references/documentation.md`.
- Does the hosted Collibra MCP server (per-user OAuth) expose the skills and the Guardian tools, and when?
- Does the private preview portal offer all four SDK wheels, not only `collibra-sdk`?
- Gating: which capability flag, if any, for the Guardian tools and this skill before `main`.
- The product's Contract tab tells customers to "update your SDK reference" with command-line steps; it should point to
  this assistant once it exists.
- One name for the contract skill in the product's copy: "agent contract" (the official name).

## Before every PR

- [ ] Take the latest Guardian SDK from the portal; install it in the fixture project; check every item of
      [the SDK interface](sdk-and-platforms.md#the-sdk-interface-the-skill-relies-on) still holds; update the fixtures.
- [ ] `go test -race ./...` and `golangci-lint run` pass.
- [ ] `evals/guardian-onboarding/lint.sh` is clean, and the evals named in [evals.md](evals.md#when-to-run-them) pass, with the
      results file added.
- [ ] Nothing internal: no internal hosts, people, tickets, internal repositories or system names, credentials, or real
      tenant data, anywhere in the change.
- [ ] The skill still matches the product documentation: each linked page exists (and, once the product is generally
      available, the links point to the general availability documentation), and nothing in "Known gaps in the
      documentation" has since been documented differently.
- [ ] This folder is updated to match what was built, and the reason for any change is in this file.

## Change log

| Date | Change | Why |
|---|---|---|
| 2026-11-03 | First version of the specification | Design review |
| 2026-11-03 | The two causes of stale rules, and how a new rules file reaches the agent | Review with the Guardian team |
| 2026-11-03 | The skill helps with Python 3.12: uses one already installed, or offers the official install after "yes, install" | Testers and customers may not have it |
| 2026-11-03 | Rule outcome marks confirmed; the skill gives the deploy commands and the customer runs them | Review of the first build |
| 2026-11-03 | Show an existing use case or contract before offering to reuse it; the customer decides | Review feedback |
| 2026-11-03 | Propose linking a specialised contract; the customer decides | Review feedback |
| 2026-11-03 | The platform command-line tool is checked only when a step needs it; install, sign-in and profile are optional | Customers who don't need it shouldn't be asked |
| 2026-11-03 | Follow the product documentation; never name the rules file before it exists; check framework versions; mode names Enforce and Monitor | Checked against the documentation and the SDK |
| 2026-11-03 | No changes to the released Guardian APIs; the skill works around the missing reasons | The APIs are released |
| 2026-11-03 | Renamed from Guardian Setup to Guardian Onboarding | Avoid confusion with Guardian Agents |
| 2026-11-03 | The customer chooses the mode; guide the Collibra settings; wording follows the agent-contract skill | Review feedback |
| 2026-11-03 | Find the agent's code when the skill starts outside it; anchor every file to the project path | Customers may start the skill anywhere |
| 2026-11-03 | Where the rules file lives by how the agent runs; finding the deployment when the customer doesn't know | A customer may not know how their agent is deployed |
| 2026-11-03 | The use case link names `AIUseCaseUsesAIAgent` (`...7109`) | Four relation types on AI Use Case share the role "uses" |
