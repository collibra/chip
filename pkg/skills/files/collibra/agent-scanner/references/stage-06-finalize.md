# Stage 06: Finalize

Turn the extraction into the complete fingerprint document. Load `references/finalize-procedure.md` and `references/validation-rules.md` with this stage.

If Stage 02 chose a scan from the stored document (`redeployed`, or `unchanged` with a send asked for) and the stored scan passed the check below, `agent_scan.json` is the known document with only its unhashed fields set, as that file describes. You still build the program and pass the self-check below (Steps 1 and 2), and you run it on that document to confirm it is valid and its hashes recompute (Step 3 as a check, not as a rewrite); then go to Step 5.

## Step 1: Build the finalize program

Write a program in the output folder from `finalize-procedure.md`: `finalize-agent-scan.py` (Python 3, standard library only; preferred) or `finalize-agent-scan.sh` (bash with `jq` and `shasum -a 256` or `sha256sum`). Follow the steps in that file exactly, in order. Do not add behaviour that is not in it: no network calls, no reading outside the scan root, no changes to `extraction.json`.

## Step 2: Pass the self-check

Run the program's hashing on the six test extractions at the end of `finalize-procedure.md` and compare every hash. All must match exactly. For the first one, also assemble the whole document and compare it with the expected document shown there (without `generatedOn`): every key and value, including `via` on the models, system prompt, middleware and guardrails.

- Mismatch: find the difference against the written steps (usual causes: wrong concatenation order, not sorting the hash strings themselves, formatting `5` as `5.0`, hashing a lowercase-before-normalise value) and fix the program. Never change an expected value.
- Three failed attempts: stop. Tell the customer the document cannot be completed here. Never fall back to hand-written hashes (H6).

## Step 3: Run it on the extraction

From the output folder, run the program on `extraction.json`, writing `agent_scan.json`. Keep the program next to it so the customer can see exactly what ran, including how the hashes are built.

- **Success:** it prints the counts, the document `contentHash` and a suggested agent id. Go to Step 5.
- **Rejected:** it lists problems. Go to Step 4.

## Step 4: Fix a rejection

Fix `extraction.json` and rerun. Common causes and fixes are in `validation-rules.md › Finalize rejections`. Fix the extraction from the code, not by loosening the facts: if the code really has no clear entry point, that is a question for the customer, not something to paper over.

If the same problem returns after two fixes, stop and show the customer the problem line and what you tried.

## Step 5: Read the warnings

The program adds its own warnings (a dropped reference, a defaulted confidence) to yours. Read the whole list. A warning that points to a gap in the extraction (a dropped reference means a tool or agent was missed) is worth another look at the code now; fix it and rerun. The rest stay in the document and appear in the final review.

## Step 6: Sanity check against the code

Before showing it, check three things in the document against what you read:

1. The agent count and the ROOT match the table the customer confirmed in Stage 03.
2. A tool named in the code with a clearly relevant guardrail (for example a size limit you saw) has that guardrail in the document.
3. No credential-like value and no personal data appear anywhere in the document. Search it. If one does, go back to Stage 05, redact, and rerun.

## Validating a stored scan (from Stage 02)

Used when Stage 02 has an unchanged known agent and the customer chose to check the scan Collibra holds. It proves the stored document is valid and its hashes are the ones this skill would compute, which is what a send would need.

1. Call `get_latest_agent_scan` with the document included and write it unchanged to `known-document.json`.
2. Build the finalize program and pass its self-check (Steps 1 and 2 above).
3. Make `extraction.json` from `known-document.json` as Stage 02 describes for the changed-only extraction (keep `agents` and `tools`, drop every field the program sets), but keep the root agent's `deployment` and the `platform` and `source` as stored: the deployment's `version` and `endpoint.url` are in the document hash, so the hash only recomputes with them. Run the program on it (Step 3), writing `agent_scan-check.json`. Never edit the stored document to make it pass.
4. Report one of: **rejected**, with the problems the program lists; **different hashes**, naming each agent or tool whose recomputed `contentHash` differs from the stored one (the stored scan was made by a producer that reads the same code differently, so a re-read is the way to bring it in line); or **valid**, when the program accepted it and every hash, the agents', the tools' and the document's, equals the stored one. State the schema and producer version of the stored scan.

## Exit criteria

- The program passed the self-check in this session.
- `agent_scan.json` exists and was produced by that program.
- Warnings reviewed.
- Nothing sensitive in the document.

End with the short summary and move to Stage 07.
