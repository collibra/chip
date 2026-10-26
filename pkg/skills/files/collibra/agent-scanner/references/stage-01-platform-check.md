# Stage 01: Platform check

Run this right after Stage 00 (intake), before the known-agent lookup. It finds out where the agent's telemetry goes and where the agent runs, for two reasons (`references/platform-router.md`, "What the platform is for"): the **route** becomes the prefix of the agent id, and the **host's CLI** can later fill in what the code leaves open. They are often the same platform and need not be: AWS hosting with MLflow tracing is route `mlflow`, host `aws`. It makes no network call: it only searches files and runs local commands. It asks the customer only when the route cannot be told (Step 3).

## Step 1: Use the output folder

Stage 00 created the output folder and `intake.json` is in it. Write everything this stage produces there.

## Step 2: Search for hints

Search the scan root once for the hints in `references/platform-router.md` (Step 1 there lists them by route): imports and manifests, the pipeline and infrastructure files, serverless runtimes, tracing and telemetry libraries. One bounded search, file names and matching lines only; read nothing in full. A hint is evidence, not proof: keep the file and line it came from.

The scan root is the folder you work in, or the one the customer named in their opening message. If it is not readable, say so and stop.

## Step 3: Decide the host and the route

Apply "Choosing the host and the route" in the router. The **host** is the hosting platform whose hints you found (`databricks`, `aws`, `azure`, `gcp`, `snowflake`), or none. The **route** is the telemetry-only route whose hints you found (`otel`, `mlflow`, `langfuse`) if there is one, otherwise the host's own route, otherwise `custom`; with the Databricks exception the router describes. If several candidate routes are left that the rules do not separate, ask the customer now, in one form question, "Where does this agent's telemetry arrive?", with the candidates and `custom` as options (the first candidate is the default). The route is one of `aws`, `azure`, `gcp`, `snowflake`, `databricks`, `otel`, `mlflow`, `langfuse`, `custom`; nothing else.

## Step 4: Offline facts and the host's CLI

For a host that has a platform file (today Databricks), read the facts that file lists under "Offline facts": model name, endpoint name, app name, workspace host. One search in the files it names.

For the host, if there is one, check that its CLI is installed with its version command (`databricks`, `aws`, `az`, `gcloud`, `snow`) and call nothing else. This is what Stage 04 may use later to read the deployed configuration. A route without a host (`otel`, `mlflow`, `langfuse`, `custom`) has no CLI.

For a cloud host without a platform file (`aws`, `azure`, `gcp`, `snowflake`), also read the instance and the resource id from the literals the router lists under "Finding the parts of a cloud id". Keep only a literal, never a variable or a secret reference, and keep the file and line.

## Step 4b: Ask for the parts of a cloud id that are still missing

Only when the route is a cloud route (`aws`, `azure`, `gcp`, `snowflake`) that is also the host, and the instance or the resource id was not found in the files. Do not ask when a known agent will settle it (Stage 02 finds an agent by the route and the resource name; if you cannot tell yet whether it exists, ask now). Ask in one form question: "The id of an agent on <cloud> is `<route>:<instance>:<resource>`. I could not find the <missing part> and I cannot look it up yet. What should I do?" with the options "Use a provisional id built from the repository (default)" and "I will give it to you" (they type it in the field for other answers). A provisional id is `<route>:<repository address>#<agent name>`, with a warning. Never guess a part.

## Step 5: Write `platform.json` and say what you found

Write `platform.json` in the output folder:

```json
{ "scanRoot": "...", "repositoryAddress": "...",
  "route": "mlflow", "routeCandidates": ["mlflow"],
  "host": { "name": "aws", "hasPlatformFile": false,
            "cli": { "name": "aws", "available": true } },
  "evidence": { "route": ["agent/agent.py:6"], "host": ["infra/main.tf:1", ".github/workflows/deploy.yml:15"] },
  "idParts": { "instance": null, "resource": null },
  "offlineFacts": {} }
```

`idParts` holds the instance and the resource id of a cloud route when they are known (from the files or from the customer), with `null` for a part that is not. `host` is null when no hosting platform was found. `route` is `custom` and `routeCandidates` empty when nothing was found; `route` is null and `routeCandidates` lists the choices when the customer has to decide. For a Databricks agent the host and the route are both `databricks`. Never assume a platform.

Say what you found in one line, and only if there is something: "This looks like it runs on <host> (<file>) and its telemetry goes through <route>, so the id will start with `<route>:`", and for a host without a platform file add "I cannot look up its deployment yet." Say nothing when the route is `custom` and there is no host.

## Exit criteria

- `platform.json` is in the output folder, with a route.
- Nothing was sent and nothing in the repository changed; the customer was asked only if the route was ambiguous.
