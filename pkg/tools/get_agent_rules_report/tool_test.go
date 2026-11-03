package get_agent_rules_report_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/collibra/chip/pkg/tools/get_agent_rules_report"
	"github.com/collibra/chip/pkg/tools/testutil"
)

const aiAgentID = "3f1c2b7a-8d4e-4a6b-9c1d-2e3f4a5b6c7d"

const jobID = "6a7b8c9d-0e1f-4a2b-8c3d-4e5f6a7b8c9d"

func rule(id, outcome, reason string) string {
	return `{"ruleId":"` + id + `","agentContractId":"1a2b3c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d","agentContractVersionId":"2b3c4d5e-6f7a-4b8c-9d0e-1f2a3b4c5d6e",` +
		`"agentContractLayer":"GLOBAL","statement":"Never share customer emails.","outcome":"` + outcome + `","reason":"` + reason + `",` +
		`"guards":[{"id":"9a8b7c6d-5e4f-4a3b-8c2d-1e0f9a8b7c6d","owner":"support_triage_agent","guard":{"action":"deny","secretGuardDetail":"x"}}]}`
}

func report(bundleID string) string {
	bundle := ""
	if bundleID != "" {
		bundle = `"bundleId":"` + bundleID + `",`
	}
	return `{"jobId":"` + jobID + `",` + bundle + `"agentVersionId":"5e6f7a8b-9c0d-4e1f-8a2b-3c4d5e6f7a8b","startedAt":"2026-10-02T09:00:00Z","finishedAt":"2026-10-02T09:04:00Z","rules":[` +
		rule("a1000000-0000-4000-8000-000000000001", "bound", "") + "," +
		rule("a1000000-0000-4000-8000-000000000002", "bound", "") + "," +
		rule("a1000000-0000-4000-8000-000000000003", "not_applicable", "The agent has no email tool.") + `]}`
}

type fakeGuardian struct {
	client *http.Client
	calls  atomic.Int32
}

func newFakeGuardian(t *testing.T, status int, body string) *fakeGuardian {
	t.Helper()
	f := &fakeGuardian{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/guardianEnforcement/v1/compilationReports/by/aiAgent/{id}", func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	f.client = testutil.NewClient(srv)
	return f
}

func run(t *testing.T, f *fakeGuardian, id string) get_agent_rules_report.Output {
	t.Helper()
	out, err := get_agent_rules_report.NewTool(f.client).Handler(t.Context(), get_agent_rules_report.Input{AIAgentID: id})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestGetAgentRulesReport_ReturnsRulesAndCountsWithoutJobOrGuards(t *testing.T) {
	f := newFakeGuardian(t, http.StatusOK, report("7f8a9b0c-1d2e-4f3a-8b4c-5d6e7f8a9b0c"))
	out := run(t, f, aiAgentID)

	if out.Status != get_agent_rules_report.StatusSuccess || out.BundleID != "7f8a9b0c-1d2e-4f3a-8b4c-5d6e7f8a9b0c" ||
		out.AgentVersionID != "5e6f7a8b-9c0d-4e1f-8a2b-3c4d5e6f7a8b" || out.StartedAt == "" || out.FinishedAt == "" || len(out.Rules) != 3 {
		t.Fatalf("unexpected output: %+v", out)
	}
	r := out.Rules[2]
	if r.Outcome != "not_applicable" || r.Reason != "The agent has no email tool." || r.AgentContractLayer != "GLOBAL" || r.Statement == "" || r.AgentContractVersionID == "" {
		t.Errorf("rule not passed through: %+v", r)
	}
	if len(out.OutcomeCounts) != 2 || out.OutcomeCounts[0].Outcome != "bound" || out.OutcomeCounts[0].Count != 2 || out.OutcomeCounts[1].Count != 1 {
		t.Errorf("outcome counts wrong: %+v", out.OutcomeCounts)
	}
	encoded, _ := json.Marshal(out)
	for _, leaked := range []string{jobID, "guards", "secretGuardDetail", "9a8b7c6d-5e4f-4a3b-8c2d-1e0f9a8b7c6d"} {
		if strings.Contains(string(encoded), leaked) {
			t.Errorf("output must not contain %q: %s", leaked, encoded)
		}
	}
}

func TestGetAgentRulesReport_BundleIDAbsentWhenNoRulesFileWasMade(t *testing.T) {
	f := newFakeGuardian(t, http.StatusOK, report(""))
	out := run(t, f, aiAgentID)
	if out.Status != get_agent_rules_report.StatusSuccess || out.BundleID != "" || !strings.Contains(out.Message, "no rules file") {
		t.Fatalf("unexpected output: %+v", out)
	}
	encoded, _ := json.Marshal(out)
	if strings.Contains(string(encoded), `"bundleId"`) {
		t.Errorf("bundleId should be absent: %s", encoded)
	}
}

func TestGetAgentRulesReport_DocumentedErrors(t *testing.T) {
	for _, tc := range []struct {
		status int
		code   string
		want   get_agent_rules_report.OutputStatus
		hint   string
	}{
		{http.StatusNotFound, "compilationReportNotFound", get_agent_rules_report.StatusNotFound, "get_agent_guardian_status"},
		{http.StatusNotFound, "aiAgentNotFound", get_agent_rules_report.StatusNotFound, "list_scanned_agents"},
		{http.StatusForbidden, "tenantEnvironmentForbidden", get_agent_rules_report.StatusError, "Do not retry"},
		{http.StatusUnauthorized, "bearerTokenRequired", get_agent_rules_report.StatusError, "Do not retry"},
	} {
		f := newFakeGuardian(t, tc.status, `{"statusCode":0,"errorCode":"`+tc.code+`"}`)
		out := run(t, f, aiAgentID)
		if out.Status != tc.want || out.ErrorCode != tc.code || out.HTTPStatus != tc.status || !strings.Contains(out.Guidance, tc.hint) {
			t.Errorf("%s: output %+v", tc.code, out)
		}
	}
}

func TestGetAgentRulesReport_RejectsANonUUIDWithoutCalling(t *testing.T) {
	f := newFakeGuardian(t, http.StatusOK, report(""))
	out := run(t, f, " ")
	if out.Status != get_agent_rules_report.StatusValidationError || f.calls.Load() != 0 {
		t.Fatalf("status %q after %d call(s)", out.Status, f.calls.Load())
	}
}
