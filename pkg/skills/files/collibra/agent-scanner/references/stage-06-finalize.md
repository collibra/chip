# Stage 06: Finalize

Turn the extraction into the complete scan document with the `finalize_agent_scan` tool of this Collibra connection. Load `references/validation-rules.md` with this stage. You never compute a hash yourself (H6).

The tool runs inside the Collibra connection on the customer's computer: it calls nothing, sends nothing and changes nothing, so it needs no consent. It checks the extraction, repairs references that match nothing, computes every hash and returns the document.

If Stage 02 chose a scan from the stored document (`redeployed`, or `unchanged` with a send asked for) and the stored scan passed the check below, `agent_scan.json` comes from that check (Step 2 as a check, not a rewrite); then go to Step 4.

## Step 1: Add the source

If the extraction has no `source` and the scan root is a git repository, add it now: `repoUrl` from `git -C <scan root> remote get-url origin`, `commitSha` from `git -C <scan root> rev-parse HEAD`. Ignore a failure, and leave out an empty value. When the extraction already has a `source` (for example a deployed commit, Stage 04), keep it as it is.

## Step 2: Call the tool

Call `finalize_agent_scan` with `extraction` set to the content of `extraction.json`, as a JSON object.

- **`success`:** write its `document` unchanged to `agent_scan.json` in the output folder. Note the counts, the `contentHash` and the `suggestedAgentId`. Go to Step 4.
- **`validation_error`:** it lists every problem in `problems`. Go to Step 3.

If the Collibra connection doesn't offer `finalize_agent_scan`, stop here: tell the customer the document can't be completed with this connection, and offer to keep the extraction. Never fall back to hashes of your own.

## Step 3: Fix a rejection

Fix `extraction.json` and call the tool again. Common causes and fixes are in `validation-rules.md › Finalize rejections`. Fix the extraction from the code, not by loosening the facts: if the code really has no clear entry point, that is a question for the customer, not something to paper over.

If the same problem returns after two fixes, stop and show the customer the problem and what you tried.

## Step 4: Read the warnings

The tool adds its own warnings (a dropped reference, a defaulted confidence, a dropped endpoint) to yours. Read the whole list. A warning that points to a gap in the extraction (a dropped reference means a tool or agent was missed) is worth another look at the code now; fix it and call the tool again. The rest stay in the document and appear in the final review.

## Step 5: Sanity check against the code

Before showing it, check three things in the document against what you read:

1. The agent count and the ROOT match the table the customer confirmed in Stage 03.
2. A tool named in the code with a clearly relevant guardrail (for example a size limit you saw) has that guardrail in the document.
3. No credential-like value and no personal data appear anywhere in the document. Search it. If one does, go back to Stage 05, redact, and call the tool again.

## Validating a stored scan (from Stage 02)

Used when Stage 02 has an unchanged known agent and the customer chose to check the scan Collibra holds. It proves the stored document is valid and its hashes are the ones this skill would compute, which is what a send would need.

1. Call `get_latest_agent_scan` with the document included and write it unchanged to `known-document.json`.
2. Make `extraction.json` from `known-document.json` as Stage 02 describes for the changed-only extraction (keep `agents` and `tools`, drop every field the tool sets), but keep the root agent's `deployment` and the `platform` and `source` as stored: the deployment's `version` and `endpoint.url` are in the document hash, so the hash only recomputes with them. Call `finalize_agent_scan` on it (Step 2) and write the document to `agent_scan-check.json`. Never edit the stored document to make it pass.
3. Report one of: **rejected**, with the problems the tool lists; **different hashes**, naming each agent or tool whose recomputed `contentHash` differs from the stored one (the stored scan was made by a producer that reads the same code differently, so a re-read is the way to bring it in line); or **valid**, when the tool accepted it and every hash, the agents', the tools' and the document's, equals the stored one. State the schema and producer version of the stored scan.

## Exit criteria

- `agent_scan.json` exists and is the `document` `finalize_agent_scan` returned, unchanged.
- Warnings reviewed.
- Nothing sensitive in the document.

End with the short summary and move to Stage 07.
