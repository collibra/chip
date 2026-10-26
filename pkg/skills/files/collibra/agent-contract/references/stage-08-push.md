# Stage 08 — Publish to Collibra

Save exactly what the customer consented to in Stage 07 — nothing more, nothing else — and confirm what happened by reading it back.

**Load with this stage:** `references/collibra-connection.md`. This is the only stage that uses write capabilities.

## Entry criteria

Explicit consent from Stage 07 for the exact version shown there. If you are not certain the consent was for this version, or the Stage 07 table and the reply to it are not in your current context, go back to Stage 07.

## Step 1 — Pre-flight (all reads; stop on any failure)

1. **Write capability present?** If not → output the handoff format from `collibra-connection.md`, state clearly that nothing has been saved, and end. Do not attempt a workaround.
2. **Baseline unchanged? (edit only)** `get_contract(id)` again and compare to the Stage 01 baseline, rule by rule and by version number. If anything differs (someone else saved a new version), **stop**: *"The contract in Collibra has changed since we started — <what changed>. I haven't saved anything. Let's review the differences before continuing."* Return to Stage 07 with the new baseline.
3. **Existence re-check (new only)** `list_contracts(scope)`, and for a global contract `list_contracts(global)`. If a contract now exists where none did → stop, show it, return to Stage 01 Step 5 branch.
4. **Unverified actions** — if `get_agent_actions(agent)` is now available, re-run Stage 06 C1 on those rules. If an action doesn't exist → stop, show the affected rules, return to Stage 04 for those rules. If still unavailable → proceed only if the customer accepted the "unverified" note in Stage 07.
5. **Permission** — if any pre-flight read returns a permission error → `R-PERMISSION`, stop.
6. **Domain (new only)** — a Governance Asset domain id from `find_domain`, chosen by the customer. No id → stop and ask.
7. **Placeholders** — no statement contains an unfilled placeholder such as `<agent name>` or `<organisation>`. If one does, stop and return to Stage 04 for that rule. A `<` or `>` used as a comparison inside a real rule ("above >$500") is fine.

## Step 2 — Write

Use only the write capabilities in `collibra-connection.md`. One write per contract. Each write is called twice: first without `confirm` to get Collibra's preview, then with `confirm=true`. The preview must show exactly the Stage 07 content; if it does not, stop and go back to Stage 07.

**New contract**
1. `create_contract(scope, name, domain, rules[])` with every statement verbatim from Stage 07, in the order shown.

**Edit**
1. `replace_rules(contract id, statements[])` with the **full ordered list**: UNCHANGED rules exactly as read, CHANGED rules in their new wording, ADDED rules as new entries; RETIRED rules are simply left out. The preview lists what is added, removed and kept: it must match the Stage 07 table, and its current version number must match the baseline.
UNCHANGED rules are never reworded.

Collibra applies its own permissions to the write. Relay what it says; do not trigger, skip or describe anything beyond that.

**Rules for writes**
- Statements are written verbatim. No reformatting, no trailing rationale, no numbering inside the statement.
- If a write **fails**, stop immediately. Do not retry blindly. Go to Step 3 to determine what was actually written. Exception: a version conflict means someone else saved first: read the contract again, show the customer what changed, and go back to Stage 07.
- A **permission error** on a write is never retried on request alone. Apply `R-PERMISSION`; a retry needs the customer to tell you the permission has been changed, and then it is one attempt. If Collibra says Agent Contracts is not turned on, tell the customer it has to be enabled in AI Command Center first. Offer the handoff format meanwhile.
- Never issue a write that was not in the Stage 07 table.
- Never use a delete, purge, or bulk operation in the publish path, whatever the host offers (H2).

### Owner of a new specialised contract

After a successful `create_contract` with scope specialised, make sure the current user owns the contract asset. Collibra normally assigns this itself, so this is a check, not a second write by default:
1. `get_asset_details` on the new contract's id; look for a responsibility with role **Owner** that is not inherited and whose user is the `createdBy` user id returned by the create.
2. Present → nothing more to do.
3. Missing → `edit_asset` on the contract's id with one operation, `set_responsibility`, role `Owner`, `userId` = `createdBy`. This is the only use of `edit_asset` in this skill and only for this assignment. It is covered by the consent already given for creating the contract; do not ask again, but report it.
4. If that assignment fails (permission or otherwise): say so plainly, that the contract exists but has no owner assigned to the customer, and that someone with the right permission must assign it in Collibra. Do not retry or look for another route.

Global contracts get no extra step. Edits of existing contracts never change owners.

## Step 3 — Read back and verify

`get_contract(id)`. Compare to the Stage 07 table:
- Every ADDED / new statement present, verbatim.
- Every CHANGED statement updated.
- Every RETIRED rule absent from the active version (earlier versions still hold it).
- Every UNCHANGED statement identical, with the same rule id as in the baseline.
- The version number went up by one, or stayed the same if nothing changed.

Any mismatch → report it precisely (which rule, expected vs actual). Do not "fix" by issuing further writes without going back through Stage 07 for the fix.

## Step 4 — Report

```
PUBLISHED — <contract name>
Scope:   Global | Specialised → <agent>
Result:  Created | Updated to version <n> | No change (already identical)
Owner:   <you> (specialised, new contracts only)
Rules:   <n> total — <a> added · <b> changed · <c> retired · <d> unchanged
Marks:   <✅ n> enforceable · <🟡 n> partly enforceable · <📋 n> policy-only · <⚠️ n> needs a change (carried) · <➖ n> not about this agent
```

Then, in plain words:
- What happens next: *"The contract is saved as version <n>."* For a new specialised contract add: *"It governs no agent yet: agents are linked to it in Collibra."* Do not describe anything beyond what Collibra returned.
- What the customer might do next: enrich the use case if it was thin; register the agent if the contract was drafted for an unregistered one; review policy-only rules with their compliance team.
- Offer: *"Want me to output a copy of the published contract for your records?"*

## Partial failure

If a write failed part-way:

```
PARTIALLY PUBLISHED — <contract name>
Saved:      <exact list of rules written, verbatim>
Not saved:  <exact list>
Error:      <one line, no stack traces or internal identifiers>
```

*"I've stopped. Nothing else was changed. We can retry the unsaved items after you've checked the contract in Collibra, or leave it as is."* Retrying requires going back through Stage 07 for the unsaved items only.

## Deleting a contract

There is no delete tool in this build, so you never delete a contract. When the customer asks for it, do not refuse: `get_contract(id)` and `list_contracts(scope)`, show name, kind, agent / all agents, rule count, last changed, and say plainly: *"I can't delete contracts from here. A contract is deleted by deleting its asset in Collibra, which needs the right permission there. Deleting it is permanent and stops any enforcement it provides for <agent / all agents>."* Offer the non-destructive alternatives (edit the rules, or output the rules as a record). You have deleted nothing; say so.

## Things you never do here

- Never write without Stage 07 consent for this exact version .
- Never continue past a failed write or a mismatched read-back.
- Never write to any object other than this contract.
- Never delete anything: not a rule, not a contract.
- Never describe endpoints, payloads, or how Collibra processes the contract afterwards (H6).

## Exit

Session complete. If the customer wants another contract → Stage 01 from the top (H9).
