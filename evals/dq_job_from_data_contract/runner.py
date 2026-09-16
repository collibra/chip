#!/usr/bin/env python3
"""Runs the dq-job-from-data-contract behavioral eval suite.

This is a skill-level suite, not a single-tool one (contrast with
evals/create_data_quality_job/runner.py): each trial drives the agent through a
whole skill-guided workflow across several tools
(pull_data_contract_manifest, get_asset_details, create_data_quality_job,
find_data_quality_rules, create_data_quality_rule, get_data_quality_rule_results),
plus the skill catalog tools (list_collibra_skills, load_collibra_skill) for the
trigger scenarios. For each scenario this:

  1. Builds the real chip binary (so the eval exercises the actual tool schemas/
     descriptions and the actual embedded skill catalog, not a re-implementation).
  2. Starts the in-process mock Collibra backend (fixtures/mock_collibra.py) on a
     free port.
  3. Runs the scenario's prompt through the Claude Agent SDK, with chip attached as
     a real MCP server (stdio) pointed at the mock backend, N times ("trials").
  4. Grades every trial's tool-call transcript - task scenarios with checks.py,
     trigger scenarios with trigger_checks.py.
  5. Prints a pass rate per scenario.

CRITICAL SAFETY RULE (carried over from the skill's own testing guide): this skill
performs live writes (jobs, rules) in the real product. Every trial's prompt is
run with an appended instruction forbidding confirm=true on any tool call, and
checks.py's `_confirm_never_true` independently verifies no call ever set it - a
single violation there is a hard defect, not sampling noise, exactly like
create_data_quality_job's confirm-gate scenarios.

Usage:
    pip install -r requirements.txt
    export ANTHROPIC_API_KEY=...
    python3 runner.py                              # everything: task + trigger
    python3 runner.py --suite task                 # only the 5 workflow scenarios
    python3 runner.py --suite trigger               # only the 20 trigger queries
    python3 runner.py --scenario email-full-flow    # a single named scenario
    python3 runner.py --trials 5                     # override every trial count
"""

from __future__ import annotations

import argparse
import asyncio
import socket
import subprocess
import sys
import tempfile
from pathlib import Path

import yaml

sys.path.insert(0, str(Path(__file__).parent))
from checks import CHECKS  # noqa: E402
from trigger_checks import CHECKS as TRIGGER_CHECKS  # noqa: E402
from fixtures.mock_collibra import serve as serve_mock  # noqa: E402

try:
    from claude_agent_sdk import AssistantMessage, ClaudeAgentOptions, ToolUseBlock, query
except ImportError:  # pragma: no cover
    print(
        "claude_agent_sdk is not installed. Run: pip install -r requirements.txt\n"
        "(This scaffold was written against the Python Claude Agent SDK - adjust the "
        "imports below if your installed version's API differs.)",
        file=sys.stderr,
    )
    raise

REPO_ROOT = Path(__file__).parents[2]
MCP_SERVER_LABEL = "chip"

# The full toolset this skill-guided workflow can touch, across both suites.
# Registered names confirmed against pkg/tools/*/tool.go and pkg/skills/*.go -
# not guessed. Trigger scenarios only ever need list_collibra_skills /
# load_collibra_skill, but giving every trial the same allowed_tools keeps
# run_trial() a single code path for both suites.
TOOL_NAMES = [
    "list_collibra_skills",
    "load_collibra_skill",
    "pull_data_contract_manifest",
    "get_asset_details",
    "create_data_quality_job",
    "find_data_quality_rules",
    "create_data_quality_rule",
    "get_data_quality_rule_results",
]
QUALIFIED_TOOL_NAMES = {name: f"mcp__{MCP_SERVER_LABEL}__{name}" for name in TOOL_NAMES}
QUALIFIED_TO_BARE = {q: bare for bare, q in QUALIFIED_TOOL_NAMES.items()}

# This skill performs live writes; every trial must stay in preview/dry-run mode.
# Appended to every prompt rather than passed via a dedicated system-prompt SDK
# field, since evals/create_data_quality_job/runner.py (the template this was
# adapted from) doesn't use one either - if your installed claude-agent-sdk
# version exposes ClaudeAgentOptions.system_prompt (or append_system_prompt),
# prefer that over string concatenation.
DRY_RUN_INSTRUCTION = (
    "\n\nImportant constraint for this session: you may call read-only tools freely, "
    "and you may call create_data_quality_job or create_data_quality_rule to preview "
    "(confirm omitted or confirm=false), but you must NEVER call any tool with "
    "confirm=true. Stop and describe your proposal instead of confirming a write."
)


def free_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


def build_chip_binary(dest_dir: Path) -> Path:
    binary_path = dest_dir / "chip"
    print(f"Building chip binary -> {binary_path} ...")
    subprocess.run(
        ["go", "build", "-o", str(binary_path), "./cmd/chip"],
        cwd=REPO_ROOT,
        check=True,
    )
    return binary_path


async def run_trial(binary_path: Path, mock_port: int, prompt: str) -> list[dict]:
    """Runs one trial and returns the transcript as a list of {"name", "input"}
    tool calls, across every tool in TOOL_NAMES (not just one) - a skill-guided
    workflow can call several of them in the same trial."""
    options = ClaudeAgentOptions(
        mcp_servers={
            MCP_SERVER_LABEL: {
                "type": "stdio",
                "command": str(binary_path),
                "args": [],
                "env": {
                    "COLLIBRA_MCP_API_URL": f"http://127.0.0.1:{mock_port}",
                    # Both the DQ tools and the skill catalog tools are opt-in
                    # experimental features (pkg/tools/register.go
                    # DataQualityFeatureName, pkg/skills/register.go FeatureName).
                    "COLLIBRA_MCP_EXPERIMENTAL": "data-quality,skills",
                },
            }
        },
        allowed_tools=list(QUALIFIED_TOOL_NAMES.values()),
        permission_mode="bypassPermissions",
    )

    calls: list[dict] = []
    async for message in query(prompt=prompt + DRY_RUN_INSTRUCTION, options=options):
        if isinstance(message, AssistantMessage):
            for block in message.content:
                if isinstance(block, ToolUseBlock) and block.name in QUALIFIED_TO_BARE:
                    calls.append({"name": QUALIFIED_TO_BARE[block.name], "input": block.input})
    return calls


async def run_task_scenario(binary_path: Path, mock_port: int, scenario: dict, trials_override: int | None) -> tuple[int, int, list[str]]:
    check_fn = CHECKS[scenario["check"]]
    trials = trials_override if trials_override is not None else scenario.get("trials", 3)
    passed = 0
    failure_reasons: list[str] = []

    for i in range(trials):
        calls = await run_trial(binary_path, mock_port, scenario["prompt"])
        result = check_fn(calls)
        if result.passed:
            passed += 1
        else:
            failure_reasons.append(f"trial {i + 1}: {result.reason}")

    return passed, trials, failure_reasons


async def run_trigger_scenario(binary_path: Path, mock_port: int, scenario: dict, trials_override: int | None) -> tuple[int, int, list[str]]:
    trials = trials_override if trials_override is not None else scenario.get("trials", 3)
    passed = 0
    failure_reasons: list[str] = []

    for i in range(trials):
        calls = await run_trial(binary_path, mock_port, scenario["query"])
        result = TRIGGER_CHECKS["trigger_check"](calls, should_trigger=scenario["should_trigger"])
        if result.passed:
            passed += 1
        else:
            failure_reasons.append(f"trial {i + 1}: {result.reason}")

    return passed, trials, failure_reasons


async def main_async(args: argparse.Namespace) -> int:
    task_scenarios = yaml.safe_load((Path(__file__).parent / "scenarios.yaml").read_text())
    trigger_scenarios = yaml.safe_load((Path(__file__).parent / "trigger_scenarios.yaml").read_text())

    if args.scenario:
        task_scenarios = [s for s in task_scenarios if s["name"] == args.scenario]
        trigger_scenarios = [s for s in trigger_scenarios if s["name"] == args.scenario]
        if not task_scenarios and not trigger_scenarios:
            print(f"No scenario named {args.scenario!r}", file=sys.stderr)
            return 1

    run_task = args.suite in ("task", "all") and task_scenarios
    run_trigger = args.suite in ("trigger", "all") and trigger_scenarios

    with tempfile.TemporaryDirectory() as tmp:
        binary_path = build_chip_binary(Path(tmp))
        mock_port = free_port()
        serve_mock(mock_port)
        print(f"Mock Collibra backend on http://127.0.0.1:{mock_port}\n")

        overall_ok = True

        if run_task:
            print("=" * 20 + " TASK SCENARIOS " + "=" * 20)
            for scenario in task_scenarios:
                print(f"=== {scenario['name']} ===")
                print(scenario["description"].strip())
                passed, total, reasons = await run_task_scenario(binary_path, mock_port, scenario, args.trials)
                rate = passed / total
                status = "PASS" if passed == total else ("FLAKY" if passed > 0 else "FAIL")
                if passed != total:
                    overall_ok = False
                print(f"-> {status}: {passed}/{total} trials passed ({rate:.0%})")
                for reason in reasons:
                    print(f"   - {reason}")
                print()

        if run_trigger:
            print("=" * 20 + " TRIGGER SCENARIOS " + "=" * 20)
            for scenario in trigger_scenarios:
                passed, total, reasons = await run_trigger_scenario(binary_path, mock_port, scenario, args.trials)
                rate = passed / total
                status = "PASS" if passed == total else ("FLAKY" if passed > 0 else "FAIL")
                if passed != total:
                    overall_ok = False
                trigger_label = "should-trigger" if scenario["should_trigger"] else "should-NOT-trigger"
                print(f"=== {scenario['name']} ({trigger_label}) ===")
                print(f"-> {status}: {passed}/{total} trials passed ({rate:.0%})")
                for reason in reasons:
                    print(f"   - {reason}")
                print()

        return 0 if overall_ok else 1


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--suite", choices=["task", "trigger", "all"], default="all", help="Which suite to run (default: all)")
    parser.add_argument("--scenario", help="Run only the named scenario (searches both suites)")
    parser.add_argument("--trials", type=int, help="Override every scenario's trial count")
    args = parser.parse_args()
    sys.exit(asyncio.run(main_async(args)))


if __name__ == "__main__":
    main()
