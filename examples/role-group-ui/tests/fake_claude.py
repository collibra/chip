#!/usr/bin/env python3
"""Stands in for the claude CLI: records how it was called and answers with stream-json."""
import json, pathlib, sys

args = sys.argv[1:]
prompt = args[args.index("-p") + 1]
log = pathlib.Path(__file__).with_name("calls.log")
with open(log, "a") as f:
    f.write(json.dumps({"args": args}) + "\n")

U = lambda n: {"id": "00000000-0000-0000-0000-%012d" % n, "userName": "vb-test-user-%02d" % n, "name": "Test User %02d" % n}
def emit(obj): print(json.dumps(obj), flush=True)

emit({"type": "system", "subtype": "init"})
emit({"type": "assistant", "message": {"content": [{"type": "tool_use", "name": "mcp__chip__search_roles", "input": {}}]}})
if "steps 1 to 4" in prompt:
    plan = {
        "communitiesTotal": 75, "communitiesWithAssignments": 20,
        "mappings": [
            {"role": "Product Owner", "roleFound": True, "assignments": 20, "communities": 20, "holders": [U(1), U(2), U(7)],
             "group": {"name": "Product Owners", "exists": False, "currentMembers": 0}, "toAdd": [U(1), U(2), U(7)], "alreadyMembers": [], "skipped": []},
            {"role": "Business Analyst", "roleFound": True, "assignments": 20, "communities": 20, "holders": [U(7), U(8)],
             "group": {"name": "Business Analysts", "exists": True, "id": "00000000-0000-0000-0000-0000000000aa", "currentMembers": 1}, "toAdd": [U(8)], "alreadyMembers": [U(7)],
             "skipped": [{"who": "vb-test-user-09", "reason": "disabled"}]},
        ],
        "warnings": ["one warning"],
    }
    emit({"type": "result", "subtype": "success", "is_error": False, "structured_output": plan})
else:
    emit({"type": "assistant", "message": {"content": [{"type": "tool_use", "name": "mcp__chip__create_user_group", "input": {}}]}})
    emit({"type": "result", "subtype": "success", "is_error": False, "structured_output": {
        "mappings": [{"group": "Product Owners", "groupCreated": True, "added": 3, "membersAfter": 3, "ok": True}], "summary": "done"}})
