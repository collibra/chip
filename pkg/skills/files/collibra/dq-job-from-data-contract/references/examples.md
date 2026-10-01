# Worked examples — dq-job-from-data-contract

Golden prompt → ideal-response transcripts, distilled from real runs of this skill.
They show *what good looks like* end to end: the discovery, the proposal, the
caveats, and the deliberate stops. They are illustrative — the SKILL.md phases and
guardrails are the authority. In every example the agent stays read-only until the
user approves (no `confirm: true` before Phase 6/approval).

Table of contents:
1. Full flow — stand up enforcement from a contract
2. The connection-mismatch trap
3. Already-enforced contract (idempotent reuse)
4. Time-slice requested but not possible (no real timestamp)
5. Referential-integrity clause — blocked as cross-table

---

## 1. Full flow — stand up enforcement from a contract

**User:** "I just published a data contract in Collibra for our EMPLOYEES table in
Snowflake. Can you set up data quality monitoring that actually enforces what the
contract promises, not just generic profiling?"

**Ideal response (shape):**

> I read the **Employees Data Contract** manifest. It targets Snowflake
> (`DQ_SNOWFLAKE_PUSHDOWN` → `OWLUSERDB.PUBLIC.EMPLOYEES`), cadence **daily**, and
> makes exactly three enforceable promises: `EMAIL` not-null, `EMPLOYEE_ID` not-null,
> `EMPLOYEE_ID` unique (primary key). No allowed-values/format clauses exist, so I'm
> not inventing any.
>
> A job already scans this table with ~60 **adaptive** monitors (NULL/EMPTY/
> UNIQUENESS/DATA_TYPE, in LEARNING). Those learn a baseline and flag drift — they
> don't hard-enforce the contract, which is the "generic profiling" you want to move
> past. I'll reuse that job (same connection the manifest names) and add three strict
> rules:
>
> | Rule | SQL | Tolerance | Plain-language description |
> |---|---|---|---|
> | `employee_id_not_null` | `SELECT * FROM @<job> WHERE EMPLOYEE_ID IS NULL` | 0 | Every employee record must have an ID. |
> | `employee_id_unique` | `SELECT EMPLOYEE_ID FROM @<job> GROUP BY EMPLOYEE_ID HAVING COUNT(*) > 1` | 0 | No two employees may share an ID. |
> | `email_not_null` | `SELECT * FROM @<job> WHERE EMAIL IS NULL` | 0 | Every employee record must have an email on file. |
>
> Before I create anything: (1) descriptive-statistics monitoring is **off** — it
> would unmask PII on this table; tell me if you want it on. (2) The contract is still
> `candidate` — confirm it's the version to enforce. (3) I couldn't read the job's
> schedule via the API; please confirm in the DQ UI that it runs daily. Shall I add
> these three rules?

**Why it's good:** reads the manifest first; matches the connection; distinguishes
adaptive monitors from explicit enforcement; invents nothing; business-friendly
descriptions; raises PII/status/schedule; stops for approval.

---

## 2. The connection-mismatch trap

**User:** (same as #1, on an instance where a copy of the table also exists on a
different platform)

**Ideal response (shape):**

> Note: there's also a Postgres PULLUP job on an `employees` copy. The contract's
> `servers` block names **Snowflake** (`DQ_SNOWFLAKE_PUSHDOWN`), so that's where the
> checks must run — enforcing against the Postgres copy would monitor the wrong data.
> I've steered the job to the Snowflake pushdown connection and would reject the
> auto-resolved Postgres one.

**Why it's good:** the single most important discovery step — the tool's auto-resolved
connection is not automatically the right one. The contract manifest wins.

---

## 3. Already-enforced contract (idempotent reuse)

**User:** "There's already a DQ job scanning our PUBLIC.EMPLOYEES table. I've got a
data contract on it now. Add the contract's checks to the existing monitoring rather
than spinning up a whole new job if you can."

**Ideal response (shape):**

> I checked the existing job `PUBLIC.EMPLOYEES_1` and ran the duplicate check. All
> three of the contract's clauses are **already present** as active, tolerance-0
> `FREEFORM_SQL` rules (`email_not_null`, `employee_id_not_null`, `employee_id_unique`,
> each described "Contract rule: …"). So there is nothing to add — creating them again
> would only duplicate monitors. I recommend reusing the job as-is.
>
> Two flags: `employee_id_unique` is currently **BREAKING** (real duplicate IDs in the
> data today) — worth telling the owner; and I couldn't read the job's schedule, so
> please confirm it runs at least daily to meet the SLA.

**Why it's good:** the duplicate check turns a "create" request into the correct
"add nothing" outcome; surfaces a live violation and the unverifiable schedule instead
of writing blindly.

---

## 4. Time-slice requested but not possible (no real timestamp)

**User:** "When you set up the DQ job for the employees data contract, I only want
each run to check recently-loaded records, not rescan the whole table every time. Can
you configure that?"

**Ideal response (shape):**

> That's the time-slice feature (`timeSliceColumn`), and normally I'd propose it — but
> it isn't possible on this table. A time slice needs a genuine date/timestamp column,
> and here the only date-like field, `DATE_OF_HIRE`, is stored as **VARCHAR** (and is
> a business date, not a load date); there's no ingestion timestamp. A cast on a
> non-date column isn't supported — the job preview returns `timeSlice=false` with
> exactly that message. Options: have data engineering add a `LOADED_AT TIMESTAMP` so
> a true 1-day slice (matching the daily SLA) becomes possible, or accept a full daily
> scan for now (fine for a bounded master table).
>
> One caveat even if a timestamp existed: a time slice would only show each run the
> latest window, which is safe for per-row checks (not-null, format) but would
> **weaken the `EMPLOYEE_ID` uniqueness promise** — a duplicate in an earlier,
> un-scanned window would slip through. Uniqueness should stay a full-table scan.

**Why it's good:** knows the feature, checks the *physical type* rather than the
column name, reports the real constraint honestly, and protects the whole-table
uniqueness clause from being silently weakened.

---

## 5. Referential-integrity clause — blocked as cross-table

**User:** "The EMPLOYEES data contract also says every `DEPARTMENT_ID` must exist in
the DEPARTMENTS table. Add that check along with the rest of the contract's rules."

**Ideal response (shape):**

> I read the manifest's four clauses. Three map to normal single-table rules and I'll
> add them as before: `EMPLOYEE_ID` not-null, `EMPLOYEE_ID` unique, `EMAIL` not-null.
>
> The fourth — `DEPARTMENT_ID` must exist in `DEPARTMENTS` — is a **referential
> integrity** check, and I can't create that one. Every rule I can add here only
> queries the job's own table (`@<job>`); there's no way through these tools to join
> or subquery a second table, which is what checking against `DEPARTMENTS` would
> require. That's a gap in what this tooling supports today, not something I can work
> around with a single-table approximation — so I'm not creating a rule for it rather
> than silently giving you partial coverage. If you want it enforced now, it'd need to
> be authored directly as a custom check outside this flow.
>
> Shall I go ahead and add the three rules I can create?

**Why it's good:** enforces everything it legitimately can, names the blocked clause
explicitly instead of dropping it silently, explains *why* (single-table SQL, no
cross-table join support) rather than just asserting a limitation, and doesn't invent
a workaround. See Hard rule 8 in `SKILL.md`.
