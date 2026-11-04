# Stage 07: Save the rules file, and deploy (step 6 of 8)

Download the agent's latest rules file in the customer's environment, check it is the file Collibra made, put it where the Guardian SDK reads it together with the setting that names it, and give the deploy steps. You never deploy.

**Tools:** `get_agent_rules_file`. Local commands.

**Before** (by depth):
- New: "Step 6 of 8: save the rules and deploy. Collibra has made a rules file: the checks for your agent in one file. I'll save it where the Guardian SDK in your agent reads it, and check it's exactly the file Collibra made. Your agent reads it when it starts, so after this you deploy or restart the agent, and from then on Guardian applies your rules."
- Knows Collibra: the same, the first time "rules file" comes up; otherwise "Step 6 of 8: save the rules file and deploy."
- Uses Guardian: nothing before the step.

**Before:** "I'll save your agent's rules file to `<path>`, where the Guardian SDK will find it, and check it's the file Collibra made."

If the client can't run code or the customer has no code, skip to step 6 (the hand-off note), with the rules file's id and the agent's link.

## Step 1: Get the file's details

Get the rules file only through `get_agent_rules_file`: never ask the customer to install or run the Collibra CLI for it. If the tool isn't available, that is the "Guardian tools missing" block from step 0, not a reason to use the CLI. Call `get_agent_rules_file` with the AI Agent id. It returns the file's id, version, agent version, size, SHA-256 (`contentHash`), when it was made, and a short-lived download link.

- **No rules file yet** (`noBundleAvailable`): back to step 5.
- The file's `agentVersionId` must be the agent's newest version, and its id must equal `compilation.latestBundle.id` from `get_agent_guardian_status`. If not, say what you found and go back to step 5.

**The download link is a secret.** Use it once, only inside the download command. Never show it in a message, never write it to a file, a script on disk, the state file or a note, and never print it in command output. If it expires, get a fresh one with `get_agent_rules_file`.

## Step 2: Where the file goes, and one yes

The rules file is always downloaded into the agent's project, so the customer can find it later: never into a
temporary, scratch or session folder. Without a project (no code), use `collibra-guardian/` in the folder the customer
started in, and say its full path.

Never name a path before the file is there: the file and the setting that names it are written in the same change and
shipped in the same deploy. A setting that names a missing file stops the agent.

| How the agent runs | The file is downloaded to | Then, in the same change |
|---|---|---|
| Locally, or from a notebook | `<project>/collibra-guardian/bundle.zip` | `policy_bundle = "collibra-guardian/bundle.zip"` in `collibra-guardian.toml` (the path is relative to the folder the agent starts from) |
| Databricks App | `<project>/collibra-guardian/bundle.zip` | Unzipped into `policy/` in the App's source folder (Apps unzip every zip on deploy, so ship the folder, not the zip), and `COLLIBRA_GUARDIAN_POLICY_BUNDLE=policy` in `app.yaml`'s environment |
| Model Serving | `<project>/collibra-guardian/bundle.zip` | Copied into the model's `code_paths`, with the path built from the entry module's `__file__`, because the model's working folder isn't its code folder. A wrong path fails quietly: every action is allowed |
| Deployed by a pipeline from git | `<project>/collibra-guardian/bundle.zip` | Where the pipeline packages files, as the customer or their developer says, with the setting for the platform it deploys to |
| Nobody knows | `<project>/collibra-guardian/bundle.zip` | No setting: the hand-off note says where it goes for each platform |

If the setting already names this path (an update), only the file is replaced; nothing else changes.

Show the change: the full path the file is downloaded to, where it is placed for the platform, and the exact line added
to the settings. Then ask: "Shall I download the rules file to `<full path>`, check it, and add the setting that points
to it? I'll change nothing until you say yes."

## Step 3: Download and check

After the customer's yes, download straight to that path with the project's Python (or curl and shasum). The command
prints only the size and the SHA-256:

```python
import hashlib, sys, urllib.request
url, out = sys.argv[1], sys.argv[2]
with urllib.request.urlopen(url, timeout=120) as r, open(out, "wb") as f:
    f.write(r.read())
data = open(out, "rb").read()
print(len(data), hashlib.sha256(data).hexdigest())
```

Then check, and use the file only if every check passes:

1. **SHA-256** equals `contentHash`, and the size equals `sizeInBytes`. If not, download once more with a fresh link. Twice wrong: delete the file, then "The file doesn't match what Collibra made, so I've removed it and won't use it. Please share the details below with Collibra support." Load `references/support-details.md` and stop.
2. **The archive holds `spec.yaml`**, and its `spec_bundle_id` equals the file's id (read it with the project's Python and `yaml.safe_load`; pyyaml comes with the SDK). No `spec.yaml`: delete the file; it's an old rules file, so apply the rules again (step 5).
3. **The SDK's own loader accepts it:** load the file with the rules-file loader in the installed Guardian SDK packages, in the project's environment. Run the SDK on the file only, never the agent. If you can't find the loader, say this check was skipped.

A download that fails: "The download didn't complete, so I'll try once more." Then once more with a fresh link, and
delete any partial file.

When every check passes, place the file for the platform and write the setting, as shown in step 2. Then check that the
file at its final place has the same SHA-256 (for the unzipped folder: that `policy/spec.yaml` is there with the right
`spec_bundle_id`), that the settings file still parses and names exactly that path, and that nothing else changed.
Write the rules file id to the state file. Tell the customer the full path where the file is.

## Step 4: The deploy steps

"To start checking, deploy your agent again: `<steps for the platform>`. Guardian reads the rules file when your agent starts."

| How the agent runs | Steps |
|---|---|
| Locally, or from a notebook | Restart the agent's process |
| Databricks App | Upload the App's source with the new `policy/` folder and settings, then `databricks apps deploy`. A few minutes |
| Model Serving | Log and register a new model version with the file in `code_paths`, then deploy it to the endpoint. Up to half an hour. This makes a new model version; don't send a new scan only because of it |
| A pipeline | Commit the changed files; the pipeline deploys |

Before giving commands for the platform's command-line tool, follow `shared/platform-prerequisites.md`: check the tool, offer to install it only if it's missing, have the customer sign in and choose the profile. Give the exact commands with the chosen profile. The customer runs them (in Claude Code, typing `! <command>` runs it in the session). Without the tool, give the steps in the platform's web interface, or put them in the hand-off note for whoever deploys.

For an agent that runs where the customer has the Collibra CLI, add: "The Collibra CLI can download a newer rules file before each start: `collibra ai-gov guardian sync`." with "Learn more:" and the page on downloading the latest rules file with the CLI, and the page on installing the Collibra CLI (`references/documentation.md`).

## Step 5: Until the deploy

Say: "Until you deploy, your agent runs as before. When it starts with the rules file, Guardian checks it."

## Step 6: The hand-off note, when nobody knows how it's deployed

Write it to `<project>/collibra-guardian/handoff.md` after the customer agrees (covered by the same yes if it was part of the change shown, otherwise ask), or show it in the conversation. It holds no secrets and no download link:

- what changed in the project and which files to ship;
- where the rules file goes and which setting names it, for a Databricks App, for Model Serving and for a pipeline (the table in step 3);
- how to change what Guardian does when a rule is broken (`enforcement_mode`, `monitor` or `enforce`);
- for a customer with no code: how to prepare the agent (step 1) and that the scan is needed;
- the agent's link in Collibra, and that saying "continue setting up Guardian" in their own session picks up from the state file.

## Exit

"Step 6 of 8 done: the rules file is saved in [`<path in the project>`](<absolute path>) and checked. Deploy your agent to start checking." When the customer has deployed, add where the deployed agent runs, only from what is known: on Databricks, the App [`<app name>`](https://<workspace host>/apps/<app name>) or the serving endpoint [`<endpoint name>`](https://<workspace host>/ml/endpoints/<endpoint name>), only when both the workspace address and the name are known from the code, the scan or a read-only look-up, never guessed; and its AI Endpoint in Collibra, [`<name>`](<instance>/asset/<endpoint id>), when its id is known. Then "Learn more:" and the page on where the SDK finds the rules file (`references/documentation.md`).

Exit when the file is in place and checked, and the customer has the deploy steps or the hand-off note.
