# Validation rules

What gets a fingerprint rejected, and what to do. Written for you; tell the customer in plain words.

## Finalize rejections (Stage 06)

The finalize program lists each problem on its own line.

| Problem reported | Cause | Fix |
|---|---|---|
| `"agents" and "tools" must both be JSON arrays` | One of them is missing, or is a string holding JSON. | Write both as native arrays. |
| `<kind> name '<x>' is shorter than 3 characters` | Collibra needs names of 3 or more characters. | Choose a descriptive name, keep the original in a warning. |
| `two <kind>s are both named '<x>'` | Duplicate names within agents or within tools. | Give each a distinct name (include the module or file), record the originals in a warning. |
| `exactly one agent must have role ROOT, found N` | None or several roots. | Zero: apply the default from Stage 00 (the agent nothing else delegates to) and add a warning. Several: Stage 03, one root per file. |
| `agent '<x>' has role '<y>'` | Role is not `ROOT`, `SUBAGENT` or `ORPHAN`. | Use one of the three. |
| `agent '<x>' lists models but not exactly one with isDefault true` | Several or no default model. | Mark exactly one model `isDefault: true`. If none is clearly the default, pick the one used for the agent's main reasoning and add a warning. |
| `agent '<x>' is missing the boolean <flag>` | A completeness flag is absent. | Add it, `true` or `false` (Stage 05). |
| `platform name '<x>' must be UPPER_SNAKE_CASE` or `"platform" must be an object with only "name" and "metadata"` | The platform block is malformed. | Use `{ "name": "DATABRICKS", "metadata": {...} }`. |
| `agent '<x>' carries deploymentVersion` or `endpointUrl` | The old flat fields, which the service refuses. | Put them in `deployment`: `{ "version": ..., "endpoint": { "url": ..., "name": ... } }`. |
| `agent '<x>' carries deployment and is not the root agent` | The deployment belongs on the root only. | Move it to the root agent or remove it. |
| `deployment.endpoint.url carries credentials` | The address has a user part (`user:password@`), and Collibra shows it as sent. | Remove the user part, or leave the endpoint out and warn. |

The program drops a reference to an unknown agent or tool and adds a warning instead of rejecting. Treat those warnings as a hint that the extraction missed a tool or agent.

## Collibra rejections (Stage 07)

When a send fails, the answer carries an error code and a message. Explain in plain words and say what to change.

| Code | Meaning | What to do |
|---|---|---|
| `requestInvalid` | The agent id or the document is missing or unreadable. | Check `agent_scan.json` exists and is JSON; check an agent id was given. |
| `fingerprintVersionUnsupported` | This Collibra does not read this fingerprint version. | Stop. Tell the customer this skill and their Collibra are out of step and the skill needs updating. Do not edit the version. |
| `fingerprintRootAgentInvalid` | Not exactly one ROOT. | Back to Stage 03. |
| `fingerprintDuplicateName` | Duplicate names. | Back to Stage 05, rename. |
| `fingerprintNameTooShort` | A name under 3 characters. | Back to Stage 05, rename. |
| `fingerprintUnresolvedReference` | An agent or tool names something the document lacks. | Back to Stage 05: add the missing agent or tool, or remove the reference. |
| `fingerprintDefaultModelInvalid` | Models listed without exactly one default. | Back to Stage 05. |
| `fingerprintGuardrailConfidenceMissing` | A guardrail without a confidence. | Back to Stage 05: add a confidence, rerun Stage 06. |
| `fingerprintLlmFallbackWithoutLlm` | The document claims no LLM was used but marks fields as LLM-derived. | Stop. This should not happen with the finalize program's output. Show the customer the message; do not edit `parser` by hand. |
| `fingerprintRootOnlyField` | A deployment-only field on a non-root agent. | Stop. This skill never writes these fields; report the message. |
| Sign-in or permission errors (401 or 403) | The Collibra connection is not signed in, or may not write fingerprints, or is for a different environment. | Not temporary. Offer the handoff. Retry once only after the customer says access changed. |

A rejected document is not stored by Collibra. After any change, rerun Stage 06, show the summary again and ask for consent again.

## Same code, same hashes

Scanning unchanged code should give the same fingerprint every time. That is why labels are lowercase and quotes are verbatim. If the customer scans again and Collibra reports a new state for code they did not touch, the likely causes are a reworded prompt, a changed label spelling, or a different set of tools. Compare the two `extraction.json` files before blaming anything else.
