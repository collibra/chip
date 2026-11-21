#!/usr/bin/env python3
"""Local UI that runs the collibra-role-holders-to-group skill through Claude Code.

Two Claude runs, one approval between them:

1. Plan (read-only). Claude runs the skill's read steps and returns a structured plan. The write tools are
   denied on the command line, so nothing can be written in this phase, whatever the prompt says.
2. Write (after one approval). Claude carries out exactly the approved plan with the two tools it needs.

The plan the server executes is the one it stored itself; the browser only sends an approval token.
Standard library only. Listens on 127.0.0.1.
"""
import hmac
import http.server
import json
import os
import pathlib
import re
import secrets
import shutil
import subprocess
import threading
import time
import uuid

HOST = "127.0.0.1"
PORT = int(os.environ.get("PORT", "8765"))
# The executable is found on PATH and nothing in the environment can replace it. Tests set it in code.
CLAUDE_BIN = shutil.which("claude") or "claude"
RUN_TIMEOUT_SECONDS = int(os.environ.get("RUN_TIMEOUT_SECONDS", "420"))
STATIC_DIR = pathlib.Path(__file__).parent / "static"
MAX_BODY_BYTES = 16 * 1024
MAX_MAPPINGS = 5
UUID_RE = re.compile(r"^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$")

READ_TOOLS = [
    "mcp__chip__search_roles",
    "mcp__chip__search_user_groups",
    "mcp__chip__search_users",
    "mcp__chip__find_responsibilities",
    "mcp__chip__list_communities",
    "mcp__chip__list_domains",
]
# The only writes the approved plan needs. Everything else that writes stays denied in both phases.
WRITE_TOOLS_PHASE_2 = ["mcp__chip__create_user_group", "mcp__chip__add_users_to_user_group"]
DENIED_WRITE_TOOLS = [
    "mcp__chip__create_user_group",
    "mcp__chip__add_users_to_user_group",
    "mcp__chip__remove_users_from_user_group",
    "mcp__chip__rename_user_group",
    "mcp__chip__delete_user_group",
    "mcp__chip__create_user",
    "mcp__chip__change_user",
]

ACTIVITY = {
    "mcp__chip__search_roles": "Looking up roles",
    "mcp__chip__find_responsibilities": "Finding who holds the role",
    "mcp__chip__search_users": "Looking up users",
    "mcp__chip__search_user_groups": "Checking the user groups",
    "mcp__chip__list_communities": "Counting communities",
    "mcp__chip__list_domains": "Listing domains",
    "mcp__chip__create_user_group": "Creating a user group",
    "mcp__chip__add_users_to_user_group": "Adding users to a group",
    "Skill": "Loading the skill",
    "ToolSearch": "Loading tools",
    "StructuredOutput": "Preparing the result",
}

USER_SCHEMA = {
    "type": "object",
    "properties": {
        "id": {"type": "string"},
        "userName": {"type": "string"},
        "name": {"type": "string", "description": "First and last name"},
    },
    "required": ["id", "userName"],
}
SKIPPED_SCHEMA = {
    "type": "object",
    "properties": {"who": {"type": "string"}, "reason": {"type": "string"}},
    "required": ["who", "reason"],
}
PLAN_SCHEMA = {
    "type": "object",
    "properties": {
        "communitiesTotal": {"type": "integer", "description": "All communities in Collibra"},
        "communitiesWithAssignments": {"type": "integer", "description": "Distinct communities where any of the roles is assigned"},
        "mappings": {
            "type": "array",
            "items": {
                "type": "object",
                "properties": {
                    "role": {"type": "string"},
                    "roleFound": {"type": "boolean"},
                    "assignments": {"type": "integer"},
                    "communities": {"type": "integer", "description": "Distinct resources the role is assigned on"},
                    "holders": {"type": "array", "items": USER_SCHEMA, "description": "All distinct enabled people who hold the role"},
                    "group": {
                        "type": "object",
                        "properties": {
                            "name": {"type": "string"},
                            "exists": {"type": "boolean"},
                            "id": {"type": "string"},
                            "currentMembers": {"type": "integer"},
                        },
                        "required": ["name", "exists", "currentMembers"],
                    },
                    "toAdd": {"type": "array", "items": USER_SCHEMA},
                    "alreadyMembers": {"type": "array", "items": USER_SCHEMA},
                    "skipped": {"type": "array", "items": SKIPPED_SCHEMA},
                },
                "required": ["role", "roleFound", "assignments", "communities", "holders", "group", "toAdd", "alreadyMembers", "skipped"],
            },
        },
        "warnings": {"type": "array", "items": {"type": "string"}},
    },
    "required": ["communitiesTotal", "communitiesWithAssignments", "mappings", "warnings"],
}
RESULT_SCHEMA = {
    "type": "object",
    "properties": {
        "mappings": {
            "type": "array",
            "items": {
                "type": "object",
                "properties": {
                    "group": {"type": "string"},
                    "groupId": {"type": "string"},
                    "groupCreated": {"type": "boolean"},
                    "added": {"type": "integer"},
                    "membersAfter": {"type": "integer"},
                    "ok": {"type": "boolean"},
                    "error": {"type": "string"},
                },
                "required": ["group", "groupCreated", "added", "membersAfter", "ok"],
            },
        },
        "summary": {"type": "string"},
    },
    "required": ["mappings", "summary"],
}


def plan_prompt(mappings):
    return (
        "Use the collibra-role-holders-to-group skill, steps 1 to 4 only. This is a read-only analysis driven by a UI: "
        "the roles and group names are given below, so do not ask any questions. Do NOT write anything; the write tools "
        "are disabled. For each mapping: find the role (exact name, RESOURCE type); read ALL of its role assignments "
        "(find_responsibilities with limit 1000, and page while total is larger); collect the distinct user owners and "
        "count the distinct resources; do not expand group owners, mention them in warnings; resolve the users in "
        "batches of 50 with search_users (includeDisabled=true); put disabled users in skipped with reason 'disabled' and "
        "leave them out of holders and toAdd; check whether the target group exists (nameMatchMode EXACT) and, if so, "
        "count and list its members; then toAdd is the enabled holders who are not members yet, and alreadyMembers the "
        "ones who are. Count all communities in Collibra (list_communities with limit 1, read total) and the distinct "
        "communities where any of the roles is assigned. If a role does not exist, set roleFound to false with empty "
        "lists and explain it in warnings. Give each person as id, userName and name (first and last name). "
        "Return only the structured result.\n\nMappings (JSON): " + json.dumps(mappings)
    )


def write_prompt(plan):
    approved = [
        {
            "role": m["role"],
            "group": m["group"],
            "toAdd": [{"id": u["id"], "userName": u["userName"]} for u in m["toAdd"]],
        }
        for m in plan["mappings"]
    ]
    return (
        "Use the collibra-role-holders-to-group skill, steps 6 and 7. The user approved the plan below in the UI: that "
        "is the one approval. Carry out exactly this plan and nothing else. For each mapping: if group.exists is false, "
        "create the group with create_user_group (confirm=true; if the tool says the name is taken, use the existing "
        "group it names); then, if toAdd is not empty, add exactly those user ids with add_users_to_user_group "
        "(confirm=true); then check the result with search_users (groupId, includeDisabled=true, limit=1000) and report "
        "membersAfter as its total. Do not call any other tool that writes. If a step fails, stop that mapping and "
        "report ok=false with the error text. Return only the structured result.\n\nApproved plan (JSON): "
        + json.dumps({"mappings": approved, "groups": [{"name": m["group"]["name"], "exists": m["group"]["exists"], "id": m["group"].get("id", "")} for m in plan["mappings"]]})
    )


class Job:
    def __init__(self, kind, parent=None):
        self.id = uuid.uuid4().hex
        self.kind = kind
        self.parent = parent
        self.state = "running"
        self.events = []
        self.cond = threading.Condition()
        self.plan = None
        self.token = None
        self.approved = False

    def emit(self, event_type, data):
        with self.cond:
            self.events.append({"type": event_type, "data": data, "at": time.time()})
            self.cond.notify_all()

    def finish(self, state):
        with self.cond:
            self.state = state
            self.cond.notify_all()


JOBS = {}
JOBS_LOCK = threading.Lock()
ACTIVE = threading.Lock()


def clean_text(value, limit=80):
    if not isinstance(value, str):
        raise ValueError("expected text")
    value = value.strip()
    if not value or len(value) > limit or re.search(r"[\x00-\x1f\x7f]", value):
        raise ValueError("a name must be 1-%d characters without control characters" % limit)
    return value


def parse_mappings(body):
    rows = body.get("mappings")
    if not isinstance(rows, list) or not 1 <= len(rows) <= MAX_MAPPINGS:
        raise ValueError("give between 1 and %d role to group mappings" % MAX_MAPPINGS)
    mappings = []
    for row in rows:
        if not isinstance(row, dict):
            raise ValueError("each mapping needs a role and a group")
        mappings.append({"role": clean_text(row.get("role")), "group": clean_text(row.get("group"))})
    roles = [m["role"].lower() for m in mappings]
    groups = [m["group"].lower() for m in mappings]
    if len(set(roles)) != len(roles) or len(set(groups)) != len(groups):
        raise ValueError("each role and each group may appear only once")
    return mappings


def person(value):
    if not isinstance(value, dict) or not UUID_RE.match(str(value.get("id", ""))):
        raise ValueError("a person without a valid id")
    return {"id": value["id"], "userName": str(value.get("userName", "")), "name": str(value.get("name", ""))}


def validate_plan(raw, mappings):
    """Checks the structure Claude returned and works out the totals itself, so no arithmetic is taken on trust."""
    if not isinstance(raw, dict) or not isinstance(raw.get("mappings"), list):
        raise ValueError("the plan has no mappings")
    by_role = {str(m.get("role", "")).lower(): m for m in raw["mappings"] if isinstance(m, dict)}
    plan_mappings = []
    for wanted in mappings:
        m = by_role.get(wanted["role"].lower())
        if m is None:
            raise ValueError("the plan has nothing for the role %r" % wanted["role"])
        group = m.get("group") or {}
        exists = bool(group.get("exists"))
        group_id = str(group.get("id") or "")
        if exists and not UUID_RE.match(group_id):
            raise ValueError("the existing group %r has no valid id" % wanted["group"])
        to_add = [person(u) for u in m.get("toAdd", [])]
        already = [person(u) for u in m.get("alreadyMembers", [])]
        holders = [person(u) for u in m.get("holders", [])]
        if not bool(m.get("roleFound")):
            to_add = []
        plan_mappings.append({
            "role": wanted["role"],
            "roleFound": bool(m.get("roleFound")),
            "assignments": int(m.get("assignments", 0)),
            "communities": int(m.get("communities", 0)),
            "holders": holders,
            "group": {"name": wanted["group"], "exists": exists, "id": group_id, "currentMembers": int(group.get("currentMembers", 0))},
            "toAdd": to_add,
            "alreadyMembers": already,
            "skipped": [{"who": str(s.get("who", "")), "reason": str(s.get("reason", ""))} for s in m.get("skipped", []) if isinstance(s, dict)],
        })
    distinct_users = {u["id"] for m in plan_mappings for u in m["holders"]}
    to_add_pairs = sum(len(m["toAdd"]) for m in plan_mappings)
    groups_to_create = sum(1 for m in plan_mappings if m["roleFound"] and not m["group"]["exists"] and m["toAdd"])
    return {
        "stats": {
            "communitiesTotal": int(raw.get("communitiesTotal", 0)),
            "communitiesWithAssignments": int(raw.get("communitiesWithAssignments", 0)),
            "assignments": sum(m["assignments"] for m in plan_mappings),
            "usersFound": len(distinct_users),
            "membershipsToAdd": to_add_pairs,
            "groupsToCreate": groups_to_create,
        },
        "mappings": plan_mappings,
        "warnings": [str(w) for w in raw.get("warnings", [])],
    }


def run_claude(job, prompt, schema, allowed_tools):
    command = [
        CLAUDE_BIN, "-p", prompt,
        "--output-format", "stream-json", "--verbose",
        "--json-schema", json.dumps(schema),
        "--allowedTools", *allowed_tools,
        "--disallowedTools", *[t for t in DENIED_WRITE_TOOLS if t not in allowed_tools],
    ]
    process = subprocess.Popen(command, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
    timer = threading.Timer(RUN_TIMEOUT_SECONDS, process.kill)
    timer.start()
    structured = None
    failure = None
    try:
        for line in process.stdout:
            try:
                event = json.loads(line)
            except ValueError:
                continue
            kind = event.get("type")
            if kind == "assistant":
                for block in event.get("message", {}).get("content", []):
                    if block.get("type") == "tool_use":
                        name = block.get("name", "")
                        job.emit("activity", {"tool": name, "label": ACTIVITY.get(name, "Working")})
            elif kind == "result":
                if event.get("is_error") or event.get("subtype") != "success":
                    failure = str(event.get("result") or event.get("subtype"))
                else:
                    structured = event.get("structured_output")
                    if structured is None:
                        failure = "Claude finished without a structured result"
        process.wait()
    finally:
        timer.cancel()
    if process.returncode not in (0, None) and structured is None and failure is None:
        failure = "Claude stopped with exit code %s (or took longer than %d seconds)" % (process.returncode, RUN_TIMEOUT_SECONDS)
    return structured, failure


def plan_worker(job, mappings):
    try:
        raw, failure = run_claude(job, plan_prompt(mappings), PLAN_SCHEMA, READ_TOOLS + ["Skill"])
        if failure:
            job.emit("error", {"message": failure})
            job.finish("failed")
            return
        plan = validate_plan(raw, mappings)
        job.plan = plan
        job.token = secrets.token_hex(16)
        job.emit("plan", {"plan": plan, "approvalNeeded": plan["stats"]["membershipsToAdd"] > 0 or plan["stats"]["groupsToCreate"] > 0})
        job.finish("awaiting_approval" if plan["stats"]["membershipsToAdd"] > 0 or plan["stats"]["groupsToCreate"] > 0 else "done")
    except (ValueError, TypeError) as error:
        job.emit("error", {"message": "The plan could not be used: %s" % error})
        job.finish("failed")
    except Exception as error:  # a worker thread must always report back
        job.emit("error", {"message": "Unexpected problem: %s" % error})
        job.finish("failed")
    finally:
        ACTIVE.release()


def write_worker(job, plan):
    try:
        raw, failure = run_claude(job, write_prompt(plan), RESULT_SCHEMA, READ_TOOLS + WRITE_TOOLS_PHASE_2 + ["Skill"])
        if failure:
            job.emit("error", {"message": failure})
            job.finish("failed")
            return
        job.emit("result", {"result": raw})
        job.finish("done")
    except Exception as error:
        job.emit("error", {"message": "Unexpected problem: %s" % error})
        job.finish("failed")
    finally:
        ACTIVE.release()


def start_plan(mappings):
    if not ACTIVE.acquire(blocking=False):
        return None
    job = Job("plan")
    with JOBS_LOCK:
        JOBS[job.id] = job
    threading.Thread(target=plan_worker, args=(job, mappings), daemon=True).start()
    return job


def approve(job_id, token):
    with JOBS_LOCK:
        plan_job = JOBS.get(job_id)
    if plan_job is None or plan_job.kind != "plan":
        return None, 404, "unknown plan"
    if plan_job.state != "awaiting_approval" or plan_job.plan is None:
        return None, 409, "this plan is not waiting for approval"
    if not hmac.compare_digest(str(token or ""), plan_job.token or ""):
        return None, 403, "wrong approval token"
    with plan_job.cond:
        if plan_job.approved:
            return None, 409, "this plan was already approved"
        plan_job.approved = True
        plan_job.state = "approved"
    if not ACTIVE.acquire(blocking=False):
        with plan_job.cond:
            plan_job.approved = False
            plan_job.state = "awaiting_approval"
        return None, 409, "another run is in progress"
    job = Job("write", parent=plan_job.id)
    with JOBS_LOCK:
        JOBS[job.id] = job
    threading.Thread(target=write_worker, args=(job, plan_job.plan), daemon=True).start()
    return job, 200, ""


class Handler(http.server.BaseHTTPRequestHandler):
    server_version = "RoleGroupUI"

    def log_message(self, fmt, *args):
        pass

    def _allowed_host(self):
        host = (self.headers.get("Host") or "").lower()
        return host in {"127.0.0.1:%d" % PORT, "localhost:%d" % PORT}

    def _send(self, status, body, content_type="application/json"):
        data = body if isinstance(body, bytes) else json.dumps(body).encode()
        self.send_response(status)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(data)))
        self.send_header("Cache-Control", "no-store")
        self.send_header("X-Content-Type-Options", "nosniff")
        self.send_header("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; frame-ancestors 'none'")
        self.end_headers()
        self.wfile.write(data)

    def _body(self):
        length = int(self.headers.get("Content-Length") or 0)
        if length > MAX_BODY_BYTES:
            raise ValueError("request too large")
        return json.loads(self.rfile.read(length) or b"{}")

    def _same_origin(self):
        origin = self.headers.get("Origin")
        return origin is None or origin in {"http://127.0.0.1:%d" % PORT, "http://localhost:%d" % PORT}

    def do_GET(self):
        if not self._allowed_host():
            return self._send(403, {"error": "forbidden host"})
        path = self.path.split("?")[0]
        if path in ("/", "/index.html"):
            return self._send(200, (STATIC_DIR / "index.html").read_bytes(), "text/html; charset=utf-8")
        for name, content_type in (("app.js", "text/javascript; charset=utf-8"), ("style.css", "text/css; charset=utf-8")):
            if path == "/" + name:
                return self._send(200, (STATIC_DIR / name).read_bytes(), content_type)
        match = re.fullmatch(r"/api/jobs/([0-9a-f]{32})/events", path)
        if match:
            return self._stream(match.group(1))
        if path == "/api/health":
            return self._send(200, {"ok": True})
        self._send(404, {"error": "not found"})

    def do_POST(self):
        if not self._allowed_host() or not self._same_origin():
            return self._send(403, {"error": "forbidden"})
        try:
            body = self._body()
            if self.path == "/api/plan":
                job = start_plan(parse_mappings(body))
                if job is None:
                    return self._send(409, {"error": "another run is in progress"})
                return self._send(202, {"jobId": job.id})
            match = re.fullmatch(r"/api/jobs/([0-9a-f]{32})/approve", self.path)
            if match:
                job, status, message = approve(match.group(1), body.get("token"))
                if job is None:
                    return self._send(status, {"error": message})
                return self._send(202, {"jobId": job.id})
        except (ValueError, json.JSONDecodeError) as error:
            return self._send(400, {"error": str(error)})
        self._send(404, {"error": "not found"})

    def _stream(self, job_id):
        with JOBS_LOCK:
            job = JOBS.get(job_id)
        if job is None:
            return self._send(404, {"error": "unknown job"})
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Cache-Control", "no-store")
        self.send_header("Connection", "close")
        self.end_headers()
        sent = 0
        try:
            while True:
                with job.cond:
                    while sent >= len(job.events) and job.state == "running":
                        job.cond.wait(timeout=15)
                        if sent >= len(job.events) and job.state == "running":
                            self.wfile.write(b": keep-alive\n\n")
                            self.wfile.flush()
                    pending = job.events[sent:]
                    state = job.state
                for event in pending:
                    payload = dict(event["data"])
                    if event["type"] == "plan":
                        payload["token"] = job.token
                        payload["planJobId"] = job.id
                    self.wfile.write(("event: %s\ndata: %s\n\n" % (event["type"], json.dumps(payload))).encode())
                    sent += 1
                self.wfile.flush()
                if state != "running" and sent >= len(job.events):
                    self.wfile.write(("event: end\ndata: %s\n\n" % json.dumps({"state": state})).encode())
                    self.wfile.flush()
                    return
        except (BrokenPipeError, ConnectionResetError):
            return


def main():
    server = http.server.ThreadingHTTPServer((HOST, PORT), Handler)
    print("Role holders to group: http://%s:%d" % (HOST, PORT))
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass


if __name__ == "__main__":
    main()
