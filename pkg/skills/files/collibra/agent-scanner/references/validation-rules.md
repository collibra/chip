# Validation rules

What gets an agent scan rejected, and what to do. Written for you; tell the customer in plain words.

## Finalize rejections (Stage 06)

`finalize_agent_scan` lists every problem in `problems`, each with what to change.

| Problem reported | Cause | Fix |
|---|---|---|
| `extraction.agents must be a list` (or `tools`) | One of them is missing, or is a string holding JSON. | Write both as native arrays. |
| `<kind> name '<x>' is shorter than 3 characters` | Collibra needs names of 3 or more characters. | Choose a descriptive name, keep the original in a warning. |
| `two <kind>s share the name '<x>'` | Duplicate names within agents or within tools. | Give each a distinct name (include the module or file), record the originals in a warning. |
| `exactly one agent must have role ROOT; found N` | None or several roots. | Zero: apply the default from Stage 00 (the agent nothing else delegates to) and add a warning. Several: Stage 03, one root per file. |
| `agent '<x>' has role '<y>'` | Role is not `ROOT`, `SUBAGENT` or `ORPHAN`. | Use one of the three. |
| `agent '<x>' lists models but N of them has isDefault true` | Several or no default model. | Mark exactly one model `isDefault: true`. If none is clearly the default, pick the one used for the agent's main reasoning and add a warning. |
| `agent '<x>' needs '<flag>' as true or false` | A completeness flag is absent. | Add it, `true` or `false` (Stage 05). |
| `platform.name '<x>' must be UPPER_SNAKE_CASE` or `platform must be an object with only 'name' and 'metadata'` | The platform block is malformed. | Use `{ "name": "DATABRICKS", "metadata": {...} }`. |
| `agent '<x>' carries the old field 'deploymentVersion'` (or `'endpointUrl'`) | The old flat fields, which the service refuses. | Put them in `deployment`: `{ "version": ..., "endpoint": { "url": ..., "name": ... } }`. |
| `agent '<x>' is not the ROOT agent but carries 'deployment'` | The deployment belongs on the root only. | Move it to the root agent or remove it. |
| `deployment.endpoint.url must be an address without a user or password before '@'` | The address has a user part (`user:password@`), and Collibra shows it as sent. | Remove the user part, or leave the endpoint out and warn. |
| `<x> has an unknown field '<y>'` or `<x> is missing the required field '<y>'` | The schema refuses fields it doesn't know, and needs the ones `agent-scan-fields.md` marks as required. | Remove the unknown field (or put it where `agent-scan-fields.md` says) and add the missing one from the code. |

The tool drops a reference to an unknown agent or tool and adds a warning instead of rejecting. Treat those warnings as a hint that the extraction missed a tool or agent.

## Collibra rejections (Stage 07)

When a send fails, the answer carries an error code and a message. Explain in plain words and say what to change.

| Code | Meaning | What to do |
|---|---|---|
| `requestInvalid` | The agent id or the document is missing or unreadable. | Check `agent_scan.json` exists and is JSON; check an agent id was given. |
| `scanVersionUnsupported` | This Collibra does not read this scan format version. | Stop. Tell the customer this skill and their Collibra are out of step and the skill needs updating. Do not edit the version. |
| `scanRootAgentInvalid` | Not exactly one ROOT. | Back to Stage 03. |
| `scanDuplicateName` | Duplicate names. | Back to Stage 05, rename. |
| `scanNameTooShort` | A name under 3 characters. | Back to Stage 05, rename. |
| `scanUnresolvedReference` | An agent or tool names something the document lacks. | Back to Stage 05: add the missing agent or tool, or remove the reference. |
| `scanDefaultModelInvalid` | Models listed without exactly one default. | Back to Stage 05. |
| `scanGuardrailConfidenceMissing` | A guardrail without a confidence. | Back to Stage 05: add a confidence, rerun Stage 06. |
| `scanLlmFallbackWithoutLlm` | The document claims no LLM was used but marks fields as LLM-derived. | Stop. This should not happen with the output of `finalize_agent_scan`. Show the customer the message; do not edit `parser` by hand. |
| `scanRootOnlyField` | A deployment-only field on a non-root agent. | Stop. This skill never writes these fields; report the message. |
| Sign-in or permission errors (401 or 403) | The Collibra connection is not signed in, or may not write agent scans, or is for a different environment. | Not temporary. Offer the handoff. Retry once only after the customer says access changed. |

A rejected document is not stored by Collibra. After any change, rerun Stage 06, show the summary again and ask for consent again.

## Same code, same hashes

Scanning unchanged code should give the same scan every time. That is why labels are lowercase and quotes are verbatim. If the customer scans again and Collibra reports a new state for code they did not touch, the likely causes are a reworded prompt, a changed label spelling, or a different set of tools. Compare the two `extraction.json` files before blaming anything else.
