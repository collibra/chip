# Stage 04 — Rule authoring

Turn what the customer wants into rules Guardian can enforce, one at a time, with the customer in control of the wording.

**Load with this stage:** `references/statement-guide.md`, `references/rule-families.md`. Keep the Stage 02 context summary (agent actions, use case, existing rules) in view.

## Entry criteria

- Stage 01 done: you know whether this is a new or existing contract, and whether it is global or specialised.
- Stage 02 done: you have the named agents' lists of actions (or an explicit "unverified — offline" marker) and the use-case summary, or, with no agents named, the kinds of agents the rules are about.
- Stage 03 done: you have the raw material — the customer's typed intent, extracted candidate rules from a document, or a starter draft from Stage 05.

## What you produce

A **working contract**: a numbered list of rules in the review format from `statement-guide.md`, each with a provisional check mark from Stage 06. After a single change show only the changed row; re-print the full contract at the end of each family, every five rules, and at stage exit. New rules are numbered `1..n`; Collibra assigns rule ids when it saves. In edit mode number existing rules by their position in the baseline and tag each item reword / remove / add.

## The loop — for each candidate rule

### 1. Restate
Show the customer's input and your proposed rule side by side. Keep their words where they already meet the statement guide.

> You wrote: "no big refunds without sign-off"
> Proposed rule 3: *The agent must not approve a refund above <amount> without <role>.*
> Two things I need from you: the amount (with currency), and who signs off.

### 2. Name the action
Every rule must name something the agent actually does. Use the Stage 02 action list:

- If the customer's rule clearly maps to one action, name it in plain words (not the internal tool identifier unless the customer uses it) and confirm.
- If it maps to several actions (e.g. the agent has three ways to message a customer), ask whether the rule covers all of them; default to all.
- If it maps to none, say so and point at what the agent actually does: *"Your agent doesn't have an action that does this — it can't send email, for instance. It can \<name 2–3 real actions from the Stage 02 list\>, if one of those is what you meant. Otherwise I can keep this marked 'not about this agent' for completeness (a), or drop it (b). Which?"* Always fill in real actions from Stage 02, never invent one.
- Remove (edit mode): no action to name. Record the customer's one-line reason and move on.
- If Stage 02 is offline/unverified, write the rule anyway and tag it `(action unverified)`; Stage 08 re-checks before publish.
- Global contract, or specialised with no agents named: there's no action list. Name the kind of action in plain words ("send a message to a customer"), as Stage 02 explained.

### 3. Sharpen
Apply the seven properties (P1–P7 in `statement-guide.md`) in order. Ask only the questions whose answer would change the wording or the mark, and put the questions one rule needs into **one message (at most three)** rather than one per turn:

| Check | Ask |
|---|---|
| P1 One obligation | "This says two things — split into two rules?" |
| P2 Specific action | "Which action of the agent does this apply to?" |
| P3 Single-moment | "This depends on an earlier step. Enforce at the moment of X instead, or keep as policy-only?" |
| P4 Concrete subject | "What counts as 'sensitive' here — can you name the categories?" |
| P5 Threshold | "Per transaction or in total? Which currency? Above, or at or above?" |
| P6 Refuse vs hand over | "Is there a person whose job this is? Then let's name the role." |
| P7 Process commitment | "This is an obligation on the organisation, not the agent — keep it as policy-only?" |

Stop asking once the rule is clean. Do not run the full checklist aloud for a rule that already passes.

### 4. Robustness (only for rules that pass 1–3)
Pick the one or two most relevant robustness prompts from `statement-guide.md` (prompts 1–7: other ways to do the same thing, variants, splitting, proxies, who else could be the target, the word or the promise, language). Ask them as plain questions. Accept "no" without argument.

### 5. Check
Run the Stage 06 check on the rule and record the mark (✅ 🟡 📋 ⚠️ ➖). Show the glyph next to every proposed rule from the first time you propose it — a rule described as "needs a change" in prose without its ⚠️ is not marked. If ⚠️, propose the fix and loop back to step 3. If 📋 or 🟡, tell the customer what that means in one sentence and let them choose to keep, rewrite, or drop.

### 6. Place
Add the rule to the working contract. If it is near-identical to an existing rule (new or already published), say so and propose a merge. If it **contradicts** another rule in the same contract (one allows what the other forbids for the same action and condition), stop: *"Rules 4 and 9 contradict each other — I won't publish both. Which should stand: rule 4 (a) or rule 9 (b)?"* Never resolve a conflict silently. For a specialised contract, also run the global check below.

## Cross-contract awareness

### Specialised contract: the global check

The global contract always applies, and a specialised rule can't relax it. Compare each new or changed rule with every global rule that covers the same action. Sort it into one of four outcomes:

| Outcome | What it looks like | Do |
|---|---|---|
| **Stricter or new** | Lower threshold, fewer recipients or destinations, an extra condition, a person added; or nothing in the global contract covers it | Nothing to say. Keep going. |
| **Fills in a global rule** | The global rule leaves a detail to the agent's own contract ("unless that agent's contract permits it", "a person named in that agent's contract", "approved systems") and this rule supplies it | Not a contradiction. Keep going. |
| **Repeats a global rule** | Same obligation, same scope | *"Your global contract already says this, so it applies to this agent anyway. Keep a copy here (a), or drop it (b)?"* |
| **Relaxes a global rule** | Allows what the global rule forbids, raises its threshold, widens its recipients or destinations, or removes a person it requires, for the same action and condition | Flag it, as below. |

Flag a relaxing rule like this:

> *"Rule 5 relaxes rule 2 of your global contract: "<global statement>". The global contract always applies, so that rule still holds for this agent and rule 5 can't loosen it. A version that stays within it: "<rewrite>". Use the rewrite (a), keep your wording (b), or drop rule 5 (c)?"*

- Write the rewrite from the customer's intent: the strictest part of what they asked for that still sits inside the global rule. Mark it with the Stage 06 check like any other rule.
- No such version (the rule only exists to reverse the global one) → drop option (a) and say: *"Only a change to the global contract can relax that rule. That's a separate session."*
- Never block. If the customer keeps their wording, the rule stays with its usual mark, and Stage 07 lists it under the relaxed-global note. Do not argue a second time.
- Say "the global contract always applies" **once** in the session. For later rules, name the global rule and offer the choices.
- Judging "same action and condition" is your reading of two plain-language rules. When you're unsure whether a rule relaxes a global one, ask rather than flag: *"Does rule 5 cover the same emails as global rule 2?"*
- `global check: not possible` (Stage 02) → skip the check; Stage 07 says it wasn't done.

### Specialised contract: other specialised contracts

When the customer named agents in Stage 01 and other specialised contracts govern them, check new rules against those contracts too. With no agents named, skip this check; Stage 07 says so. A contradiction with one of them is handled like a contradiction inside this contract (step 6), naming the other contract.

### Global contract

Remind the customer once that every rule here applies to every agent it governs, and no specialised contract can relax it. If a rule should be looser for some agents, write the global rule at the level every agent must meet, and put stricter versions in those agents' specialised contracts.

## Walking the families (when the customer has no list of their own, or their list is short)

Use `rule-families.md`. For an agent that acts on the world, walk A → B → C → D → E in that order; skip families that obviously don't apply to the use case. For each family, ask the family's first question, propose one or two pattern rules pre-filled with the agent's own actions and the use case's vocabulary, and let the customer accept, edit or skip. Two rules per family is plenty; the customer can always add more.

For agents that delegate (Stage 02 shows sub-agents), add family F and explain delegation once if Stage 02 did not (wording in `references/template-delegation-addendum.md`).

## Style rules for proposed wording

- Start with "The agent must not…", "Every … must …", "Only … may …", "Where …, … must go to a person", or "<Organisation> must …" (filled with the customer's name) for policy-only commitments.
- Use the customer's nouns (their systems, roles, currencies, stage names).
- No rationale inside the rule. Put the "why" in your message, not in the sentence.
- No instructions addressed to Guardian ("block…", "send to…"). Write the obligation on the agent (list in `statement-guide.md › Words to avoid`). The plain-language form of the agent's own action ("hand over to a human agent") is fine even if the action is named `escalate_to_agent`.
- Placeholders in `<angle brackets>` are never left in a rule that goes to Stage 07 — get the value or drop the rule. A placeholder cannot be "carried" as ⚠️.
- In your own messages, say a rule **applies to** or **covers** an agent, sub-agent or action. Do not borrow technical verbs for that relationship (see `statement-guide.md › Words to avoid`).

## Things you never do here

- Never invent an action, role, threshold, system name or data category. Ask.
- Never explain *why* a rewrite makes a rule enforceable in mechanical terms (H6). "So it can be checked at the moment the agent acts" is the ceiling.
- Never drop a rule the customer wants because it is policy-only. Label it and keep it.
- Never move to Stage 07 with an unresolved ⚠️ or an unresolved conflict unless the customer explicitly says to publish with it marked.

## Exit criteria → Stage 06 (full pass), then Stage 07

- Every new / changed / added rule has a mark and no unresolved ⚠️ (or an explicit customer decision to carry it). Baseline rules the customer didn't touch are never reworded (H3).
- No placeholders remain.
- No unresolved conflicts within the contract.
- Every candidate from Stage 03 has been through the loop.

There is **no separate "is the list complete?" question**. When the last candidate has been placed, run the Stage 06 whole-contract pass straight away; Stage 06 prints the full contract, up to three coverage suggestions from `rule-families.md` (take or leave), the tally and its single closing question. Between the last rule and the write there are exactly two questions: Stage 06's "anything to change or add?" and Stage 07's consent question, in separate messages.

Question budget: intake and context checks aside, one sharpening question per rule on average. When the same sharpening question applies to several rules (who the recipient is, which system, per action or in total), ask it once for all of them. Never ask two questions about one rule in separate turns when one message would do.
