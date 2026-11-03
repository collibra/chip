package get_agent_guardian_status_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/collibra/chip/pkg/tools/get_agent_guardian_status"
	"github.com/collibra/chip/pkg/tools/testutil"
)

const aiAgentID = "3f1c2b7a-8d4e-4a6b-9c1d-2e3f4a5b6c7d"

const jobID = "6a7b8c9d-0e1f-4a2b-8c3d-4e5f6a7b8c9d"

const overview = `{
  "aiAgent": {"id": "` + aiAgentID + `", "name": "support_triage_agent"},
  "fingerprint": {"status": "COMPLETED", "id": "0b1c2d3e-4f5a-4b6c-8d7e-9f0a1b2c3d4e", "createdOn": "2026-10-01T08:00:00Z", "lastReceivedOn": "2026-10-02T08:00:00Z",
    "aiAgentVersion": {"id": "5e6f7a8b-9c0d-4e1f-8a2b-3c4d5e6f7a8b", "name": "support_triage_agent_v1"}},
  "contracts": [
    {"id": "1a2b3c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d", "name": "Global contract", "layer": "GLOBAL", "activeVersionId": "2b3c4d5e-6f7a-4b8c-9d0e-1f2a3b4c5d6e"},
    {"id": "3c4d5e6f-7a8b-4c9d-8e0f-1a2b3c4d5e6f", "name": "Support contract", "layer": "SPECIALIZED", "activeVersionId": "4d5e6f7a-8b9c-4d0e-9f1a-2b3c4d5e6f7a"}
  ],
  "compilation": {"status": "STALE", "staleBecause": ["CONTRACT_VERSION_CHANGED"],
    "latestJob": {"id": "` + jobID + `", "state": "COMPLETED", "createdOn": "2026-10-02T09:00:00Z", "bundleId": "7f8a9b0c-1d2e-4f3a-8b4c-5d6e7f8a9b0c"},
    "latestBundle": {"id": "7f8a9b0c-1d2e-4f3a-8b4c-5d6e7f8a9b0c", "version": 2, "compiledAt": "2026-10-02T09:04:00Z", "agentVersionId": "5e6f7a8b-9c0d-4e1f-8a2b-3c4d5e6f7a8b",
      "agentFingerprintId": "0b1c2d3e-4f5a-4b6c-8d7e-9f0a1b2c3d4e", "contractVersionIds": ["2b3c4d5e-6f7a-4b8c-9d0e-1f2a3b4c5d6e"]}},
  "enforcement": {"status": "UP_TO_DATE", "observedBundle": {"id": "7f8a9b0c-1d2e-4f3a-8b4c-5d6e7f8a9b0c", "observedOn": "2026-10-02T10:00:00Z"}, "lastObservedOn": "2026-10-02T10:00:00Z"},
  "collection": {"status": "NOT_COLLECTED", "reason": "SOURCE_NOT_CONNECTED"}
}`

type fakeGuardian struct {
	client *http.Client
	calls  atomic.Int32
}

func newFakeGuardian(t *testing.T, status int, body string) *fakeGuardian {
	t.Helper()
	f := &fakeGuardian{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/guardianOverview/v1/aiAgents/{id}", func(w http.ResponseWriter, r *http.Request) {
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

func run(t *testing.T, f *fakeGuardian, id string) get_agent_guardian_status.Output {
	t.Helper()
	out, err := get_agent_guardian_status.NewTool(f.client).Handler(t.Context(), get_agent_guardian_status.Input{AIAgentID: id})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestGetAgentGuardianStatus_PassesTheBlocksThroughWithoutTheJobID(t *testing.T) {
	f := newFakeGuardian(t, http.StatusOK, overview)
	out := run(t, f, aiAgentID)

	if out.Status != get_agent_guardian_status.StatusSuccess || len(out.AbsentBlocks) != 0 {
		t.Fatalf("unexpected output: %+v", out)
	}
	if out.AIAgent.Name != "support_triage_agent" || out.Fingerprint.AIAgentVersion.Name != "support_triage_agent_v1" ||
		len(*out.Contracts) != 2 || (*out.Contracts)[0].Layer != "GLOBAL" ||
		out.Compilation.Status != "STALE" || out.Compilation.StaleBecause[0] != "CONTRACT_VERSION_CHANGED" ||
		out.Compilation.LatestJob.State != "COMPLETED" || out.Compilation.LatestBundle.Version != 2 ||
		out.Enforcement.ObservedBundle == nil || out.Collection.Reason != "SOURCE_NOT_CONNECTED" {
		t.Errorf("blocks not passed through: %+v", out)
	}

	encoded, _ := json.Marshal(out)
	if strings.Contains(string(encoded), jobID) {
		t.Errorf("the job id must not be returned: %s", encoded)
	}
	for _, field := range []string{`"staleBecause"`, `"latestJob"`, `"latestBundle"`, `"activeVersionId"`, `"lastReceivedOn"`, `"observedBundle"`, `"agentFingerprintId"`, `"contractVersionIds"`} {
		if !strings.Contains(string(encoded), field) {
			t.Errorf("field %s missing from the output: %s", field, encoded)
		}
	}
}

func TestGetAgentGuardianStatus_ListsAbsentBlocks(t *testing.T) {
	f := newFakeGuardian(t, http.StatusOK, `{"aiAgent":{"id":"`+aiAgentID+`"},"fingerprint":{"status":"IN_PROGRESS","id":"0b1c2d3e-4f5a-4b6c-8d7e-9f0a1b2c3d4e"},"contracts":[]}`)
	out := run(t, f, aiAgentID)

	if out.Status != get_agent_guardian_status.StatusSuccess {
		t.Fatalf("unexpected output: %+v", out)
	}
	if !slices.Equal(out.AbsentBlocks, []string{"compilation", "enforcement", "collection"}) {
		t.Errorf("absentBlocks = %v", out.AbsentBlocks)
	}
	if out.Contracts == nil || len(*out.Contracts) != 0 {
		t.Errorf("an empty contracts list must stay present: %+v", out.Contracts)
	}
	if !strings.Contains(out.Message, "compilation, enforcement, collection") {
		t.Errorf("message should name the absent blocks: %s", out.Message)
	}
}

func TestGetAgentGuardianStatus_DocumentedErrors(t *testing.T) {
	for _, tc := range []struct {
		status   int
		code     string
		want     get_agent_guardian_status.OutputStatus
		guidance string
	}{
		{http.StatusForbidden, "accessDenied", get_agent_guardian_status.StatusError, "Do not retry"},
		{http.StatusNotFound, "aiAgentNotFound", get_agent_guardian_status.StatusNotFound, "still be importing"},
		{http.StatusServiceUnavailable, "agentContractsUnavailable", get_agent_guardian_status.StatusError, "temporary"},
	} {
		f := newFakeGuardian(t, tc.status, `{"statusCode":0,"errorCode":"`+tc.code+`"}`)
		out := run(t, f, aiAgentID)
		if out.Status != tc.want || out.ErrorCode != tc.code || out.HTTPStatus != tc.status || !strings.Contains(out.Guidance, tc.guidance) {
			t.Errorf("%s: output %+v", tc.code, out)
		}
		if f.calls.Load() != 1 {
			t.Errorf("%s: %d call(s), want 1", tc.code, f.calls.Load())
		}
	}
}

func TestGetAgentGuardianStatus_RejectsANonUUIDWithoutCalling(t *testing.T) {
	f := newFakeGuardian(t, http.StatusOK, overview)
	out := run(t, f, "databricks:1:cat.s.agent")
	if out.Status != get_agent_guardian_status.StatusValidationError || f.calls.Load() != 0 {
		t.Fatalf("status %q after %d call(s)", out.Status, f.calls.Load())
	}
}
