# .pesto

PESTO packs for collibra/chip, published by team `ai-services` as git-backed packs: PESTO keeps
a pointer to each manifest under `packs/ai-services/` and reads it, and the files it names, from
this repository (or, for the shared files, from collibra/ai-evals) at submit.

collibra/chip is deprecated, so it carries ONE pack, kept until chip is retired, and no ladder of
smaller packs.

| pack | what it does | $/attempt |
|---|---|---|
| `chip-legacy` | implements the story, one review → fix round, opens the PR ready for review with a findings ledger, gets CI green, builds the branch's chip into a chip-binary image, provisions an OAuth DGC whose ai-copilot runs that binary, and a fresh validate-ac calls chip's tools through `/rest/mcp` and posts per-criterion evidence; fixes nothing it finds unmet | $126 |

- **$/attempt** is the most one attempt can spend on Claude: every agent node's `cost_budget_usd`
  summed, times the review-round count. Script nodes (`build-chip`, `provision-env`,
  `teardown-env`) cost nothing.
- **Review rounds.** One review → remediate-review round, in the `repeat_groups` block
  `review-round`. Raise it at submit with `repeat_groups: [{name: review-round, count: 3}]`; each
  extra round adds up to $40.
- **Per-run values** are `<<NAME - explanation>>` placeholders, answered at submit: `JIRA_KEY`,
  `STORY` (reaches implement only), `SPEC` (reaches review and validate-ac, which cannot read
  Jira), `BASE_BRANCH`, `BRANCH_PREFIX`, `TASK_LABEL`, `PR_TITLE`. The acceptance criteria
  themselves are the submit's `acceptance_criteria`, rendered into validate-ac.
- **Conventions the prompts carry.** Commit subjects are Conventional Commits
  (CONTRIBUTING.md, `.githooks/commit-message-check`) with the Jira key at the end,
  `<type>(<scope>): <description> <key>`, never key first (the repository hook rejects it). The
  PR body follows
  `.github/pull_request_template.md`; review checks `docs/TOOL_CONTRIBUTION_STANDARDS.md`,
  `SKILLS.md` and AGENTS.md's stepdown rule. Pods run `go build ./...` and
  `go test -count=1 -p 2 ./...` (`files/go-tests.sh`); `-race` and golangci-lint run only in CI's
  `build` check, which reconcile-ci reads.
- **The chip under test.** `build-chip` compiles the reconciled branch head and pushes it to
  `docker-sandbox.repository.collibra.io/chip-binary:<run tag>-amd64`; its result's digest-pinned
  `CHIP_IMAGE` fills the shared spec, and validate-ac checks the copilot pod runs that digest
  before it judges anything.
- **`validate-ac` always runs**, so a failed build or provision still yields a verdict: code-level
  criteria judged from the branch, runtime criteria not met as environment faults.
- **`teardown-env` always runs**, after validate-ac, with `ALLOW_FAILED_DEPLOYMENTS: "true"` so a
  failed provision does not also fail the teardown.

## Files

| file | from | what it does | read by |
|---|---|---|---|
| `files/go-tests.sh` | here | post_script of every node that commits: `go build ./...` and `go test -count=1 -p 2 ./...` (no `-race`: the pod has no cgo) | implement, remediate-review, reconcile-ci |
| `files/build-chip-binary.sh` | here | compiles `./cmd/chip` from the branch head and publishes it as the chip-binary image (`CHIP_SOURCE=source`); `CHIP_SOURCE=ci` waits for a CI image instead, which needs `CI_IMAGE_REPO` because chip's CI publishes none | build-chip |
| `wait-for-ci-image.sh` | collibra/ai-evals | polls a registry until CI's `<version>-sha-<short commit>` tag exists; bound to every `build-*` node as task_env `WAIT_FOR_CI_IMAGE_SH` | build-chip |
| `dgc-ai-copilot.magneto.yaml` | collibra/ai-evals | the ONE magneto spec every AI Services pack provisions, bound as task_env `SPEC_B64`; this pack fills it for an OAuth stack with the front door and chip in binary mode | provision-env |
| `run-env-step.sh` | collibra/ai-evals | runs collibra/dq's `provision-env.sh` / `teardown-env.sh` (`DQ_STEP`) after filling the spec's placeholders | provision-env, teardown-env |

The shared files are referenced, never copied: `files: [{path: ..., git: {repo: collibra/ai-evals,
path: .pesto/packs/ai-services/files/<file>, ref: main}}]`. Their documentation is collibra/ai-evals'
`.pesto/README.md`.
