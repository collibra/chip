"""A minimal, dependency-free stand-in for the Collibra backend that the
dq-job-from-data-contract skill drives across several tools.

Unlike evals/create_data_quality_job/fixtures/mock_collibra.py (one tool, one
mock), this skill chains multiple tools in sequence:

    list_collibra_skills / load_collibra_skill  -> in-process catalog, no HTTP
    pull_data_contract_manifest                 -> GET /rest/dataProduct/v1/dataContracts/{id}/activeVersion/manifest
                                                     (pkg/clients/dgc_client.go PullActiveDataContractManifest)
    get_asset_details                           -> POST /graphql/knowledgeGraph/v1
                                                     (pkg/clients/dgc_client.go GetAssetSummary,
                                                     pkg/clients/dgc_gql_client.go CreateAssetDetailsGraphQLQuery)
    create_data_quality_job                     -> discovery GETs + POST /rest/dq/1.0/jobs
                                                     (pkg/tools/create_dq_job/discover.go, pkg/clients/dgc_dq_client.go)
    find_data_quality_rules                     -> POST /rest/dq/internal/v1/monitoring/monitors/dashboard
                                                     (pkg/clients/dq_rule_search_client.go FindDQRules)
    create_data_quality_rule                    -> POST /rest/dq/internal/v1/monitoring/monitor
                                                     (pkg/clients/dq_rules_client.go CreateDQRule)
    get_data_quality_rule_results                -> GET /rest/dq/internal/v1/monitoring/rules/{jobName}/{ruleName}
                                                     (pkg/clients/dq_rules_client.go GetDQRuleResults)

The endpoints reused wholesale from create_data_quality_job's mock are the
connection/dataSource/schema/table/column discovery GETs and the job
name-collision / create POST. Everything else here is new.

Fixture story (mirrors the skill's own worked example in
pkg/skills/files/collibra/dq-job-from-data-contract/references/examples.md):
an EMPLOYEES table on a Snowflake-like PUSHDOWN connection, governed by an
active Data Contract whose manifest states three enforceable clauses
(EMAIL not-null, EMPLOYEE_ID not-null + unique) plus one referential-integrity
clause (DEPARTMENT_ID must exist in DEPARTMENTS) that the skill must decline
to enforce (no cross-table rule support). EMPLOYEES' only date-like column
(DATE_OF_HIRE) is stored as VARCHAR, so there is deliberately no genuine load
timestamp - this is what makes the time-slice-latest-only scenario correctly
conclude slicing isn't possible.

A second, wrong-platform connection (DQ_POSTGRES_PULLUP, fronting a Postgres
copy of the table) is included to exercise the connection-mismatch trap
mentioned in the skill's rule 2 - kept to a single extra connection/table/
column set, not a full parallel fixture, to avoid ballooning this file.

Run standalone for manual poking:
    python3 mock_collibra.py 8812
"""

from __future__ import annotations

import json
import re
import sys
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlparse

# ---------------------------------------------------------------------------
# Connections
# ---------------------------------------------------------------------------

CONNECTION_ID_SNOWFLAKE = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
EDGE_SITE_ID_SNOWFLAKE = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
CONNECTION_NAME_SNOWFLAKE = "DQ_SNOWFLAKE_PUSHDOWN"

CONNECTION_ID_POSTGRES = "cccccccc-cccc-cccc-cccc-cccccccccccc"
EDGE_SITE_ID_POSTGRES = "dddddddd-dddd-dddd-dddd-dddddddddddd"
CONNECTION_NAME_POSTGRES = "DQ_POSTGRES_PULLUP"

CONNECTIONS = [
    {
        "connectionId": CONNECTION_ID_SNOWFLAKE,
        "connectionName": CONNECTION_NAME_SNOWFLAKE,
        "capabilityTypes": ["PUSHDOWN"],
        "databaseProductName": "SNOWFLAKE",
        "edgeSiteId": EDGE_SITE_ID_SNOWFLAKE,
        "edgeSiteName": "EVAL-EDGE-SNOWFLAKE",
    },
    {
        # Wrong-platform copy of the table - present so a careless discovery walk
        # could plausibly resolve here instead of the manifest's own connection.
        "connectionId": CONNECTION_ID_POSTGRES,
        "connectionName": CONNECTION_NAME_POSTGRES,
        "capabilityTypes": ["PULLUP"],
        "databaseProductName": "POSTGRES",
        "edgeSiteId": EDGE_SITE_ID_POSTGRES,
        "edgeSiteName": "EVAL-EDGE-POSTGRES",
    },
]

DATA_SOURCES_BY_CONNECTION = {
    CONNECTION_ID_SNOWFLAKE: [{"dataSourceName": "OWLUSERDB", "supportsSchemas": True, "totalJobs": 1}],
    CONNECTION_ID_POSTGRES: [{"dataSourceName": "employees_copy_db", "supportsSchemas": True, "totalJobs": 0}],
}

SCHEMAS_BY_CONNECTION = {
    CONNECTION_ID_SNOWFLAKE: [{"name": "PUBLIC"}],
    CONNECTION_ID_POSTGRES: [{"name": "public"}],
}

TABLES_BY_CONNECTION = {
    CONNECTION_ID_SNOWFLAKE: [{"name": "EMPLOYEES", "type": "TABLE"}],
    CONNECTION_ID_POSTGRES: [{"name": "employees", "type": "TABLE"}],
}

# Column type names treated as usable for a time-slice predicate (a real
# date/timestamp column). EMPLOYEES deliberately has none - its only date-like
# field, DATE_OF_HIRE, is stored as varchar, matching the skill's own worked
# example (references/examples.md, scenario 4).
DATE_LIKE_TYPES = {"date", "timestamp", "timestamptz"}

TABLE_COLUMNS = {
    "EMPLOYEES": [
        {"name": "EMPLOYEE_ID", "type": "int4", "disabled": False},
        {"name": "EMAIL", "type": "varchar", "disabled": False},
        {"name": "DEPARTMENT_ID", "type": "int4", "disabled": False},
        {"name": "DATE_OF_HIRE", "type": "varchar", "disabled": False},
    ],
    "employees": [
        {"name": "id", "type": "int4", "disabled": False},
        {"name": "email", "type": "varchar", "disabled": False},
        {"name": "department_id", "type": "int4", "disabled": False},
        {"name": "date_of_hire", "type": "varchar", "disabled": False},
    ],
}

# ---------------------------------------------------------------------------
# Existing DQ job on EMPLOYEES (Phase 2 reuse path)
# ---------------------------------------------------------------------------

EXISTING_JOB_NAME = "PUBLIC.EMPLOYEES_1"
EXISTING_JOB_CONNECTION_ID = CONNECTION_ID_SNOWFLAKE

EXISTING_JOBS = [
    {
        "jobName": EXISTING_JOB_NAME,
        "jobType": "PUSHDOWN",
        "connectionId": CONNECTION_ID_SNOWFLAKE,
        "tableName": "EMPLOYEES",
        "schemaName": "PUBLIC",
    }
]

# No contract rules exist on the job yet - find_data_quality_rules should come
# back empty so the duplicate check finds nothing already covering the
# contract's clauses (deliberately NOT the "already-enforced" fixture state;
# that flavor is example #3 in references/examples.md, not one of the five
# evals.json scenarios ported here).
EXISTING_RULES_ON_JOB: list[dict] = []

# ---------------------------------------------------------------------------
# Data contract manifest (pull_data_contract_manifest)
# ---------------------------------------------------------------------------

DATA_CONTRACT_ID = "d0000000-0000-0000-0000-000000000001"
EMPLOYEES_TABLE_ASSET_ID = "d0000000-0000-0000-0000-000000000002"

# A deliberately small ODCS-flavored manifest - just enough for the skill to
# extract: servers (-> connection matching), schema.properties (-> rule
# mapping, including the DEPARTMENT_ID referential-integrity clause the skill
# must decline), and slaProperties.processingFrequency (-> schedule).
MANIFEST_YAML = """\
apiVersion: v3.0.0
kind: DataContract
id: {data_contract_id}
name: Employees Data Contract
version: 1.0.0
status: active
servers:
  - server: snowflake-prod
    type: snowflake
    connectionName: {connection_name}
    database: OWLUSERDB
    schema: PUBLIC
schema:
  - name: EMPLOYEES
    physicalName: EMPLOYEES
    properties:
      - name: EMPLOYEE_ID
        physicalType: NUMBER
        required: true
        unique: true
        primaryKey: true
      - name: EMAIL
        physicalType: VARCHAR
        required: true
      - name: DEPARTMENT_ID
        physicalType: NUMBER
        required: false
        quality:
          - rule: referentialIntegrity
            description: "DEPARTMENT_ID must exist in DEPARTMENTS.DEPARTMENT_ID"
            mustExistIn: DEPARTMENTS.DEPARTMENT_ID
      - name: DATE_OF_HIRE
        physicalType: VARCHAR
        required: false
slaProperties:
  - property: processingFrequency
    value: daily
""".format(data_contract_id=DATA_CONTRACT_ID, connection_name=CONNECTION_NAME_SNOWFLAKE)

# ---------------------------------------------------------------------------
# get_asset_details (GraphQL knowledgeGraph) - only wired for the two assets
# this fixture cares about; bridging from a Data Product is out of scope for
# the five ported scenarios, which all name the table/contract directly.
# ---------------------------------------------------------------------------

ASSETS_BY_ID = {
    EMPLOYEES_TABLE_ASSET_ID: {
        "id": EMPLOYEES_TABLE_ASSET_ID,
        "displayName": "EMPLOYEES",
        "type": {"name": "Table"},
        "domain": {"name": "TEST - DQ From Contract"},
        "status": {"name": "Active"},
        "stringAttributes": [],
        "numericAttributes": [],
        "booleanAttributes": [],
        "dateAttributes": [],
        "outgoingRelations": [
            {
                "type": {"id": "rel-type-governed-by", "role": "is governed by"},
                "target": {
                    "id": DATA_CONTRACT_ID,
                    "displayName": "Employees Data Contract",
                    "type": {"name": "Data Contract"},
                },
            }
        ],
        "incomingRelations": [],
    },
    DATA_CONTRACT_ID: {
        "id": DATA_CONTRACT_ID,
        "displayName": "Employees Data Contract",
        "type": {"name": "Data Contract"},
        "domain": {"name": "TEST - DQ From Contract"},
        "status": {"name": "Active"},
        "stringAttributes": [],
        "numericAttributes": [],
        "booleanAttributes": [],
        "dateAttributes": [],
        "outgoingRelations": [],
        "incomingRelations": [
            {
                "type": {"id": "rel-type-governed-by", "role": "governs functioning of"},
                "source": {
                    "id": EMPLOYEES_TABLE_ASSET_ID,
                    "displayName": "EMPLOYEES",
                    "type": {"name": "Table"},
                },
            }
        ],
    },
}

# ---------------------------------------------------------------------------
# get_data_quality_rule_results - canned single-run history for any rule name
# under the existing job, so a scenario that asks "is it passing" gets a real
# (deterministic) answer to report on.
# ---------------------------------------------------------------------------

CANNED_RULE_RESULT = {
    "runDate": 1735689600000,  # 2025-01-01T00:00:00Z
    "ruleStatus": "PASSING",
    "passFail": True,
    "score": 100,
    "totalCount": 500.0,
    "breakingRecords": 0.0,
    "passingRecords": 500.0,
}

REQUEST_LOG: list[dict] = []
_LOG_LOCK = threading.Lock()


def _log(method: str, path: str, body: bytes | None) -> None:
    with _LOG_LOCK:
        REQUEST_LOG.append({"method": method, "path": path, "body": body.decode() if body else None})


class Handler(BaseHTTPRequestHandler):
    def log_message(self, fmt, *args):  # noqa: A002 - silence default stderr logging
        pass

    def _send_json(self, status: int, payload: dict) -> None:
        body = json.dumps(payload).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def _send_raw(self, status: int, body: bytes, content_type: str = "text/plain") -> None:
        self.send_response(status)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    # ------------------------------------------------------------------ GET
    def do_GET(self):  # noqa: N802
        parsed = urlparse(self.path)
        path = parsed.path
        _log("GET", path, None)

        # --- pull_data_contract_manifest -----------------------------------
        m = re.match(r"^/rest/dataProduct/v1/dataContracts/([^/]+)/activeVersion/manifest$", path)
        if m:
            if m.group(1) == DATA_CONTRACT_ID:
                self._send_raw(200, MANIFEST_YAML.encode(), content_type="application/yaml")
            else:
                self._send_raw(404, b'{"error":"data contract not found"}', content_type="application/json")
            return

        # --- create_data_quality_job discovery (reused shape) --------------
        if path == "/rest/dq/internal/v1/connections":
            self._send_json(200, {"results": CONNECTIONS})
            return
        m = re.match(r"^/rest/dq/internal/v1/monitoring/edge/connections/([^/]+)/dataSources$", path)
        if m:
            self._send_json(200, {"results": DATA_SOURCES_BY_CONNECTION.get(m.group(1), []), "total": len(DATA_SOURCES_BY_CONNECTION.get(m.group(1), []))})
            return
        m = re.match(r"^/rest/dq/internal/v1/monitoring/edge/[^/]+/connections/([^/]+)/schemas$", path)
        if m:
            self._send_json(200, {"results": SCHEMAS_BY_CONNECTION.get(m.group(1), []), "total": len(SCHEMAS_BY_CONNECTION.get(m.group(1), []))})
            return
        m = re.match(r"^/rest/dq/internal/v1/monitoring/edge/[^/]+/connections/([^/]+)/tables$", path)
        if m:
            self._send_json(200, {"results": TABLES_BY_CONNECTION.get(m.group(1), []), "total": len(TABLES_BY_CONNECTION.get(m.group(1), []))})
            return
        if re.match(r"^/rest/dq/internal/v1/monitoring/edge/[^/]+/connections/[^/]+/columns$", path):
            table_name = parse_qs(parsed.query).get("tableName", ["EMPLOYEES"])[0]
            self._send_json(200, {"results": TABLE_COLUMNS.get(table_name, TABLE_COLUMNS["EMPLOYEES"])})
            return
        if path == "/rest/dq/1.0/jobs":
            # Used both for auto-naming collision search and for "does a job
            # already exist on this table" discovery - report the one seeded
            # job so the reuse scenarios have something to find.
            self._send_json(200, {"results": EXISTING_JOBS})
            return
        if path == "/rest/2.0/users/current":
            self._send_json(200, {"id": "eval-user", "userName": "eval.user", "emailAddress": "eval.user@example.com"})
            return

        # --- get_data_quality_rule_results -----------------------------------
        m = re.match(r"^/rest/dq/internal/v1/monitoring/rules/([^/]+)/([^/]+)$", path)
        if m:
            job_name, rule_name = m.group(1), m.group(2)
            self._send_json(
                200,
                {
                    "dataset": job_name,
                    "ruleName": rule_name,
                    "ruleType": "FREEFORM_SQL",
                    "ruleValue": f"SELECT * FROM @{job_name} WHERE 1=0",
                    "tolerance": 0,
                    "isActive": 1,
                    "results": [CANNED_RULE_RESULT],
                    "total": 1,
                    "offset": 0,
                    "limit": 10,
                },
            )
            return

        self._send_json(404, {"error": f"mock has no handler for GET {path}"})

    # ----------------------------------------------------------------- POST
    def do_POST(self):  # noqa: N802
        length = int(self.headers.get("Content-Length", 0))
        body = self.rfile.read(length) if length else b""
        _log("POST", self.path, body)

        try:
            parsed_body = json.loads(body) if body else {}
        except json.JSONDecodeError:
            parsed_body = {}

        # --- permission preflight -------------------------------------------
        if self.path == "/graphql":
            self._send_json(
                200,
                {"data": {"api": {"currentUser": {"global": [], "resource": ["RESOURCE_MANAGE_ALL"]}}}},
            )
            return

        # --- get_asset_details ------------------------------------------------
        if self.path == "/graphql/knowledgeGraph/v1":
            variables = parsed_body.get("variables", {})
            asset_ids = variables.get("assetIds", [])
            assets = [ASSETS_BY_ID[a] for a in asset_ids if a in ASSETS_BY_ID]
            self._send_json(200, {"data": {"assets": assets}})
            return

        # --- create_data_quality_job (confirm=true path) ---------------------
        if self.path == "/rest/dq/1.0/jobs":
            job_name = parsed_body.get("jobName") or "PUBLIC.EMPLOYEES_2"
            self._send_json(
                201,
                {"jobName": job_name, "jobType": parsed_body.get("jobType", "PUSHDOWN"), "jobRunId": "eval-run-1"},
            )
            return

        # --- find_data_quality_rules ------------------------------------------
        if self.path == "/rest/dq/internal/v1/monitoring/monitors/dashboard":
            self._send_json(
                200,
                {
                    "results": EXISTING_RULES_ON_JOB,
                    "total": len(EXISTING_RULES_ON_JOB),
                    "offset": parsed_body.get("offset", 0),
                    "limit": parsed_body.get("limit", 25),
                },
            )
            return

        # --- create_data_quality_rule (confirm=true path) ---------------------
        if self.path == "/rest/dq/internal/v1/monitoring/monitor":
            self._send_json(
                200,
                {"jobName": parsed_body.get("jobName", EXISTING_JOB_NAME), "monitorName": parsed_body.get("monitorName", "")},
            )
            return

        self._send_json(404, {"error": f"mock has no handler for POST {self.path}"})


def serve(port: int) -> ThreadingHTTPServer:
    server = ThreadingHTTPServer(("127.0.0.1", port), Handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    return server


if __name__ == "__main__":
    chosen_port = int(sys.argv[1]) if len(sys.argv) > 1 else 8812
    srv = serve(chosen_port)
    print(f"Mock Collibra server listening on http://127.0.0.1:{chosen_port}")
    try:
        threading.Event().wait()
    except KeyboardInterrupt:
        srv.shutdown()
