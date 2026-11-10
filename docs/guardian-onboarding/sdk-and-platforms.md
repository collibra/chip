# Platforms, the Guardian SDK, and the customer's project

## Python 3.12 or later (in `_shared/platform-prerequisites.md`)

The Guardian SDK needs Python 3.12 or later in the agent's project environment. Check it, and help when it's missing,
the same way as a platform tool:

1. **Check (always):** the project environment's Python (`<project env>/bin/python --version`, or `python3 --version`
   when there's no environment yet), and whether a newer Python is already on the computer (`uv python list` when `uv`
   is there, `python3.12 --version`, `python3.13 --version`).
2. **A newer Python is already there:** offer to create the project's environment with it, as part of the step 1
   changes (one yes for all of them). Never change the customer's system Python.
3. **None is there (optional install):** offer the official command for the customer's system, shown first, and run it
   only after the customer's yes:

   | Customer has | Command |
   |---|---|
   | `uv` (no admin rights needed) | `uv python install 3.12` |
   | macOS with Homebrew | `brew install python@3.12` |
   | Windows | `winget install Python.Python.3.12` |
   | Anything else | the installer from python.org, which the customer runs |

   If the customer can't install software, or IT will do it, write the message for IT and pause until "continue".
4. **Where the agent is deployed** must also run Python 3.12 or later. Check the platform's setting where the project
   shows it; otherwise put it in the note for whoever deploys.

## Platform command-line tools: `_shared/platform-prerequisites.md`

A new shared reference, pulled in by the scanner and by this skill with `shared: platform-prerequisites.md` in their
frontmatter, so both behave the same.

**Only when a step needs it, never at the start.** The platform's command-line tool (for Databricks, the `databricks`
command) is needed for two things only: reading how the agent is deployed, during the scan, and deploying the agent with
the rules file, in stage 07. So the start of the skill checks only Python 3.12 or later and the Collibra connection, and
the tool is looked at the first time one of those two steps comes up. A customer who only writes contracts, or whose
agent isn't on a supported platform, is never asked about it.

At that moment, for the platform found:

1. **Check (always, and again after every "continue"):** the version command, and the signed-in profiles without
   contacting any workspace. Read-only, and it costs nothing. This is also how the skill notices a tool someone installed
   later, for example the customer's IT team.
2. **Install (optional):** only an offer, with the vendor's official command for the customer's system shown first, and
   only after the customer's yes. Never a downloaded script piped into a shell. If the customer says no, can't install
   software, or IT will do it, write the message for IT if they want one, and carry on without the tool.
3. **Sign in (optional):** only when the skill is about to use the tool and the profile isn't signed in. The customer runs
   it, because it opens a browser and the assistant never handles tokens or secrets. In Claude Code, suggest typing
   `! <command>` so it runs in the session.
4. **Confirm the account (optional, but always before the first use):** show the profiles with their workspace
   addresses; the customer picks one, never the assistant, even when there's only one; then one read-only "who am I"
   call, and say which account will be used. Look-ups stay read-only.

**Without the tool, nothing is lost, only moved:**

| Needs the platform | With the tool, after the customer agrees | Without it |
|---|---|---|
| How the agent is deployed, during the scan | The assistant reads it, read-only | The scan describes the agent from its code only, and its summary says it couldn't read the deployment (the scanner's warning "deployment details not looked up"). Collibra still gets the agent, but no AI Endpoint until a later scan with the tool |
| Deploying with the rules file (stage 07) | The assistant gives the exact commands, or runs them after the customer agrees | The customer deploys in the platform's web interface or runs the commands themselves, or the steps go into the note for whoever deploys |
| Checking the agent runs the new rules | Not needed: this reads Collibra | The same |

| Platform | Check | Install (macOS / Windows) | Log in | Verify |
|---|---|---|---|---|
| Databricks | `databricks --version` | `brew tap databricks/tap && brew install databricks` / `winget install Databricks.DatabricksCLI` | `databricks auth login --host <workspace URL> --profile <name>` | `databricks auth profiles` (Valid), `databricks current-user me -p <name>` |
| AWS | `aws --version` | `brew install awscli` / `winget install Amazon.AWSCLI` | `aws configure sso`, then `aws sso login --profile <name>` | `aws sts get-caller-identity --profile <name>` |
| Azure | `az --version` | `brew install azure-cli` / `winget install Microsoft.AzureCLI` | `az login` | `az account show` |
| Google Cloud | `gcloud --version` | `brew install --cask google-cloud-sdk` / the vendor installer | `gcloud auth login` | `gcloud auth list` |
| Snowflake | `snow --version` | the vendor's Snowflake CLI installer | `snow connection add` | `snow connection test` |

Confirm every install command against the vendor's current documentation when writing the file. Only Databricks has a
platform file in the scanner today, so only its look-up is used now.

Also check the Databricks workspace id: it's the digits after `?o=` in the workspace URL. Never accept the example id
from the documentation.

## The Guardian SDK

The SDK isn't public and can't ship with chip, which is public. Customers download it from the private preview portal.
It's four Python wheels, all pure Python, for Python 3.12 or later:

| Wheel | Purpose | Needs from PyPI |
|---|---|---|
| `collibra_sdk-<version>-py3-none-any.whl` | The SDK: activation, middleware, injection, the MLflow integration | per extra, below |
| `collibra_policy_enforcement-<version>-py3-none-any.whl` | The engine that applies the rules file | `pyyaml` |
| `collibra_guardian_contract-<version>-py3-none-any.whl` | Definitions shared with Collibra's side (for example names used on traces), in their own package so Collibra's side doesn't need the whole SDK | none |
| `collibra_guardian_auto-<version>-py3-none-any.whl` | Only for the install-once way | none |

Extras the skill uses: `guardian-langchain` (LangChain and LangGraph), `guardian-injection` (switching Guardian on
without changing how the agent is built), `mlflow` (Guardian's decisions on MLflow traces), and `guardian-auto` only for
the install-once way.

**No versions in the skill.** The SDK is updated independently. The skill reads the wheels the customer downloaded: their
names and each wheel's `Requires-Dist` and `Requires-Python`. It builds the install line from them, and stops with a
clear message when a Collibra dependency wheel is missing. Never install from a source archive (`.tar.gz`): installing
one can run code.

### The SDK interface the skill relies on

Agree this list with the SDK owners, so a change to any item is flagged before a release:

- the extras names above;
- `collibra_sdk.guardian.injection.start_injection`;
- `import collibra_sdk.guardian.mlflow` as the switch for MLflow traces;
- the launcher module `collibra_sdk.guardian.launch`;
- the settings, read from `collibra-guardian.toml` in the working folder (parent folders aren't searched) or from
  `COLLIBRA_GUARDIAN_<SETTING>` variables, which win: `enforcement_mode` (`enforce` or `monitor`; `enforce` when unset),
  `policy_bundle`, `framework_version_policy` (`strict` or `permissive`; `strict` when unset), `engine_failure` (`flag`
  or `block`; `flag` when unset) and `rule_engine`. An unknown key is warned about and skipped;
- **a named rules file has to exist:** when `policy_bundle` names a file that isn't there, the agent doesn't start
  (`ConfigurationError`); with no `policy_bundle` and no `./bundle.zip`, Guardian allows everything and only logs
  `guardian.no_policy_bundle`;
- a rules file that is a zip holding `spec.yaml` (or that folder unzipped), whose `spec_bundle_id` is the file's id;
- Python 3.12 or later; the framework version check, which with `strict` stops the agent before it runs when LangChain
  or LangGraph is outside the tested range (in the documentation's SDK settings page);
- the lowest LangChain, LangGraph and `langgraph-prebuilt` versions in the `guardian-langchain` extra's `Requires-Dist`;
- `create_guarded_agent` and `create_guarded_deep_agent`, the constructors the skill offers when the customer doesn't
  want injection;
- the log lines `guardian.summarization_unguarded`, `guardian.injection_deep_agent_not_placed` and
  `guardian.placed_by_hand`, the trace field `collibra.guardian.run.unguarded`, and `collibra.guardian.purpose` on a decision.

## Attaching Guardian

The SDK offers six ways. Injection (ways 4 to 6) patches LangChain's `create_agent`, and from SDK 0.8 Deep Agents'
`create_deep_agent` too, so it covers both; a hand-built LangGraph `StateGraph` needs way 3. Injection places Guardian
as way 2 does, so it also checks the step that summarises a long conversation. Way 1 doesn't, and logs
`guardian.placed_by_hand`, so when the customer wants a way that changes the scan, the skill offers way 2.

| # | Way | Changes the agent's scan? |
|---|---|---|
| 1 | `GuardianMiddleware()` in `create_agent(middleware=[...])` | Yes: middleware is part of the scan |
| 2 | `create_guarded_agent(...)`, or `create_guarded_deep_agent(...)` for Deep Agents | Yes: the constructor is part of the scan |
| 3 | `guard_graph` and node wrappers | Probably; check before relying on it |
| 4 | `start_injection()` as the first line | No |
| 5 | The launcher, `python -m collibra_sdk.guardian.launch <file>` | No |
| 6 | The `guardian-auto` extra | No |

**Default: way 4 plus the MLflow import**, the first lines of the module the platform loads:

```python
from collibra_sdk.guardian.injection import start_injection
start_injection()                     # first, before anything imports create_agent
import collibra_sdk.guardian.mlflow   # puts Guardian's decisions on the MLflow traces
```

Cautions the skill handles: way 4 does nothing, with no error, if `create_agent` or `create_deep_agent` was imported
first, so the lines go at the very top. Way 6 never stops a process when it fails, so the agent can run unguarded
without anyone noticing. In both cases the skill checks from Collibra afterwards that the latest rules file is running.

## What changes in the customer's project

Shown as one diff before the customer's yes:

| File | Change | Why |
|---|---|---|
| `vendor/` (new) | The Collibra wheels needed | A deployed agent may not reach a private package index, so the wheels ship with it |
| `requirements.txt` or `pyproject.toml` | `--find-links ./vendor` and `collibra-sdk[guardian-langchain,guardian-injection,mlflow]` (with uv, a `[tool.uv.sources]` path entry) | The deployment installs the SDK from the shipped wheels |
| The entry module | The three lines above, at the very top | Guardian on, without changing the agent's scan |
| `collibra-guardian.toml` (new, in the folder the agent starts from) | `enforcement_mode = "<monitor or enforce, as the customer chose>"` only | What Guardian does when a rule is broken. The SDK's own default is `enforce`, so the skill always writes it. **No `policy_bundle` yet:** a path that names a missing file stops the agent, so the path is written in stage 07, together with the file |

The local environment gets the same packages with `pip install --find-links <folder> "collibra-sdk[...]"` (or uv).
Until the rules file exists, the SDK finds none and allows everything, which is harmless before the deploy, as long as
no setting names the file yet.

**Framework versions.** Before showing the diff, the skill checks the project's versions at both ends:

- **The lowest:** LangChain, LangGraph and `langgraph-prebuilt` against the lowest versions the wheel's
  `guardian-langchain` extra asks for. Installing the SDK moves the project up to them anyway, and fails on a pin below
  them, so the skill offers no choice and moves the requirements. SDK 0.8 raised LangChain's to 1.4.5, the LangChain
  release whose `SummarizationMiddleware` takes the `summarizer` the SDK checks the summarising step through. The tested
  range compares only major and minor, so 1.4.0 to 1.4.4 pass the range but not the extra.
- **The highest:** LangChain and LangGraph against the tested ranges in the documentation. The wheel names no highest
  version. Newer than the range, the agent wouldn't start, so the skill says so and offers (a) moving to a tested
  version, or (b) `framework_version_policy = "permissive"`, which only logs a warning.

A hand-built LangGraph graph isn't covered by ways 4 to 6, so the skill offers way 3 instead.

## When the rules go out of date, and how a new rules file reaches the agent

Rules are made for one agent version from the contracts that govern it at that moment. They go out of date ("stale") in
two cases, and in both the customer applies the latest rules again (stage 06) to get a new rules file:

| Cause | What changed | Status reported |
|---|---|---|
| The agent changed | A scan found different code, so Collibra made a new agent version, which has no rules yet | `staleBecause: SCAN_CHANGED`, or `NOT_COMPILED` for the new version |
| The rules changed | New rules in the global contract (applies to every agent), or in a specialised contract linked to the agent (applies only through that link); a contract linked or unlinked | `staleBecause: CONTRACT_VERSION_CHANGED` or `GOVERNING_SET_CHANGED` |

The "update an agent" path names the cause in plain words and offers to apply the rules again. Linking an AI use case
doesn't mark the rules stale, so after stage 05 the skill offers to apply them again itself.

The SDK has no automatic download. It reads the rules file once, when the agent's process starts: a `rule_engine`
setting (or `activate(rule_engine=...)`) means no rules file is read at all; otherwise the path in `policy_bundle`
(`COLLIBRA_GUARDIAN_POLICY_BUNDLE` wins over the file), which must exist; otherwise `./bundle.zip` in the working
folder, if it's there. A file that is there but can't be read stops the agent (`BundleError`). So a new rules file
reaches the agent in two moves:

1. **Replace the file at that path.** Stage 07 downloads the new file, checks it, and writes it to the path the settings
   already name, so nothing else changes.
2. **Restart the agent.** Locally, restart the process. A Databricks App: deploy it again with the new files. Model
   Serving: log, register and deploy a new model version with the file.

Afterwards the skill checks that the running agent uses it: after the next collection, the enforcement status's
`observedBundle` equals the latest rules file.

## Where the rules file lives, by how the agent runs

None of the ways the SDK finds a rules file changes the agent's scan: the `policy_bundle` setting (as a variable or in
`collibra-guardian.toml`) and `./bundle.zip` in the working folder aren't part of what the scanner records. What matters
is whether replacing the file needs a new deployment. **Rule: never name a path before the file is there**, so the
setting and the file are always written in the same step and shipped in the same deploy.

| Way | Supports replacing the file at a fixed path | The skill uses it |
|---|---|---|
| A `rule_engine` setting or `activate(rule_engine=...)` | No: the customer's own engine, no rules file | No |
| `COLLIBRA_GUARDIAN_POLICY_BUNDLE` | Yes | Yes, for Databricks |
| `policy_bundle` in `collibra-guardian.toml` | Yes | Yes, for local runs |
| `./bundle.zip` in the working folder | Locally only; unreliable on Model Serving, where the working folder isn't the model's folder | No |

| How the agent runs | Where the skill puts the file | A rules update means | Effect on agent versions |
|---|---|---|---|
| Locally, or from a notebook | `./collibra-guardian/bundle.zip`, and `policy_bundle` added to `collibra-guardian.toml` in the same step | Replace the file, restart the process | none |
| Databricks App | Unzipped in `policy/` in the App's source, and `COLLIBRA_GUARDIAN_POLICY_BUNDLE=policy` added to `app.yaml` in the same step | Replace `policy/`, deploy the App again | A new App deployment, not a new model version |
| Model Serving, file in the model | `bundle.zip` in `code_paths`, path from the module's `__file__` | Replace the file, log, register and deploy a new model version | A new model version, which a later scan reads as a new deployment (see decisions.md) |
| Model Serving, file outside the model (to verify) | `COLLIBRA_GUARDIAN_POLICY_BUNDLE` on the endpoint, pointing at a Unity Catalog volume | Replace the file in the volume, restart the endpoint | none |
| Deployed by a pipeline from git | Where the pipeline packages it | Replace the file, commit it; the pipeline deploys. Or the pipeline downloads the latest rules file at build time | As the pipeline deploys |

**Does the way Guardian is attached change this?** Only for two ways. All six find the rules file through the same
settings, so where the file goes and how a rules update reaches the agent depend on how the agent runs, not on the way.
Way 5 (the launcher): the deployment must keep the launcher as the start command (an App's `command` in `app.yaml`); it
can't be used on Model Serving. Way 6 (`guardian-auto`): the deployment's requirements must include the `guardian-auto`
extra and its package, and because way 6 never stops the agent when it fails, the check that the agent runs the latest
rules file is the only sign it worked.

## When the customer doesn't know how the agent is deployed

Never block on it. Find out, confirm in plain words, ask only if still unsure, and when nobody knows, finish everything
that doesn't depend on the deployment and hand the rest over.

1. **Find out, read-only, without asking:**
   - clues in the project: `app.yaml` (a Databricks App); `databricks.yml` (an asset bundle naming its apps, serving
     endpoints and jobs); `mlflow.pyfunc.log_model(...)` or `agents.deploy(...)` (Model Serving); a `ResponsesAgent` or
     `ChatAgent` class; a CI workflow running `databricks bundle deploy` or `databricks apps deploy` (a pipeline); a
     Dockerfile or Kubernetes files (another platform). The scanner's Stage 01 already collects platform hints; reuse
     them;
   - the platform, after the customer agrees and with their profile: `databricks apps list`,
     `databricks serving-endpoints list`, matched with the names found in the code (the scanner's Stage 04 look-up);
   - Collibra: an AI Agent or AI Endpoint already imported for this agent.
2. **Confirm** (header `Deployment`): "It looks like your agent runs as `<a Databricks App called X | a Model Serving
   endpoint called X>`. I found `<the clues>`. Is that right?" Options: **Yes (default)** / **No, it runs differently** /
   **I'm not sure**.
3. **If still unsure, ask about use, not deployment** (header `Agent use`): "How do people use this agent today?"
   **In a chat app in Databricks** / **Other systems call it through an endpoint** / **Only when someone runs it from code
   or a notebook** / **I don't know**. Then (header `Deploys`): "Who deploys changes to it?" **I do** / **A colleague** /
   **An automated pipeline** / **I don't know**.
4. **When nobody knows:**
   - do every stage that doesn't depend on the deployment: prepare the agent with the environment-variable setting (works
     locally, on an App and on Model Serving), the scan, contracts, use case, applying the rules, saving the rules file;
   - write a **hand-off note** for whoever deploys: what changed and which files to ship, where the rules file goes for an
     App, for Model Serving and for a pipeline, how to change what Guardian does when a rule is broken, and the agent's link in Collibra;
   - save the state, so the person who deploys can say "continue setting up Guardian" in their own session;
   - after the deploy, the status in Collibra shows whether the agent runs the latest rules file, so nobody has to
     understand the deployment to know Guardian is on.
5. **Other platforms** (outside the private preview): recognise the clues, do the platform-independent setup, and write
   the same hand-off note.

## Databricks

| | Databricks App | Model Serving endpoint |
|---|---|---|
| How it's identified | external id `databricks:<workspace id>:app/<app name>` | external id `databricks:<workspace id>:<catalog>.<schema>.<model>` |
| SDK wheels | In the App's source (`vendor/`) and its requirements | In the model's `code_paths`; `pip_requirements` lists `./code/<wheel>` |
| Rules file | Apps unzip every zip on deploy, so ship it unzipped in a folder and point the setting at the folder | `bundle.zip` in `code_paths`; build its path from the module's `__file__`, because the working directory isn't the model's folder. A wrong path fails quietly: every action is allowed |
| Mode | `COLLIBRA_GUARDIAN_ENFORCEMENT_MODE=<monitor or enforce>` in `app.yaml`, or the settings file | The same, as an endpoint environment variable |
| Traces | `MLFLOW_TRACKING_URI=databricks`, `MLFLOW_EXPERIMENT_ID` from the App's experiment resource | `ENABLE_MLFLOW_TRACING=true`, `MLFLOW_EXPERIMENT_ID` |
| Launcher (way 5) | Possible, in `app.yaml`'s `command` | Not possible: Databricks starts the model |
| Redeploy | `databricks apps deploy`, a few minutes | Log and register a new model version, then deploy it, up to half an hour |

The skill never deploys. It prepares the files and gives the steps for the platform; the customer or their developer
runs them.
