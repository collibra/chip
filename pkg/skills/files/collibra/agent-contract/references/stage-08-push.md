# Stage 08 — Publish to Collibra

Save exactly what the customer consented to in Stage 07, nothing more and nothing else, and confirm what happened by reading it back.

**Load with this stage:** `references/collibra-connection.md`. This is the only stage that uses write capabilities.

## Entry criteria

Explicit consent from Stage 07 for the exact version shown there. If you are not certain the consent was for this version, or the Stage 07 table and the reply to it are not in your current context, go back to Stage 07.

## Step 1 — Pre-flight (all reads; stop on any failure)

Run these in order. The baseline check is last on purpose: Collibra doesn't check which version an edit started from, so this read is the only thing standing between the customer's save and someone else's newer changes. Make the save straight after it.

1. **Write capability present?** If not → output the handoff format from `collibra-connection.md`, state clearly that nothing has been saved, and end. Do not attempt a workaround.
2. **Existence re-check (new only)** For specialised with agents named, `GET /agentContracts?agentId=` for each; with no agents named, skip this. For global, `GET /agentContracts`: the global contract, if there is one, is the first result. If one was missed, the save fails with "global contract already exists" and Step 2 handles it. If a contract of this kind now exists where none did → stop, show it, return to Stage 01 Step 6.
3. **Unverified actions**: if `get_agent_actions(agent version)` is now available, re-run Stage 06 C1 on those rules. If an action doesn't exist → stop, show the affected rules, return to Stage 04 for those rules. If still unavailable → proceed only if the customer accepted the "unverified" note in Stage 07.
4. **Permission**: if any pre-flight read returns a permission error → `R-PERMISSION`, stop.
5. **The list fits Collibra's limits**: at least 1 rule and at most 200; no statement over 2000 characters; no blank statement; no two statements the same once tidied (`collibra-connection.md`); no statement contains an unfilled placeholder such as `<agent name>` or `<organisation>`; a `<` or `>` used inside a real rule ("above >$500") is fine. If one fails, stop and return to Stage 06 for that rule.
6. **Domain chosen (new only)**: the Stage 01 frame names a domain. If not, return to Stage 01 Step 7.
7. **Baseline unchanged? (edit only)** `GET /agentContracts/{id}/rules` again. If the current version is not the one you loaded in Stage 01, someone saved a new version meanwhile. **Stop**: *"Someone saved version <new> of this contract since we started from version <old>: <what changed, by rule number>. I haven't saved anything. Let's review the differences before continuing."* Return to Stage 07 with the new version as the baseline.

## Step 2 — Save, once

Build the list from the Stage 07 table, in its order: every UNCHANGED rule, the new wording of every REWORDED rule, every ADDED rule. REMOVED rules are left out. Nothing else goes in.

- **New contract:** `POST /agentContracts` with the `layer`, the name and the domain agreed in Stage 01, and the list. No agent ids: linking happens in Collibra afterwards.
- **Edit:** `PUT /agentContracts/{id}/rules` with the contract id and the list. Nothing else goes in the body.

The pushed list becomes the contract's current version straight away; Collibra keeps the previous one in its history.

Each write is called twice: first with `confirm=false`, which returns Collibra's preview and writes nothing, then with `confirm=true`. The preview must show exactly the Stage 07 content: for an edit, its added, removed and kept lists must match the Stage 07 table and its current version number must match the baseline; for a new contract, the layer, name, domain and statements. If not, stop and go back to Stage 07.

**If Collibra explains the error** (`collibra-connection.md › Errors Collibra explains`), nothing was written. Don't read back; do this:

| Collibra says | Do |
|---|---|
| Feature not enabled | *"Agent Contracts isn't turned on for your organisation, so Collibra won't save contracts yet. An admin can turn it on in AI Command Center. I haven't saved anything."* Offer the handoff format. Don't retry in this session unless the customer says it has been turned on, and then once. |
| No permission | `R-PERMISSION`, and the permission rule below. |
| Updated by another request | Someone saved a moment before you. Collibra suggests retrying, but **don't**: a retry would replace their version with this one. Handle it like Step 1.7: re-load the rules, tell the customer what changed by rule number, say nothing was saved, and return to Stage 07 with the new version as the baseline. |
| Name already used in the domain | *"Collibra already has something called **<name>** in **<domain>**, so I couldn't save. I haven't saved anything. What name should we use instead?"* Check the new name as in Stage 01 Step 7: warn if a contract in another domain has it, and ask again if one in this domain does. Then re-print the Stage 07 frame with the new name and ask the Stage 07 consent question again. The rules haven't changed, so don't re-show the review. |
| Global contract already exists | Stop. Return to Stage 01 Step 6; the customer can edit the existing one instead. |
| Domain not found, or not a Governance Asset domain | Return to Stage 01 Step 7 for the domain. Then re-print the Stage 07 frame and ask the consent question again. |
| Duplicate, blank or invalid statements | Report which rules, return to Stage 06 for them. |

**Rules for writes**
- Statements are written verbatim. No reformatting, no trailing rationale, no numbering inside the statement.
- Never send a list that differs from the Stage 07 table: no rule dropped, added, reworded or moved.
- If the save **fails with an error Collibra doesn't explain** (a timeout, a server error, no answer), do not retry. Go to Step 3 to find out what landed.
- A **permission error** on a write is never retried on request alone. Apply `R-PERMISSION`; a retry needs the customer to tell you the permission has been changed, and then it is one attempt. Offer the handoff format meanwhile.
- Never use a delete, purge, or bulk operation in the save path, whatever the host offers (H2). There is no delete tool in this build.

### Owner of a new specialised contract

After a successful create of a specialised contract, make sure the contract has a direct Owner. Collibra normally assigns the creator itself, so this is a check, not a second write by default. The Stage 07 frame told the customer about it.
1. `get_asset_details` on the new contract's id; look in `responsibilities` for a role **Owner** with `inherited` false. The tool lists user names, not user ids, so it shows that a direct Owner exists but cannot prove who it is.
2. A direct Owner is present → nothing more to do.
3. None → `edit_asset` on the contract's id with one operation, `set_responsibility`, role `Owner`, `userId` = the `createdBy` the create returned. This is the only use of `edit_asset` in this skill and only for this assignment. Report it.
4. If that assignment fails (permission or otherwise): say so plainly, that the contract exists but has no Owner assigned to the customer, and that someone with the right permission must assign it in Collibra. Do not retry or look for another route.

Global contracts get no extra step. Edits of existing contracts never change owners.

## Step 3 — Read back and verify

- **Edit, save succeeded:** the `PUT` response is the read-back. It holds the current version and its rules.
- **New contract, save succeeded:** `GET /agentContracts/{id}/rules` with the id `POST` returned.
- **Save failed without an explanation:** for an edit, `GET /agentContracts/{id}/rules`. For a new contract, `GET /agentContracts?name=<name>` and keep only an exact name match; don't use the per-agent list, because a new specialised contract isn't linked to any agent yet. Names are only unique within a domain, so a match is the customer's new contract only if its rules are exactly the list you sent.

Compare to the Stage 07 table:
- For an edit, the version number is higher than the baseline's. For a new contract, it is version 1.
- It holds exactly the statements you sent, in the same order, verbatim once tidied.
- UNCHANGED rules kept their ids from the baseline. Removed statements are gone from the current version.

Any mismatch → report it precisely (which rule, expected vs actual). Do not "fix" it by saving again without going back through Stage 07.

## Step 4 — Report

```
PUBLISHED — <contract name>
Kind:    Global | Specialised · checked against <agent name(s) | no particular agent>
Result:  Created, version 1 | Version <v> saved  ·  <date and time saved>
Owner:   <assigned by Collibra | assigned to you | not assigned> (specialised, new contracts only)
Rules:   <n> total — <a> added · <b> reworded · <c> removed · <d> unchanged
Marks:   <✅ n> enforceable · <🟡 n> partly enforceable · <📋 n> policy-only · <⚠️ n> needs a change (carried) · <➖ n> not about this agent
Link:    <Collibra link if the capability returned one>
```

Then, in plain words:
- What happens next: *"This is now the contract's current version in Collibra. The previous version stays in its history."* For a new specialised contract, add: *"Next, link it to the agents it should govern, on the contract in Collibra. Until then it doesn't govern any agent."* Do not describe what Collibra does with it beyond that.
- What the customer might do next: enrich the use case if it was thin; register an agent the customer named that isn't in Collibra yet; review policy-only rules with their compliance team.
- Offer: *"Want me to output a copy of the published contract for your records?"*

## Failed save

If the save failed without an explanation, read back first (Step 3), then report one of two outcomes.

**Nothing was saved** (the current version is still the one you loaded, or for a new contract, no contract with that name and those rules exists):

```
NOT SAVED — <contract name>
Error:   <one line, no stack traces or internal identifiers>
Current: unchanged, still version <v> from <date>
```

*"Nothing in Collibra changed. The contract you approved is still above."* If the customer asks to retry, run Step 1 again. If it passes, save the same list once; the consent still stands because the version hasn't changed. A permission error follows the rule in Step 2 instead.

**A new version was saved after all**: verify it as in Step 3 and report PUBLISHED. Never save a second time.

**New contract exists but doesn't match** (for example it has no rules): report it as a mismatch. Never `POST` again, which would make a second contract; any fix goes back through Stage 07 as an edit of the contract that now exists.

If the read-back itself fails, say you can't tell whether the save went through, ask the customer to check the contract in Collibra, and do not save again in this session until a read succeeds.

## Deleting a contract

There is no delete tool in this build, so you never delete a contract. When the customer asks for it, do not refuse: read the contract (`GET /agentContracts/{id}` and `GET /agentContracts/{id}/rules`), show name, kind, how many agents it governs (or all agents), rule count, current version and when it was saved, and say plainly: *"I can't delete contracts from here. A contract is deleted on the contract itself in Collibra, by someone with the right permission there. Deleting it removes it, with its history, and stops any enforcement it provides for <n agents / all agents>."* Offer the non-destructive alternatives (edit the rules, or output the rules as a record). You have deleted nothing; say so.

## Things you never do here

- Never write without Stage 07 consent for this exact version.
- Never push twice for one consent, except the single retry after a read-back shows nothing was saved, or a save repeated after the customer changed the name or domain and consented again. Never `POST` twice for the same contract.
- Never retry a save that lost to another person's save, even when Collibra suggests it.
- Never continue past a mismatched read-back.
- Never write to any object other than this contract.
- Never delete anything: not a rule, not a contract.
- Never describe endpoints, payloads, or how Collibra processes the contract afterwards (H6).

## Exit

Session complete. If the customer wants another contract → Stage 01 from the top (H9).
