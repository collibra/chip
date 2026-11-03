# Guardian Onboarding skill: specification

This folder specifies `collibra/guardian-onboarding`, a skill that walks a customer from "here is my AI agent" to
"Collibra Guardian is checking my agent" in one conversation. It is written so that anyone on the team, working with an
AI coding assistant such as Claude Code, can build the skill, its tools and its evals from it, open the PRs, and keep
improving it as feedback comes in.

Nothing here ships to users. The skill itself will live in `pkg/skills/files/collibra/guardian-onboarding/`, which is embedded
in the chip binary; this folder only describes it.

## Status

Draft specification, agreed in a design review on 2026-11-03. Nothing is implemented yet. Decisions, open questions and
risks are in [decisions.md](decisions.md).

## The files

| File | What it specifies |
|---|---|
| [skill-spec.md](skill-spec.md) | The skill: who it's for, hard rules, the start, each stage with its tools, consent, checks and exit, resuming |
| [wording.md](wording.md) | What the customer reads at each stage, and the glossary |
| [tools-spec.md](tools-spec.md) | The four new tools and the change to `edit_asset` |
| [sdk-and-platforms.md](sdk-and-platforms.md) | Platform command-line tools (only when a step needs them), the Guardian SDK, what changes in the customer's project, Databricks specifics |
| [verification-and-diagnosis.md](verification-and-diagnosis.md) | How the skill checks its own work after every action, and how it finds the cause when something fails |
| [evals.md](evals.md) | The engineering eval harness and its first scenarios |
| [decisions.md](decisions.md) | Decisions, open questions, known risks, and the checklist before each PR |

## Building it with an AI assistant

Start a session in the root of this repository on a branch made from `aicc-prpr/guardian-agents`, and give the assistant
one piece at a time. For example:

```text
Read docs/guardian-onboarding/README.md and tools-spec.md, then docs/TOOL_CONTRIBUTION_STANDARDS.md and
pkg/tools/publish_agent_scan as the pattern to follow. Implement the tool get_agent_guardian_status
as specified, with its tests. Don't change anything the spec doesn't ask for. Stop when go test -race ./...
and golangci-lint run pass.
```

Suggested order, one PR-sized piece each:

1. `edit_asset add_relation` with `relationTypeId` (smallest, unblocks linking a use case).
2. The four tools, one at a time: `get_agent_guardian_status`, `apply_latest_rules`, `get_agent_rules_report`,
   `get_agent_rules_file`.
3. `_shared/platform-prerequisites.md` (from [sdk-and-platforms.md](sdk-and-platforms.md)).
4. The skill: `SKILL.md`, then the stage files, from [skill-spec.md](skill-spec.md) and [wording.md](wording.md).
5. The evals, from [evals.md](evals.md); run them and fix the skill until they pass.
6. The small hand-offs in the existing skills (the scanner's Stage 07 and the contract skill's Stage 08).

Rules for whoever builds it, person or assistant:

- **This repository is public.** No internal host names, people's names, ticket numbers, internal repository links,
  internal tool or log system names, credentials, or real tenant data, in the skill, the tools, the evals or these docs.
- **Follow what's already here.** The agent-scanner and agent-contract skills are the pattern for the skill;
  `docs/TOOL_CONTRIBUTION_STANDARDS.md` and the existing Guardian tools are the pattern for tools. Where this spec and an
  existing convention disagree, raise it rather than pick one silently.
- **Skills must not contradict tools.** `TestServedSkillsOnlyNameRegisteredTools` fails if the skill names a tool that
  isn't registered.
- **Commits follow the Conventional Commits rules in [CONTRIBUTING.md](../../CONTRIBUTING.md)**, for example
  `feat(guardian-onboarding): add get_agent_guardian_status tool`.

## Iterating

The spec is the source of truth while the skill is young. For each round of feedback (a test run, a review, a support
case):

1. Write the change into the spec file it belongs to, and the reason into [decisions.md](decisions.md).
2. Change the skill, tool or eval to match, with the assistant, in the same PR.
3. Add or update an eval scenario that would have caught the problem.

Branches: work on a branch made from `aicc-prpr/guardian-agents`, test with the team there, and open one PR back into
`aicc-prpr/guardian-agents` when the feedback is fixed.
