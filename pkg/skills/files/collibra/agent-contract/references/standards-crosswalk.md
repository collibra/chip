# Standards crosswalk — AIUC-1 and Microsoft AI Code of Conduct → rule patterns

Used by Stage 05 to annotate drafted rules and by Stage 04 when a customer asks "what does a standard say about this". Cite **id + title** for AIUC-1 and **section + clause number** for Microsoft. Do not reproduce the standards' text at length.

Sources:
- AIUC-1 — https://standard.aiuc-1.com (requirement ids as published; E007 and E014 are retired — do not cite).
- Microsoft Enterprise AI Services Code of Conduct v4.0, 2026-05-01 — https://learn.microsoft.com/en-us/legal/ai-code-of-conduct ("MS CoC" below; RAI = Responsible AI requirements; UR = Usage restrictions; AAS = Autonomous AI Systems).

Wording to the customer, always: **"relates to"**. Never "complies with", "satisfies", "required by".

---

## By rule family

| Family (rule-families.md) | AIUC-1 | MS CoC | Typical mark (internal — never shown) |
|---|---|---|---|
| A1 Special-category / personal data access | A003 Limit AI agent data access · A006 Prevent PII leakage | UR-11 no inferring sensitive attributes · RAI-8 notices and consents | ✅ |
| A2 Data-access scoping / minimisation | A003 · A005 Prevent cross-customer data exposure · B009 Limit output over-exposure | RAI-2 controls on inputs/outputs | ✅ |
| A3 Timing / stage gates | A003 · A001 Establish input data policy | RAI-2 | ✅ / 📋 |
| B1 Forbidden actions | B006 Prevent unauthorized AI agent actions · D003 Restrict unsafe tool calls | RAI-2 · AAS human controls for sensitive or irreversible actions | ✅ |
| B2 Destructive / write operations | B006 · B010 Promote secure patterns in generated code · D003 | RAI-2 · AAS | ✅ |
| B3 Volume / over-exposure caps | B009 Limit output over-exposure · D003 | RAI-2 | ✅ |
| B4 Monetary / authority thresholds | B006 · C005 Prevent agent-specific high risk outputs · C007 Flag high risk outputs for human review | UR-5 no consequential decisions without human oversight · AAS | ✅ |
| B5 Human-in-the-loop / escalation | C007 · C005 | UR-5 · UR-7 no exploiting vulnerability · RAI-5, RAI-7 meaningful human oversight · AAS | ✅ / 🟡 |
| B6 Destination / recipient allow-lists | A006 · B006 · A004 Protect IP & trade secrets | RAI-2 · RAI-9 access control | ✅ |
| B7 Ordering / prerequisites | D003 · D001 Prevent hallucinated outputs | AAS | 📋 (✅ if rewritten) |
| C1 Disclosure / transparency | E016 Implement AI disclosure mechanisms · D001 (provenance) | RAI-3 disclose AI-generated output, decisions, actions · AAS transparency | ✅ |
| C2 Confidential internals / scores | A004 · B009 | RAI-2 | ✅ / 🟡 |
| C3 Prohibited claims / promises | C003 Prevent harmful outputs · D001 | UR-6 no deception · Content: deception | ✅ / 🟡 |
| C4 Topic / scope rails | C004 Prevent out-of-scope outputs | RAI-2 · Content requirements (self-harm, violence, hate, sexual, CBRN, malicious cyber) | 🟡 |
| C5 Tone / bias / stereotyping | C003 (bias, deception) · C005 | UR-8 no social scoring / discriminatory profiling · Content: hate speech, harassment, discrimination | 🟡 |
| C6 Truthfulness / consistency | D001 · E015 Log AI system activity | RAI-3 · UR-6 | 🟡 |
| D1 Instruction override from user | B002 Detect adversarial input · B005 Implement real-time input filtering | RAI-2 | ✅ / 🟡 |
| D2 Instructions embedded in content | B002 · B005 | RAI-2 | 🟡 |
| D3 Secrets / credentials | A008 Prevent leakage of credentials and secrets | RAI-9 | ✅ |
| D4 Multi-step evasion | C003 · C005 | RAI-2 | 📋 |
| E1 Logging / audit | E015 Log AI system activity | AAS warn and remediate failures | 📋 |
| E2 Retention / residency | A001 · E005 Document data storage security · E011 Record processing locations | RAI-8 | 📋 |
| E3 Reviews / third-party testing | E008 Review internal processes · C010–C012, D002, D004 third-party testing · B001 | RAI-5 tested thoroughly, continuously | 📋 |
| E4 Training-data use | A001 · A004 | RAI-8 | 📋 |
| F Delegation (which agent may act) | A003 (agent identity and access, controls A003.2–A003.3) · B006 | AAS | ✅ / 📋 |

---

## AIUC-1 requirement ids used above (id · title · domain)

Data & Privacy — A001 Establish input data policy · A003 Limit AI agent data access · A004 Protect IP & trade secrets · A005 Prevent cross-customer data exposure · A006 Prevent PII leakage · A008 Prevent leakage of credentials and secrets
Security — B001 Third-party testing of adversarial robustness · B002 Detect adversarial input · B005 Implement real-time input filtering · B006 Prevent unauthorized AI agent actions · B009 Limit output over-exposure · B010 Promote secure patterns in generated code
Safety — C003 Prevent harmful outputs · C004 Prevent out-of-scope outputs · C005 Prevent agent-specific high risk outputs · C007 Flag high risk outputs for human review · C010 Third-party testing for harmful outputs · C011 Third-party testing for out-of-scope outputs · C012 Third-party testing for customer-defined risk
Reliability — D001 Prevent hallucinated outputs · D002 Third-party testing for hallucinations · D003 Restrict unsafe tool calls · D004 Third-party testing of tool calls
Accountability — E005 Document data storage security · E008 Review internal processes · E011 Record processing locations · E015 Log AI system activity · E016 Implement AI disclosure mechanisms
Society — F001 Prevent AI cyber misuse · F002 Prevent catastrophic misuse (relevant to C4 for free-text agents)

Other AIUC-1 ids (A002, A007, B003, B004, B007, B008, C001, C002, C006, C008, C009, E001–E004, E006, E009, E010, E012, E013, E017) are organisational or platform requirements with no agent-rule counterpart; do not cite them on rules.

---

## MS CoC clauses → the questions to ask the customer (Stage 05 step 4)

| Clause | Ask | Family |
|---|---|---|
| RAI-3 disclosure | "Which outputs reach people? Each should say it's automated." | C1 |
| RAI-5 / AAS oversight | "Which actions are sensitive or irreversible? Who can intervene?" | B4, B5 |
| RAI-6 feedback channels | "How does a person reach a human or report a problem? Should every message say so?" | C1 |
| UR-5 consequential decisions | "Does the agent decide anything about a person's legal, financial or employment position? Then a person must be in the loop." | B5 |
| UR-7 vulnerability | "If someone says they're in difficulty, should the matter go to a person?" | B5 |
| UR-8 profiling | "Could the agent rank or score people? On what must it never rank them?" | A1, C5 |
| UR-11 sensitive attributes | "Should the agent be forbidden from inferring gender, ethnicity, religion, age, health…?" | A1, C5 |
| UR-12 emotion inference | "Does the agent read tone or emotion? Should it be forbidden from inferring emotional state?" | C5 |
| UR-16/17 surveillance | "Could the agent be used to track a person over time? Forbid it explicitly?" | B1 |
| UR-21 impersonation | "Could the agent ever appear to be a specific person? Forbid it?" | C3 |
| Content requirements | "Does the agent write free text to people? Then topic rails for self-harm, sexual, violent, hateful, deceptive, cyber-attack and CBRN content." | C4 |
| RAI-9 access control | "Could the agent handle a credential? Forbid passing it anywhere." | D3 |

---

## Licensing note (internal to skill maintainers; not shown to customers)

AIUC-1 text is quoted only by id and title. Do not paste requirement statements or control text into customer-facing output. Microsoft's Code of Conduct is public documentation; paraphrase clause numbers, do not reproduce sections.
