# Platform: Databricks

Load this from the platform router when the code shows the agent runs on Databricks.

## 1. Offline facts (from the code; no network)

Look in the deploy script, `databricks.yml`, `app.yaml`, `pyproject.toml`, the agent file, and the pipeline and infrastructure files the router lists (a workflow that runs `databricks bundle deploy -t <target>` or `agents.deploy`, a Terraform `databricks_registered_model` or `databricks_model_serving`).

| Fact | Where to find it |
|---|---|
| Unity Catalog model name, `catalog.schema.model` | A constant or argument in the deploy script (`registered_model_name`, `UC_MODEL_NAME`, a `models:/catalog.schema.model` address) |
| Serving endpoint name | A constant in the deploy script. Agents deployed with the Databricks agent tooling are usually named `agents_<catalog>-<schema>-<model>` (dots replaced by dashes); use that only if the script shows it |
| Databricks App name | `databricks.yml` under `resources.apps`, or `app.yaml` |
| Workspace address | `workspace.host` in `databricks.yml`. Environment variable names are not addresses: do not read their values |
| Deploy target | In a pipeline: the `-t` / `--target` of `databricks bundle deploy`, the environment name, the trigger. In Terraform: the `name` of `databricks_model_serving` and the registered model's catalog, schema and name. Record the trigger and environment in a warning only; they do not give the live version |
| Model | `ChatDatabricks(endpoint=...)`: an endpoint name, recorded as the model id (see Stage 05) |

Record `platform.name` = `DATABRICKS`, `platform.metadata.ucModelName` when you found the model name, and `platform.metadata.workspaceId` (a string) once the workspace id is known.

**Agent id.** The prefix of an id says where the agent's telemetry goes; `databricks:` routes it through Databricks. For a Databricks agent the id is a URN: `databricks:<workspace_id>:<model_full_name>`, for example `databricks:1234567890123456:example_catalog.agents.support_triage_agent`. `workspace_id` is the numeric workspace id and `model_full_name` is the Unity Catalog three-part name. Use it unless the customer gave an id. An id must never contain `>`, be blank, or have leading or trailing whitespace.

Finding the workspace id, in this order:
0. The known agent's platform metadata (`known.json`, Stage 02): use it, do not ask or look it up again.
1. The customer gave it.
2. Live exploration (below): `databricks metastores current` returns the workspace id of the profile's workspace (confirm the subcommand and field with `-h`).
3. The workspace address, if it has the form `adb-<digits>.<n>.azuredatabricks.net` (the digits are the id). Other address forms do not contain it.
4. Otherwise ask the customer once: "What is the numeric ID of your Databricks workspace? You can find it in the workspace URL after `?o=`." Never guess it and never use another id in its place.

If the code shows no model name, say so in a warning and use the default id.

## 2. Live exploration (optional, read-only)

It makes the fingerprint describe what is actually deployed. Do it only if **all** of these hold:

1. `command -v databricks` finds the CLI, and `databricks --version` is 1.0.0 or newer. If not, skip this section and add the warning "deployment details not looked up: Databricks CLI not available".
2. The customer agreed in Stage 04's form (the look-up question). Stage 04 asks it, with the profile choice, before this section runs; the wording it uses is: "This agent runs on Databricks and I can look up how it is deployed (which model version is live, its endpoint and configuration) using your Databricks CLI. I will only read, and I won't call the agent. May I?"
3. The customer chose a profile. Run `databricks auth profiles`, show **all** profiles with their workspace addresses, and let the customer choose (with a question tool, one option per profile showing its name and address plus a final option "Do not look anything up", which also keeps the question at two options when there is only one profile; with more than three profiles, list them in text). **Never choose one yourself, even if there is only one.** Put `--profile <name>` on every command (each command runs in its own shell).

If the customer says no, or a call fails with a sign-in or permission error, stop live exploration, keep the offline facts, and warn what is missing. Do not retry. Tell the customer to sign in again (`databricks auth login --profile <name>`) if they want to try later.

### Allowed commands (read-only)

Checked against Databricks CLI v1.15.0 (`-h`): these subcommands and their positional arguments exist. Names and fields can differ in other versions, so check with `-h` if one is missing.

- **Ask for JSON:** the CLI prints text by default. Add `-o json` to every look-up (`--profile <name>` can also be written `-p <name>`).
- **Aliases are not shown by default:** add `--include-aliases` to `registered-models get` and `model-versions get` to see aliases such as `champion` or `prod`.

- `databricks auth profiles --skip-validate`: lists the profiles and their addresses without contacting any workspace. Without `--skip-validate` the CLI signs in to every profile to check it, which would reach workspaces the customer has not chosen.
- `databricks metastores current`: the workspace id and the metastore of the profile's workspace.
- `databricks registered-models get <catalog.schema.model> --include-aliases`: the full name, the metastore, the aliases.
- `databricks model-versions list <catalog.schema.model>` and `model-versions get <catalog.schema.model> <version> --include-aliases`: version numbers, the run each came from, the status.
- `databricks experiments get-run <run_id>` (or the equivalent): the run's tags, in particular the git commit (`mlflow.source.git.commit`) and repository address (`mlflow.source.git.repoURL`).
- `databricks serving-endpoints get <endpoint>` and `serving-endpoints list`: the served entities (model and version), the traffic split, the state. To find the endpoint without a name, list and pick the ones whose served entity is the model.
- `databricks apps get <app>`: the app address.
- `git status --porcelain`, `git rev-parse HEAD`, `git cat-file -e <commit>`, `git diff --stat`, `git archive`, and `git fetch` only if the customer agrees.

**Never run:** anything that creates, updates, deletes, deploys, runs a job, starts or stops compute, or queries a model or serving endpoint; anything under `databricks experimental`; `databricks auth token` or anything that prints a credential.

### Deployment check (current version and address only)

Used by Stage 02 to see whether an agent whose code did not change has been redeployed. Read-only, with the profile the customer chose: `databricks serving-endpoints get <endpoint> -o json`, where `<endpoint>` is the stored `deployment.endpoint.name` (if the stored scan has none, find it with `serving-endpoints list` by the model name). From it take: the version served, the `entity_version` of the served entity with the most traffic (the `traffic_config` routes give the shares); the endpoint's `name`; and the address `https://<workspace host>/serving-endpoints/<name>/invocations`. Compare the version and the address with the stored `deployment.version` and `deployment.endpoint.url`. Nothing else is read in this check, and the model-version, run and commit look-ups below are not run.

### Deployed configuration (fills the gaps the code leaves open)

When the code takes a value from an environment variable (for example `os.environ.get("DATABRICKS_LLM_ENDPOINT", ...)` for the model), the deployed value is in the serving endpoint's configuration: `databricks serving-endpoints get <endpoint> -o json` returns it in `config.served_entities[].environment_vars`, a map from variable name to value (`config.served_models[]` repeats it). Take the served entity whose `entity_version` is the chosen version `V`.

Write `deployed-config.json` in the output folder: `{ "source": "<endpoint> version <V>", "values": { "<NAME>": "<value>" }, "setFromSecret": ["<NAME>"] }`. Rules for the values:

- Keep a value only for a variable name the code reads (search the scan root for each name); leave the rest out.
- A value that is a secret reference (`{{secrets/<scope>/<key>}}`), or a variable whose name suggests a secret (`TOKEN`, `KEY`, `PASSWORD`, `SECRET`, `CREDENTIAL`), or a value that looks like a credential, goes in `setFromSecret` without its value (H7). Never print it.
- Stage 05 uses these values to resolve what the code left open and says in a warning where each came from.

### Which deployed version

1. List the served entities of the endpoint for this model, with their traffic percentages.
2. One version serving: use it. Several: ask which to scan. Default: the one with the most traffic. If the customer wants several, scan them one after another and publish them in ascending traffic order, so the main one is published last.
3. No endpoint serves the model: use the version an alias such as `champion` or `prod` points to, if there is one, and say so. Otherwise skip to "No deployed version found".

### Check the working copy before any comparison

If the scan root is in a git repository, run `git status --porcelain` (it includes untracked files). If the output is not empty, show this and wait for an answer:

> ⚠️ **UNCOMMITTED CHANGES.** `<repository>` has `<n>` modified or untracked files (first ten: `<list>`). Comparing or scanning the working copy would include code that no commit contains, and the commit I record (`<short HEAD>`) would not describe it.
> 1. Stop, so you can commit or stash.
> 2. Scan the deployed code instead (this ignores your working copy).
> 3. Continue anyway.
>
> Reply 1, 2 or 3. I will not change your repository.

Only an explicit answer continues. Re-check just before comparing, because the state can change during the run.

### Match the deployed version to the code

Let `V` be the chosen version and `C` its git commit (from the run's tags).

1. **Same commit.** If `C` equals the current commit (`git rev-parse HEAD`) and the working copy is clean (or the customer chose 3): the code you scanned is what is deployed. Scan the working copy. Set `deployment.version` = `V`.
2. **Different commit.**
   - If `C` is not in the clone (`git cat-file -e C^{commit}` fails), offer `git fetch`. If it still is not there, go to step 3.
   - Show `git diff --stat C HEAD` restricted to the agent's own files (the ones in the extraction's evidence paths and the files they import). If there are no differences in those files, the deployed code and the current code define the same agent: scan the working copy and set `deployment.version` = `V`, noting the unchanged result in a warning.
   - If they differ, the default is to **scan the deployed code**: extract commit `C` with `git archive C | tar -x -C <output folder>/deployed-<V>` (this never touches the repository), and run Stages 03, 05 and 06 on that folder. Offer the working copy as the alternative; a working-copy scan is then recorded without `deployment.version` and the review says it is not what is deployed.
3. **No commit recorded, or it cannot be found.** Look at the model version's logged artifacts. Models logged from code carry the agent file and its code paths. If the artifact listing shows the agent file, download it read-only into `<output folder>/deployed-<V>/` (for example with `databricks fs cp -r`) and scan that. There is no commit then, so `source` is left out and a warning says so.
4. **No deployed version found** (no endpoint, no alias, or every look-up refused): scan the working copy, leave `deployment` out, and warn: "not checked against any deployed version".

### When the deployed code is scanned

- The scan root for Stages 03, 05 and 06 is `deployed-<V>`. Evidence paths are relative to it.
- The extraction carries `source`: `repoUrl` (from the run's tag, else from the remote) and `commitSha` = `C`. The finalize step must not use `HEAD`.
- The review tells the customer: "Scanned the code deployed as version `V` (commit `C`)", the `git diff --stat C..HEAD` summary against the working copy, and that the working copy can be scanned afterwards as a separate scan.
- A working-copy scan later under the same agent id, without a `deployment`, is a separate version of the same agent (its document hash differs). That is expected.

### Earlier versions (opt-in)

Only when the earlier-versions question in Stage 04 was answered "all" or a number, live exploration is allowed, and a deployed version `V` was chosen. Otherwise skip this section; the default is the deployed version alone.

1. **Pick the versions.** From `model-versions list`, take every version lower than `V` (or the last `n` of them). More than 10: say how many there are and ask once whether to go on with all, or fewer.
2. **Read each one's commit** with `experiments get-run` (same tags as above). Group the versions by the content of the agent's files, not by commit: after the first extraction, take the files it cites as evidence plus the dependency manifests and lock files, and for each commit read the blob ids of those paths (`git rev-parse <commit>:<path>`). Versions with identical blob ids share one extraction and differ only in `deployment.version`; when only some files differ, re-read only those.
3. **Get the code for each commit** as in "Match the deployed version to the code": if it is the current commit and the working copy is clean, scan the working copy; otherwise `git archive <commit>` into `<output folder>/deployed-<commit short>`, offering `git fetch` once if the commit is missing. A version with no commit that cannot be recovered from its artifacts is skipped, with a warning naming the version.
4. **Finalize one document per version**: the same agent id, `deployment.version` = that version, `source` = that commit and repository, and no `deployment.endpoint` (only the version being served has one). Each carries the warning "earlier version of the model, not the one currently served".
5. **Every version is sent.** The deployment's `version` is part of the document hash, so each version is its own state and its own version in Collibra, even when the code did not change between them. Send every version, none dropped. Say in the review how many versions are sent, and that versions with unchanged code differ only in their deployment.
6. **Order.** Collibra numbers an agent's versions in the order they arrive. Publish the earlier versions oldest first and the version `V` last, so version numbers follow the real history.
7. **What Collibra already has.** Stage 02, when its tools are offered, reads the newest state Collibra holds, not the version line, so it cannot say which older states are present. Never claim an older version is already there; sending a state Collibra already holds only confirms it.

Stage 07 covers all documents with one summary and one consent question.

## 3. What to record

| Result | Value |
|---|---|
| `platform` | `{ "name": "DATABRICKS", "metadata": { "ucModelName": ..., "workspaceId": ..., "mlflowRunId": ... } }`. Include `mlflowRunId` only if you read it from the chosen version |
| root agent `deployment.version` | The chosen version `V` as a string, only when the scanned code is the code of that version (cases 1 and 2 above, or the deployed code scanned) |
| root agent `deployment.endpoint` | `url`: `https://<workspace host>/serving-endpoints/<endpoint name>/invocations` for a serving endpoint, or the app address from `apps get`; `name`: the serving endpoint's name (or the app's). Only when the live look-up confirmed the endpoint exists, and the host and the name are both known. Never from the code alone. The url never carries credentials |
| `source` | Only when scanning deployed code: `repoUrl` and `commitSha` of `C` |
| agent id | `databricks:<workspace_id>:<model_full_name>` unless the customer gave one |
| root agent `name` | The last part of the model name (`example_agent` for `example_catalog.example_schema.example_agent`), the same for every version of the model. Never the name of a variable in the code: it can change between versions and it names the catalog asset. If it is shorter than 3 characters, use the model name's last two parts joined with `_`. Add a warning with the name the code uses (for example "the code calls this agent `AGENT`"). Sub-agents and unused agents keep their code names. When the model name is not known, use the name from the code |

Warnings to add where they apply: the profile and workspace used (never credentials); which version was chosen and why; the traffic split seen; a look-up that was refused or missing; that the deployed code was scanned instead of the working copy; the uncommitted-changes answer.

## 4. Without live exploration

Record the offline facts only. Leave `deployment` out unless the customer gave its parts. Add one warning saying deployment details were not looked up and why.
