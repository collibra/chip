"""Deterministic graders for the dq-job-from-data-contract task scenarios.

Each function takes the full ordered list of tool calls the agent made while
answering a scenario's `prompt` - each call is {"name": <tool name>, "input": <dict>}
- and returns a CheckResult. Grading reads fixture data (fixtures/mock_collibra.py)
as the source of truth for "valid"/"expected" values, so checks stay correct if the
fixture's fake data ever changes.

Unlike evals/create_data_quality_job/checks.py (one tool per scenario), a scenario
here is a whole skill-guided workflow, so most checks here look across SEVERAL
tools' calls in the same transcript (e.g. "did pull_data_contract_manifest run, and
did the later create_data_quality_job call steer to the manifest's connection").

Deliberately code-based, not LLM-graded: every property here (which tool was
called, which field a call set, whether a value matches the fixture, whether
`confirm` was ever true) is mechanically checkable from the transcript.
"""

from __future__ import annotations

from dataclasses import dataclass

from fixtures.mock_collibra import (
    CONNECTION_ID_POSTGRES,
    CONNECTION_ID_SNOWFLAKE,
    CONNECTION_NAME_POSTGRES,
    CONNECTION_NAME_SNOWFLAKE,
    DATA_CONTRACT_ID,
    DATE_LIKE_TYPES,
    EXISTING_JOB_NAME,
    TABLE_COLUMNS,
)

PULL_MANIFEST = "pull_data_contract_manifest"
GET_ASSET_DETAILS = "get_asset_details"
CREATE_DQ_JOB = "create_data_quality_job"
FIND_DQ_RULES = "find_data_quality_rules"
CREATE_DQ_RULE = "create_data_quality_rule"
GET_DQ_RULE_RESULTS = "get_data_quality_rule_results"


@dataclass
class CheckResult:
    passed: bool
    reason: str


def _calls(calls: list[dict], name: str) -> list[dict]:
    return [c for c in calls if c["name"] == name]


def _confirm_never_true(calls: list[dict]) -> str | None:
    """Returns a failure reason if any write-capable tool call ever set
    confirm=true - every scenario in this suite must stay in preview/dry-run
    mode (see README.md's safety rule, carried over from the skill's own
    testing guide)."""
    for call in calls:
        if call["name"] in (CREATE_DQ_JOB, CREATE_DQ_RULE) and call["input"].get("confirm") is True:
            return f"{call['name']} call set confirm=true - this suite must never write ({call['input']})"
    return None


def email_full_flow(calls: list[dict], **_) -> CheckResult:
    if (reason := _confirm_never_true(calls)) is not None:
        return CheckResult(False, reason)

    manifest_calls = _calls(calls, PULL_MANIFEST)
    if not manifest_calls:
        return CheckResult(False, f"agent never called {PULL_MANIFEST}")
    if not any(c["input"].get("dataContractId") == DATA_CONTRACT_ID for c in manifest_calls):
        return CheckResult(False, f"no {PULL_MANIFEST} call used the fixture's data contract id {DATA_CONTRACT_ID!r}")

    job_calls = _calls(calls, CREATE_DQ_JOB)
    for call in job_calls:
        conn = str(call["input"].get("connection", "")).strip()
        if conn and conn.lower() in {CONNECTION_ID_POSTGRES.lower(), CONNECTION_NAME_POSTGRES.lower()}:
            return CheckResult(
                False,
                f"create_data_quality_job steered to the mismatched Postgres connection {conn!r}, "
                f"not the manifest's Snowflake server ({CONNECTION_NAME_SNOWFLAKE})",
            )

    rule_calls = _calls(calls, CREATE_DQ_RULE)
    if not rule_calls:
        return CheckResult(False, "agent never proposed any contract-derived rules (no create_data_quality_rule calls)")
    for call in rule_calls:
        tolerance = call["input"].get("tolerance", 0)
        if tolerance not in (0, None):
            return CheckResult(False, f"rule {call['input'].get('monitorName')!r} used tolerance={tolerance}, expected 0 for a contract-enforcing rule")

    return CheckResult(True, "read the manifest, avoided the mismatched connection, and proposed tolerance-0 rules, all in preview")


def existing_job_reuse(calls: list[dict], **_) -> CheckResult:
    if (reason := _confirm_never_true(calls)) is not None:
        return CheckResult(False, reason)

    find_calls = _calls(calls, FIND_DQ_RULES)
    if not find_calls:
        return CheckResult(False, f"agent never called {FIND_DQ_RULES} to check for duplicates on the existing job")
    if not any(c["input"].get("jobName") == EXISTING_JOB_NAME for c in find_calls):
        return CheckResult(
            False,
            f"no {FIND_DQ_RULES} call was scoped to the existing job {EXISTING_JOB_NAME!r} - "
            "the duplicate check must target the job being reused",
        )

    job_calls = _calls(calls, CREATE_DQ_JOB)
    for call in job_calls:
        job_name = str(call["input"].get("jobName", "")).strip()
        if job_name and job_name != EXISTING_JOB_NAME:
            return CheckResult(
                False,
                f"create_data_quality_job was called for a new job {job_name!r} though an existing job "
                f"({EXISTING_JOB_NAME}) was available to reuse and the user asked to avoid a second job",
            )

    return CheckResult(True, "found the existing job, ran the duplicate check against it, and did not spin up a second job")


def verify_and_results(calls: list[dict], **_) -> CheckResult:
    if (reason := _confirm_never_true(calls)) is not None:
        return CheckResult(False, reason)

    rule_calls = _calls(calls, CREATE_DQ_RULE)
    if not rule_calls:
        return CheckResult(False, "agent never proposed contract-derived rules (no create_data_quality_rule calls)")
    for call in rule_calls:
        tolerance = call["input"].get("tolerance", 0)
        if tolerance not in (0, None):
            return CheckResult(False, f"rule {call['input'].get('monitorName')!r} used tolerance={tolerance}, expected 0")

    results_calls = _calls(calls, GET_DQ_RULE_RESULTS)
    if not results_calls:
        return CheckResult(
            False,
            f"agent never called {GET_DQ_RULE_RESULTS} - the user explicitly asked whether it's passing "
            "or failing, so the agent must attempt to read results (even if the honest answer is "
            "'no results yet, rules run on the job's next execution')",
        )

    return CheckResult(True, "proposed tolerance-0 rules and attempted to read results before answering pass/fail")


def timeslice_latest_only(calls: list[dict], **_) -> CheckResult:
    if (reason := _confirm_never_true(calls)) is not None:
        return CheckResult(False, reason)

    job_calls = _calls(calls, CREATE_DQ_JOB)
    for call in job_calls:
        table_name = call["input"].get("tableName")
        ts_col = call["input"].get("timeSliceColumn")
        if not ts_col or not table_name:
            continue
        ts_col = ts_col.strip()
        valid = {c["name"] for c in TABLE_COLUMNS.get(table_name, []) if c["type"] in DATE_LIKE_TYPES}
        if ts_col not in valid:
            return CheckResult(
                False,
                f"timeSliceColumn was set to {ts_col!r} on {table_name}, which has no genuine "
                f"date/timestamp column in the fixture (DATE_OF_HIRE is VARCHAR) - the agent invented "
                f"a time slice the table can't actually support",
            )

    return CheckResult(True, "never invented a time-slice column on a table with no genuine date/timestamp column")


def referential_integrity_clause_blocked(calls: list[dict], **_) -> CheckResult:
    if (reason := _confirm_never_true(calls)) is not None:
        return CheckResult(False, reason)

    rule_calls = _calls(calls, CREATE_DQ_RULE)

    for call in rule_calls:
        sql = str(call["input"].get("monitorValue", ""))
        if "JOIN" in sql.upper() or "DEPARTMENTS" in sql.upper():
            return CheckResult(
                False,
                f"proposed a cross-table rule referencing a second table (monitorValue={sql!r}) - "
                "referential integrity is not supported (no JOIN/subquery on a second table) and "
                "must be declined, not approximated",
            )

    single_table_cols = {"EMPLOYEE_ID", "EMAIL"}
    if not any(
        any(col in str(c["input"].get("monitorValue", "")).upper() for col in single_table_cols)
        or c["input"].get("columnName", "").upper() in single_table_cols
        for c in rule_calls
    ):
        return CheckResult(
            False,
            "agent declined the referential-integrity clause but also never proposed the contract's "
            "other single-table clauses (EMPLOYEE_ID / EMAIL) - it should still enforce what it can",
        )

    return CheckResult(True, "proposed the single-table contract clauses and never attempted a cross-table/JOIN rule for the referential-integrity clause")


CHECKS = {
    "email_full_flow": email_full_flow,
    "existing_job_reuse": existing_job_reuse,
    "verify_and_results": verify_and_results,
    "timeslice_latest_only": timeslice_latest_only,
    "referential_integrity_clause_blocked": referential_integrity_clause_blocked,
}
