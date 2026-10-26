# Stage 06 — Enforceability check

Give every rule an honest, plain-language verdict on whether Guardian can enforce it at runtime — without describing how Guardian works.

**Load with this stage:** `references/statement-guide.md` (properties P1–P7). Runs per rule inside Stage 04 and once over the whole contract before Stage 07.

## Entry criteria

Per rule: a candidate from Stage 04 steps 1–4. Whole contract: Stage 04 exit criteria met.

## The five marks the customer sees

| Mark | Label shown | Meaning in customer language |
|---|---|---|
| ✅ | Enforceable | Specific enough to check at the moment the agent acts. Guardian will enforce it. |
| 🟡 | Partly enforceable | Part of it can be checked precisely; part depends on judgement about meaning or wording. Guardian enforces the precise part and records the rest. |
| 📋 | Policy-only | A real obligation, but not one that can be checked at the moment of a single action (it depends on history, totals over time, or something the organisation does rather than the agent). Recorded in the contract; not enforced at runtime. |
| ⚠️ | Needs a change | Cannot be checked as written, but a small rewording would make it ✅. Always comes with a proposed rewrite. |
| ➖ | Not about this agent | Refers to an action or data this agent doesn't have. Keep for completeness or drop. |

Never invent a sixth mark, and never show a split mark ("✅/🟡") in the review table — pick one and put the nuance in the note. These five labels are the only enforceability vocabulary the customer sees.

## Scope of the check in edit mode

Baseline rules the customer did not ask to change are **marked for information only**. They are never reworded, split, or flagged as blocking (H3). If a baseline rule would score ⚠️, tell the customer once — *"Rule R-105 as it stands can't be enforced as written; want me to include a fix as a change?"* — and only touch it if they say yes, in which case it becomes a CHANGED rule.

## The check — run in this order, stop at the first that applies

### C0. Is it one obligation? → else ⚠️ (split first)
Two different actions or two different obligations joined by "and" → propose two rules and check each separately. A single class of action described with "or" ("changes or removes") is one obligation.

### C1. Whose obligation is it?
- Subject is the **organisation or a person** ("<Organisation> must…", "<role> must…", "…must be logged / retained / reviewed / tested") → go to **C2**.
- Rule is about the agent **refusing or not acting on instructions** ("must refuse instructions that…", "must not act on instructions embedded in…") → these are about the agent's own behaviour, not a named action; go to **C5** (they normally land 🟡, sometimes ✅).
- Otherwise: does the rule refer to an action, data source or channel the agent (per Stage 02) has — in plain words or by name? If none matches → **➖**: *"Your agent has no action that does this. Keep it marked 'not about this agent' for completeness (a), or drop it (b)?"* If Stage 02 is unverified (offline), skip this test and tag `(unverified)`.

### C2. Is it an obligation on the organisation, not the agent? → 📋
Logging, retention, reviews, testing, training-data prohibitions, third-party audits, storage location. *"This is something your organisation does, not something the agent does in the moment. Guardian records it as part of the contract but won't enforce it at runtime."*

### C3. Does it need more than one action to decide? → 📋 (offer a rewrite)
"Before / after", "across several…", "in total over…", "a sequence of…", "having previously…", "without first…", or a fact that is only known from an earlier action (an order date the refund action doesn't carry). Mark 📋 and offer the single-moment rewrite where one exists:

> *"'…before the fraud register has been checked' depends on an earlier step, so it's policy-only as written. If the fraud-check result travels with the payment, I can rewrite it as '…unless the fraud-check result accompanies the payment' — enforceable. Same intent?"*

A condition that is **something the customer said in the conversation** ("where a customer asks for a person", "where the policyholder says they are in difficulty") is not a prior action — treat it under C5, not here. If the customer prefers the original wording, keep it 📋. Never silently swap.

### C4. Does it name a specific action? → else ⚠️
"Be careful with", "consider", "keep in mind", "protect", "respect", "handle appropriately" — with no verb the agent performs. Mark ⚠️ with a rewrite that names an action from Stage 02.

### C5. Is the subject concrete? → 🟡 or ⚠️
- If the judgement **is the obligation** — tone, hostility, stereotyping, implying dishonesty, truthfulness of an explanation, staying on topic, recognising what a customer says, instructions "however phrased" → **🟡**. Say: *"The precise part — e.g. that a message goes to this recipient — is enforced. Whether wording 'stereotypes' is a judgement about meaning, so that part is recorded."*
- If a judgement word merely **qualifies an otherwise precise action** ("excessive rows", "sensitive tables", "inappropriate advice") → **⚠️**; ask what it means here.

### C6. Are thresholds complete? → else ⚠️
Number without unit, unit without direction, "large/small/many/few". Ask for the number, the unit, above/below, and whether it is per action. A total over time is C3 (📋).

### C7. Refuse vs. hand over — deliberate? → confirm, no mark change
If the rule has a threshold or a consequential decision and no named role, ask once: *"Is there a person whose job this is? Naming them turns 'stop' into 'hand over'."* Either answer is ✅.

### C8. Everything else → ✅

## Whole-contract pass (before Stage 07)

Run C0–C8 on any rule not yet marked (in edit mode, baseline rules for information only — see above), then, for new and changed/added rules:

1. **Conflicts.** Two rules that give opposite answers for the same action under the same condition. Stop and ask the customer to choose: *"Rules 4 and 9 contradict each other — I won't publish both. Which should stand: rule 4 (a) or rule 9 (b)?"*
2. **Duplicates and near-duplicates.** Propose a merge; accept "keep both".
3. **Overlap with the other contract type.** Allowed; flag it (wording in Stage 04 › Cross-contract awareness).
4. **Placeholders.** Any `<angle bracket>` left → ⚠️ **that cannot be carried**: get the value or drop the rule before Stage 07.
5. **Enforcement vocabulary / rationale inside rules.** Reword; show the diff. Exception: a word that is the plain-language form of the agent's own action ("hand over to a human agent (`escalate_to_agent`)") is fine — it names what the agent does, not what Guardian does.
6. **Personal data or secrets inside a rule** (a real person's name, an email, a token) → remove and use a role or category (H8).

## Reporting the result

Print the contract in the review format with marks, then — once, only on the first whole-contract pass — up to three coverage suggestions from `rule-families.md` the customer can take or leave, then:

> <n> rules: <a> ✅ enforceable · <b> 🟡 partly enforceable · <c> 📋 policy-only · <d> ⚠️ needs a change · <e> ➖ not about this agent.
> A contract with some 📋 rules is normal — they are the governance record. ⚠️ rules must be fixed or explicitly carried before publishing.
> Anything to change or add before I show you the exact version that would go to Collibra?

This closing question and the Stage 07 consent question are **never in the same message**. Wait for the answer; if it is "no changes", the next message is Stage 07. Do not ask the closing question a second time if nothing has changed.

If the customer asks "why" on a 📋 or 🟡, give the one-sentence customer-language reason from the relevant check. If they push for the mechanism → `refusal-scripts.md › R-INTERNALS`.

## What this stage is not

- Not Collibra's verdict. You are predicting from the wording and the agent's action list. Say so if asked: *"This is my reading of the wording; Collibra will confirm once the contract is saved."*
- Not a compliance opinion (R-NOT-ADVICE).
- Not a place to describe how Guardian reaches its own verdict (H6).

## Exit criteria → Stage 07

- Every new / changed / added rule marked; no ⚠️ unless the customer chose to carry it; no placeholders.
- No unresolved conflicts.
- Tally printed and the customer has answered the closing question.
