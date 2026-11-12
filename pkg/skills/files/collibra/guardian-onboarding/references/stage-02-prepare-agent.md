# Stage 02: Prepare the agent (step 1 of 8)

Install the Guardian SDK in the agent's project and wire it in, before the scan, so the version the rules are made for already includes Guardian. Local commands only; no tools from this Collibra connection. All paths are under the agent's absolute project path from Stage 01.

Skip this stage when the client can't run code or the customer has no code (the hand-off note covers it), and when the project already has the SDK wired in as below (check it, say so, and go on).

## Step 1: How the agent runs and is deployed

Never block on this. Find out, confirm in plain words, ask only if still unsure.

1. **Find out, read-only, without asking,** opening only the files that show it, never the whole repository: `app.yaml` (a Databricks App); `databricks.yml` (an asset bundle naming apps, serving endpoints and jobs); `mlflow.pyfunc.log_model(...)` or `agents.deploy(...)` (Model Serving); a `ResponsesAgent` or `ChatAgent` class; a CI workflow running `databricks bundle deploy` or `databricks apps deploy` (a pipeline); a Dockerfile or Kubernetes files (another platform); an AI Agent or AI Endpoint Collibra already has for this agent. A live look-up on the platform (`databricks apps list`, `databricks serving-endpoints list`) needs the platform tool: follow `shared/platform-prerequisites.md` first, and only if the clues aren't enough.
2. **Confirm** (header `Deployment`): "It looks like your agent runs as `<a Databricks App called X | a Model Serving endpoint called X>`. I found `<the clues>`. Is that right?" **Yes (default)** / **No, it runs differently** / **I'm not sure**.
3. **If still unsure, ask about use, not deployment** (header `Agent use`): "How do people use this agent today?" **In a chat app in Databricks** / **Other systems call it through an endpoint** / **Only when someone runs it from code or a notebook** / **I don't know**. Then (header `Deploys`): "Who deploys changes to it?" **I do** / **A colleague** / **An automated pipeline** / **I don't know**.
4. **When nobody knows:** carry on with everything that doesn't depend on the deployment (this step, the scan, contracts, use case, applying the rules, saving the rules file), and step 6 ends with a hand-off note for whoever deploys. Outside the private preview platforms, recognise the clues, do the same platform-independent setup, and write the same note.

Record what you found: local or notebook, Databricks App, Model Serving, a pipeline, another platform, or unknown.

## Step 2: The Guardian SDK files

- **The SDK is already in the project** (Collibra wheels in `vendor/`, or `collibra-sdk` installed in the project's environment): use it and ask nothing about it. Say one line: "Using the Guardian SDK `<version>` already in your project." Take the version from the wheel's name, or from `importlib.metadata.version("collibra-sdk")` run with the project's environment. If the agent is deployed on Databricks, which needs the wheel files, and the project has only the installed package, ask for the folder as below.
- **No SDK in the project:** ask: "Where did you save the Guardian SDK files Collibra shared with you?" (free text; a folder path).
- No files yet: "I need the Guardian SDK files Collibra shared with you to continue. Download them, tell me the folder, and type "continue"." Pause. Install nothing.
- Read the wheels (in `vendor/`, or in the customer's folder): their names and, from each wheel's metadata, `Requires-Dist` and `Requires-Python`. Never hard-code a version; build everything from what the files say. Expect `collibra_sdk`, `collibra_policy_enforcement` and `collibra_guardian_contract` wheels (`collibra_guardian_auto` only for the install-once way). If a Collibra wheel that another one depends on is missing, stop and say which.
- Never install from a source archive (`.tar.gz`): installing one can run code. Only `.whl` files.

## Step 3: Python and the agent framework

- **Python:** the project's environment must be Python 3.12 or later. If not, help, as `shared/platform-prerequisites.md` "Python 3.12 or later" says: "Your project uses Python `<version>`, and the Guardian SDK needs 3.12 or later." Then, when a newer Python is already on the computer: "Python `<3.12>` is already installed. I'll set up your project's environment with it as part of the changes." When none is: "I can install Python 3.12 with `<command>`, or write a message for your IT team. Shall I install it?" Go on with the step only when the project has Python 3.12 or later.
- **Framework versions, the lowest:** compare the project's LangChain, LangGraph and `langgraph-prebuilt` versions with the lowest version of each that the `collibra_sdk` wheel's `guardian-langchain` extra asks for (its `Requires-Dist`). Installing the SDK moves the project up to those versions anyway, and fails if the requirements pin an older one, so offer no choice: "Your agent uses `<LangChain 1.3>`, and Guardian needs `<1.4.5>` or later. I'll move your project to `<1.4.5>` as part of the changes." The new versions go in the requirements, in the changes shown in step 5.
- **Framework versions, the highest:** compare the project's LangChain and LangGraph versions with the tested range on the supported frameworks page (`references/documentation.md`). The wheel names only the lowest versions, so if you can't open the page, say you couldn't check the highest; never guess one. Newer than the range, before showing any change: "Your agent uses `<LangChain 2.1>`, and Guardian is tested with `<1.4 up to 2.0>`. As it is, your agent wouldn't start with Guardian. I can (a) set your project to a tested version, or (b) let it start anyway and only log a warning. Which would you like?" (a) changes the version in the requirements; (b) adds `framework_version_policy = "permissive"` to the settings file. Both are part of the changes shown in step 5.
- **How the agent is built:** `create_agent` (LangChain) or Deep Agents use the default below. A hand-built LangGraph `StateGraph` isn't covered by it: offer the graph wrappers instead (`guard_graph` and node wrappers), and say they will probably show up in the agent's scan.

## Step 4: The mode

Header `Mode`: "When Guardian finds your agent about to break a rule, what should it do?"

- **Monitor: record it only** (offered first): "Your agent carries on as normal, and Collibra shows what Guardian would have stopped. A safe way to start and to check your rules."
- **Enforce: stop it:** "Guardian applies your rules: it blocks the action, or changes it (for example, hides sensitive data), as your rules say. Your users see the effect straight away."

Then: "You can change this later by asking me, or in the settings file." Neither is labelled "(default)": the customer chooses. Always write the choice as `enforcement_mode` (`monitor` or `enforce`), because the SDK stops actions when the setting is missing.

## Step 5: Show the changes

Prepare one diff with every change, and the install command, and show them together:

| File | Change |
|---|---|
| `vendor/` (new) | The Collibra wheels needed, copied from the customer's folder, so a deployed agent doesn't need a private package index |
| `requirements.txt` or `pyproject.toml` | `--find-links ./vendor` and `collibra-sdk[guardian-langchain,guardian-injection,mlflow]` (with uv, a `[tool.uv.sources]` path entry instead). For Model Serving, the wheels go in the model's `code_paths` and `pip_requirements` lists `./code/<wheel>` |
| The entry module (the module the platform loads) | The lines below, at the very top, before anything imports `create_agent` or `create_deep_agent` |
| `collibra-guardian.toml` (new, in the folder the agent starts from) | `enforcement_mode = "<monitor or enforce>"`, and `framework_version_policy = "permissive"` only if the customer chose (b) above. **Nothing that names a rules file:** a setting that names a missing file stops the agent. The rules file and its setting are added together in step 6 |
| `collibra-guardian/setup-state.json` (new) | The progress file: ids only |
| Databricks App | `COLLIBRA_GUARDIAN_ENFORCEMENT_MODE` and the trace variables (`MLFLOW_TRACKING_URI=databricks`, `MLFLOW_EXPERIMENT_ID` from the App's experiment resource) in `app.yaml`'s environment, if they aren't there |
| Model Serving, in the deploy script | `environment_vars={"COLLIBRA_GUARDIAN_ENFORCEMENT_MODE": "<monitor or enforce>"}` in the `agents.deploy(...)` call, because `collibra-guardian.toml` isn't deployed with the model and without the variable the endpoint enforces. `mlflow.set_experiment(...)` before that call, unless the script already sets an experiment: use `/Shared/<agent name>`, so Guardian's decisions have somewhere to go. `agents.deploy` turns on `ENABLE_MLFLOW_TRACING` itself; a script that deploys another way also sets `ENABLE_MLFLOW_TRACING=true`. A pin of `mlflow` below 3.15 moves to 3.15 (the SDK's `mlflow` extra needs it). No deploy script: these go in the deploy steps in step 6, not in a question |

The default way to switch Guardian on, which doesn't change the agent's scan:

```python
from collibra_sdk.guardian.injection import start_injection
start_injection()                     # first, before anything imports create_agent
import collibra_sdk.guardian.mlflow   # puts Guardian's decisions on the MLflow traces
```

These lines must be the very first lines: if `create_agent` or `create_deep_agent` was imported before them, Guardian does nothing and shows no error. Say so in one sentence when you show the diff.

Other ways exist (middleware, a guarded constructor, the launcher, the install-once extra). Offer one only when the customer asks or the default can't work, and say that middleware and a guarded constructor change the agent's scan. Of those two, offer the guarded constructor (`create_guarded_agent`, or `create_guarded_deep_agent` for Deep Agents): middleware placed by hand leaves the step that summarises a long conversation unchecked. Also say that the launcher can't be used on Model Serving, and the install-once way never stops the agent when it fails, so the check in step 7 is the only sign it worked.

The install command for the project's environment (shown, not run yet):

```text
pip install --find-links <the SDK folder> "collibra-sdk[guardian-langchain,guardian-injection,mlflow]"
```

(or the uv equivalent), run with the project's own environment, never the system Python. If the project has no environment, propose creating one under the project path as part of the same change.

For a Git repository copied read-only in Stage 01, prepare the same changes as a patch file for its owner instead.

Ask, word for word: "Shall I install the Guardian SDK and make these changes to your project? I'll change nothing until you say yes."

## Step 6: Make the changes and check them

After the customer's clear yes to that question, for exactly what was shown: make the changes, run the install, write the state file. Then check (`references/verify-and-diagnose.md`):

- the changes on disk are exactly the diff shown;
- the packages import in the project's environment (`python -c "import collibra_sdk.guardian.injection"`), without importing the agent;
- `collibra-guardian.toml` parses (`tomllib`), has `enforcement_mode` as chosen, and names no rules file;
- the entry module still parses: `compile()` it in memory, never run or import it. So does a changed deploy script, and it sets the mode variable.

## Exit

"Step 1 of 8 done: the Guardian SDK `<version, from the installed package>` is installed. Guardian has no rules yet, so for now it lets everything through. Once your rules file is in place (step 6), when your agent breaks a rule, Guardian will `<only record it | stop it>`. Changed: [`collibra-guardian.toml`](<absolute path>)`<, and each other changed file, linked the same way>`." Nothing is in Collibra yet, so the links are the changed files in the project. Then "Learn more:" and the SDK settings page and the page on how Guardian works and its modes (`references/documentation.md`).

Exit when the SDK is installed and wired in and the checks pass, or the customer chose to stop. Without the SDK files the step pauses for "continue".
