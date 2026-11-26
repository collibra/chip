import http.client, json, os, pathlib, socket, sys, tempfile, threading, time, unittest

HERE = pathlib.Path(__file__).parent
sock = socket.socket(); sock.bind(("127.0.0.1", 0)); PORT = sock.getsockname()[1]; sock.close()
LOG = str(HERE / "calls.log")
os.environ.update({"PORT": str(PORT), "RUN_TIMEOUT_SECONDS": "30"})
os.chmod(HERE / "fake_claude.py", 0o755)
sys.path.insert(0, str(HERE.parent))
import server  # noqa: E402

server.CLAUDE_BIN = str(HERE / "fake_claude.py")

httpd = server.http.server.ThreadingHTTPServer(("127.0.0.1", PORT), server.Handler)
threading.Thread(target=httpd.serve_forever, daemon=True).start()
MAPPINGS = [{"role": "Product Owner", "group": "Product Owners"}, {"role": "Business Analyst", "group": "Business Analysts"}]


def request(method, path, body=None, headers=None):
    conn = http.client.HTTPConnection("127.0.0.1", PORT, timeout=20)
    conn.request(method, path, json.dumps(body) if body is not None else None, {"Content-Type": "application/json", **(headers or {})})
    response = conn.getresponse()
    data = response.read()
    return response.status, data


def events(job_id):
    status, data = request("GET", "/api/jobs/%s/events" % job_id)
    out = []
    for chunk in data.decode().split("\n\n"):
        lines = [l for l in chunk.splitlines() if not l.startswith(":")]
        if len(lines) == 2:
            out.append((lines[0][7:], json.loads(lines[1][6:])))
    return out


def calls():
    with open(LOG) as handle:
        return [json.loads(line)["args"] for line in handle]


class RoleGroupServer(unittest.TestCase):
    def setUp(self):
        open(LOG, "w").close()

    def start_plan(self):
        for _ in range(100):
            status, data = request("POST", "/api/plan", {"mappings": MAPPINGS})
            if status == 202:
                return json.loads(data)["jobId"]
            time.sleep(0.1)
        self.fail("could not start a plan")

    def planned(self):
        job = self.start_plan()
        evs = dict(events(job))
        return job, evs["plan"]

    def test_plan_phase_can_not_write_and_stats_are_computed_by_the_server(self):
        job, plan = self.planned()
        args = calls()[0]
        allowed = args[args.index("--allowedTools") + 1: args.index("--disallowedTools")]
        denied = args[args.index("--disallowedTools") + 1:]
        for tool in ("create_user_group", "add_users_to_user_group", "remove_users_from_user_group", "delete_user_group", "create_user", "change_user", "rename_user_group"):
            self.assertIn("mcp__chip__" + tool, denied)
            self.assertNotIn("mcp__chip__" + tool, allowed)
        stats = plan["plan"]["stats"]
        self.assertEqual(stats["usersFound"], 4)            # holders are 1, 2, 7 and 7, 8: user 7 counts once
        self.assertEqual(stats["membershipsToAdd"], 4)
        self.assertEqual(stats["groupsToCreate"], 1)
        self.assertEqual(stats["communitiesTotal"], 75)
        self.assertTrue(plan["token"])

    def test_approval_needs_the_right_token_and_works_once_with_only_the_two_write_tools(self):
        job, plan = self.planned()
        status, _ = request("POST", "/api/jobs/%s/approve" % job, {"token": "wrong"})
        self.assertEqual(status, 403)
        self.assertEqual(len(calls()), 1, "a wrong token must not start a run")
        status, data = request("POST", "/api/jobs/%s/approve" % job, {"token": plan["token"]})
        self.assertEqual(status, 202)
        write_job = json.loads(data)["jobId"]
        evs = dict(events(write_job))
        self.assertTrue(evs["result"]["result"]["mappings"][0]["ok"])
        args = calls()[1]
        allowed = args[args.index("--allowedTools") + 1: args.index("--disallowedTools")]
        denied = args[args.index("--disallowedTools") + 1:]
        self.assertIn("mcp__chip__create_user_group", allowed)
        self.assertIn("mcp__chip__add_users_to_user_group", allowed)
        for tool in ("remove_users_from_user_group", "delete_user_group", "create_user", "change_user", "rename_user_group"):
            self.assertIn("mcp__chip__" + tool, denied)
        prompt = args[args.index("-p") + 1]
        self.assertIn("00000000-0000-0000-0000-000000000008", prompt)       # the stored plan, not anything from the browser
        status, _ = request("POST", "/api/jobs/%s/approve" % job, {"token": plan["token"]})
        self.assertEqual(status, 409, "a plan can be approved only once")

    def test_input_validation(self):
        for bad in ({}, {"mappings": []}, {"mappings": [{"role": "", "group": "x"}]},
                    {"mappings": [{"role": "a", "group": "x"}, {"role": "A", "group": "y"}]},
                    {"mappings": [{"role": "a\nb", "group": "x"}]}, {"mappings": [{"role": "a" * 200, "group": "x"}]},
                    {"mappings": [{"role": str(i), "group": str(i)} for i in range(9)]}):
            status, _ = request("POST", "/api/plan", bad)
            self.assertEqual(status, 400, bad)
        self.assertEqual(len(calls()), 0)

    def test_foreign_hosts_and_origins_are_refused(self):
        self.assertEqual(request("GET", "/", headers={"Host": "evil.example"})[0], 403)
        self.assertEqual(request("POST", "/api/plan", {"mappings": MAPPINGS}, {"Origin": "http://evil.example"})[0], 403)
        self.assertEqual(len(calls()), 0)

    def test_page_is_served_with_a_strict_content_security_policy(self):
        conn = http.client.HTTPConnection("127.0.0.1", PORT); conn.request("GET", "/")
        response = conn.getresponse(); response.read()
        self.assertEqual(response.status, 200)
        self.assertIn("default-src 'self'", response.getheader("Content-Security-Policy"))


if __name__ == "__main__":
    unittest.main()
