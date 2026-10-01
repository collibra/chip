"""Deterministic grader for the trigger scenarios (does the skill's description
fire at the right times?).

Unlike checks.py (five distinct behaviors, five functions), all 20 trigger
scenarios are graded by the same rule: did the transcript contain a
load_collibra_skill call that loaded THIS skill
(skillName == "collibra/dq-job-from-data-contract")?

    should_trigger: true  -> pass iff that call happened.
    should_trigger: false -> pass iff that call did NOT happen. Loading some other
                              skill (e.g. collibra/dq-rule-workbench for a
                              contract-less DQ rule request, or collibra/discovery
                              for a lineage/glossary question) is fine and expected
                              for the near-miss queries - only loading THIS skill on
                              a should-not-trigger query is a failure.

Grading only the transcript (no LLM judge), matching checks.py's approach.

Note on the tool's actual argument name: the real load_collibra_skill tool (see
pkg/skills/load_tool.go) takes `skillName`, not `name` - if you're wiring this
grader against a different transcript shape, check that field name matches.
"""

from __future__ import annotations

from dataclasses import dataclass

LOAD_SKILL_TOOL = "load_collibra_skill"
SKILL_NAME = "collibra/dq-job-from-data-contract"


@dataclass
class CheckResult:
    passed: bool
    reason: str


def _loaded_this_skill(calls: list[dict]) -> bool:
    return any(
        c["name"] == LOAD_SKILL_TOOL and c["input"].get("skillName") == SKILL_NAME
        for c in calls
    )


def trigger_check(calls: list[dict], should_trigger: bool, **_) -> CheckResult:
    loaded = _loaded_this_skill(calls)
    if should_trigger:
        if loaded:
            return CheckResult(True, f"loaded {SKILL_NAME} as expected")
        return CheckResult(False, f"query should have triggered {SKILL_NAME} but no load_collibra_skill call named it")
    if loaded:
        return CheckResult(False, f"query should NOT have triggered {SKILL_NAME} but it was loaded anyway")
    return CheckResult(True, f"correctly did not load {SKILL_NAME} (loading a different/no skill is fine here)")


CHECKS = {"trigger_check": trigger_check}
