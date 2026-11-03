package apply_latest_rules_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/collibra/chip/pkg/tools/apply_latest_rules"
	"github.com/collibra/chip/pkg/tools/testutil"
)

const aiAgentID = "3f1c2b7a-8d4e-4a6b-9c1d-2e3f4a5b6c7d"

const jobID = "6a7b8c9d-0e1f-4a2b-8c3d-4e5f6a7b8c9d"

const overview = `{
  "aiAgent": {"id": "` + aiAgentID + `", "name": "support_triage_agent"},
  "fingerprint": {"status": "COMPLETED", "id": "0b1c2d3e-4f5a-4b6c-8d7e-9f0a1b2c3d4e", "createdOn": "2026-10-01T08:00:00Z", "lastReceivedOn": "2026-10-02T08:00:00Z",
    "aiAgentVersion": {"id": "5e6f7a8b-9c0d-4e1f-8a2b-3c4d5e6f7a8b", "name": "support_triage_agent_v1"}},
  "contracts": [
    {"id": "1a2b3c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d", "name": "Global contract", "layer": "GLOBAL", "activeVersionId": "2b3c4d5e-6f7a-4b8c-9d0e-1f2a3b4c5d6e"}
  ],
  "compilation": {"status": "NOT_COMPILED"},
  "enforcement": {"status": "NOT_OBSERVED"}
}`

type fakeGuardian struct {
	client       *http.Client
	overviewGets atomic.Int32
	compilePosts atomic.Int32
	lastBody     []byte
}

func newFakeGuardian(t *testing.T, compileStatus int, compileBody string) *fakeGuardian {
	t.Helper()
	f := &fakeGuardian{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/guardianOverview/v1/aiAgents/{id}", func(w http.ResponseWriter, r *http.Request) {
		f.overviewGets.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(overview))
	})
	mux.HandleFunc("POST /rest/guardianEnforcement/v1/bundles/by/aiAgent/{id}/compile", func(w http.ResponseWriter, r *http.Request) {
		f.compilePosts.Add(1)
		f.lastBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Location", "/rest/guardianEnforcement/v1/jobs/"+jobID)
		w.WriteHeader(compileStatus)
		_, _ = w.Write([]byte(compileBody))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	f.client = testutil.NewClient(srv)
	return f
}

func (f *fakeGuardian) requests() int32 { return f.overviewGets.Load() + f.compilePosts.Load() }

func run(t *testing.T, f *fakeGuardian, in apply_latest_rules.Input) apply_latest_rules.Output {
	t.Helper()
	out, err := apply_latest_rules.NewTool(f.client).Handler(t.Context(), in)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

const accepted = `{"id":"` + jobID + `","state":"PENDING","createdOn":"2026-10-02T09:00:00Z"}`

func TestApplyLatestRules_PreviewReadsTheStatusAndSendsNoPost(t *testing.T) {
	f := newFakeGuardian(t, http.StatusAccepted, accepted)
	out := run(t, f, apply_latest_rules.Input{AIAgentID: aiAgentID})

	if out.Status != apply_latest_rules.StatusPreview || out.Preview == nil {
		t.Fatalf("unexpected output: %+v", out)
	}
	if f.compilePosts.Load() != 0 {
		t.Fatalf("preview sent %d compile request(s), want 0", f.compilePosts.Load())
	}
	p := out.Preview
	if p.AIAgentID != aiAgentID || p.AIAgent.Name != "support_triage_agent" || p.NewestVersion.Name != "support_triage_agent_v1" ||
		len(p.Contracts) != 1 || p.Contracts[0].ActiveVersionID != "2b3c4d5e-6f7a-4b8c-9d0e-1f2a3b4c5d6e" || p.CompilationStatus != "NOT_COMPILED" {
		t.Errorf("preview wrong: %+v", p)
	}
}

func TestApplyLatestRules_ConfirmSendsExactlyOnePostWithNoBodyAndNoJobID(t *testing.T) {
	f := newFakeGuardian(t, http.StatusAccepted, accepted)
	out := run(t, f, apply_latest_rules.Input{AIAgentID: aiAgentID, Confirm: true})

	if out.Status != apply_latest_rules.StatusSuccess || out.State != "PENDING" || out.CreatedOn != "2026-10-02T09:00:00Z" {
		t.Fatalf("unexpected output: %+v", out)
	}
	if f.compilePosts.Load() != 1 || f.requests() != 1 {
		t.Fatalf("sent %d compile request(s) and %d request(s) in all, want exactly 1", f.compilePosts.Load(), f.requests())
	}
	if len(f.lastBody) != 0 {
		t.Errorf("the compile request must have no body, got %q", f.lastBody)
	}
	encoded, _ := json.Marshal(out)
	if strings.Contains(string(encoded), jobID) {
		t.Errorf("the job id must not be returned: %s", encoded)
	}
}

func TestApplyLatestRules_DocumentedErrors(t *testing.T) {
	for _, tc := range []struct {
		status int
		code   string
		want   apply_latest_rules.OutputStatus
		hint   string
	}{
		{http.StatusNotFound, "aiAgentNotFound", apply_latest_rules.StatusNotFound, "importing"},
		{http.StatusForbidden, "accessDenied", apply_latest_rules.StatusError, "Do not retry"},
		{http.StatusForbidden, "tenantEnvironmentForbidden", apply_latest_rules.StatusError, "Do not retry"},
	} {
		f := newFakeGuardian(t, tc.status, `{"statusCode":0,"errorCode":"`+tc.code+`"}`)
		out := run(t, f, apply_latest_rules.Input{AIAgentID: aiAgentID, Confirm: true})
		if out.Status != tc.want || out.ErrorCode != tc.code || out.HTTPStatus != tc.status || !strings.Contains(out.Guidance, tc.hint) {
			t.Errorf("%s: output %+v", tc.code, out)
		}
		if !strings.Contains(out.Message, "Nothing was started") {
			t.Errorf("%s: message should say nothing was started: %s", tc.code, out.Message)
		}
		if f.compilePosts.Load() != 1 {
			t.Errorf("%s: %d compile request(s), want 1 (never retried)", tc.code, f.compilePosts.Load())
		}
	}
}

func TestApplyLatestRules_RejectsANonUUIDWithoutCalling(t *testing.T) {
	for _, confirm := range []bool{false, true} {
		f := newFakeGuardian(t, http.StatusAccepted, accepted)
		out := run(t, f, apply_latest_rules.Input{AIAgentID: "not-a-uuid", Confirm: confirm})
		if out.Status != apply_latest_rules.StatusValidationError || f.requests() != 0 {
			t.Fatalf("confirm=%v: status %q after %d request(s)", confirm, out.Status, f.requests())
		}
	}
}
