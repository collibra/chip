# Stage 04: Deployment

Run this after Stage 03, and only when all of these hold; otherwise skip it and go to Stage 05 (the agent is recorded as standalone, or with what Stage 01 and the known agent already gave):

- `platform.json` has a host that has a platform file (today `databricks`) and the host's CLI is installed. The route does not matter here: an agent hosted on Databricks is looked up through `databricks` whatever its telemetry route is.
- The customer agrees to a live look-up (asked in Step 2 below).
- Stage 02 did not end the run: a known agent with the verdict `unchanged` never reaches this stage unless the customer asked for a re-read.

It finds out which version of the agent is actually deployed, describes that, and decides which code is scanned. Load the platform file (`references/platform-databricks.md`) with this stage and follow its live exploration; `references/platform-router.md` holds the rules every platform file obeys (read-only, consent, no secrets, every value has a source).

## Step 1: Take what is already known

Use the known agent's platform metadata (workspace id, model name) when Stage 02 found one, and Stage 01's offline facts. Do not look up or ask again what they already say.

## Step 2: Ask what is needed, once, here

This is where the customer is asked about the look-up, because only now is it known that there is something to look up. Ask in one form, using the question rules in the main file, only the questions that apply:

1. "This agent runs on <platform>. May I look up how it is deployed (which version is live, its endpoint and configuration) with read-only commands on your <CLI>? I will not call the agent." Options: "Yes, look it up (default)" and "No, skip it". A "no" skips the rest of this stage: keep the offline facts and warn that deployment details were not looked up.
2. If yes and the platform file says a profile or account must be chosen: the profile choice, showing all of them with their addresses. Never choose one yourself, even if there is only one (add the option "Do not look anything up", so the question has two options).
3. If yes and the platform file supports it: "Should I also fingerprint the earlier versions of this agent, so Collibra has its history (one file per version, sent oldest first, all covered by one yes)?" Options: "No, only the deployed version (default)", "All of them" and "The last 5". Do not ask when Stage 02 found a known agent: Collibra already holds the history it knows.

Record the answers in `intake.json` (`platformLookup`, `earlierVersions`). A refused or failed look-up is not fatal: keep the offline facts, warn what is missing, do not retry a sign-in or permission error.

## Step 3: Follow the platform file

It decides which version is deployed, whether the working copy matches it, and whether the deployed code is extracted and scanned instead. It uses the same "relevant files" idea as Stage 02 when it compares commits: tests, documentation and plain assets do not count.

## Step 4: Fill the gaps from the deployed configuration

This is the second reason for the platform check: the host's CLI (`platform.json` says which, and that it is installed) can read what is really deployed. Where the code leaves a value open because it reads an environment variable, the deployed value is in the deployment's configuration. The platform file says where to read it (for Databricks, the serving endpoint's `environment_vars`) and writes `deployed-config.json`. Stage 05 uses it to resolve the model and other values, and says in a warning where each value came from. Never keep a value that looks like a secret, and never print one.

## Step 5: Hand back

Record the results as the router's "What a platform file hands back" table says: the `platform` block, the root agent's `deployment` (`version`, and `endpoint` with `url` and, for Databricks, the serving endpoint's `name`), `source` when deployed code is scanned, the default agent id, `deployed-config.json`, and warnings. The deployment's `version` and `endpoint.url` are in the document hash, so a redeploy is a new version in Collibra.

If the scan root changes (the deployed code was extracted into the output folder), run Stage 03 again on that folder before Stage 05, so evidence paths are relative to it.

## Exit criteria

- The look-up was done and recorded, or skipped with a reason in a warning.
- The scan root for Stage 05 is decided, and `deployed-config.json` exists when the deployment was read.
- Nothing on the platform was created, changed, deployed or called; no credential or secret value was printed or stored.
