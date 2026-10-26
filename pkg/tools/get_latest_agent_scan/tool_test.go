package get_latest_agent_scan_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/collibra/chip/pkg/tools/get_latest_agent_scan"
	"github.com/collibra/chip/pkg/tools/testutil"
)

const aiAgentID = "d654ca91-10d5-34bf-8819-78800b81fe23"

const latestPath = "/rest/guardianFingerprint/v1/fingerprints/by/aiAgent/" + aiAgentID

const fingerprint = `{"id":"f1","externalId":"databricks:1:cat.s.max","aiAgentId":"` + aiAgentID + `","contentHash":"abc","fingerprintVersion":"1.0","parserVersion":"skill-0.1","scanCount":3,"firstSeenOn":"2026-10-01T00:00:00Z","lastSeenOn":"2026-10-02T00:00:00Z",` +
	`"document":{"agents":[{"name":"example_agent","role":"ROOT","deployment":{"version":"9","endpoint":{"url":"https://h.example/serving-endpoints/ep/invocations","name":"ep"}}},{"name":"helper","role":"SUBAGENT"}],"tools":[{"name":"a"},{"name":"b"},{"name":"c"}],` +
	`"source":{"repoUrl":"git@example.com:acme/max.git","commitSha":"84792b0"},"platform":{"name":"DATABRICKS"}}}`

type fakeGuardian struct {
	client *http.Client
	calls  atomic.Int32
}

func newFakeGuardian(t *testing.T, status int, body string) *fakeGuardian {
	t.Helper()
	f := &fakeGuardian{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/guardianFingerprint/v1/fingerprints/by/aiAgent/{id}", func(w http.ResponseWriter, r *http.Request) {
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

func TestGetLatestAgentScan_SummaryByDefaultWithoutTheDocument(t *testing.T) {
	f := newFakeGuardian(t, http.StatusOK, fingerprint)

	out, err := get_latest_agent_scan.NewTool(f.client).Handler(t.Context(), get_latest_agent_scan.Input{AIAgentID: aiAgentID})
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != get_latest_agent_scan.StatusSuccess || out.ContentHash != "abc" || out.ScanCount != 3 || out.ParserVersion != "skill-0.1" {
		t.Fatalf("unexpected output: %+v", out)
	}
	if out.Source == nil || out.Source.CommitSha != "84792b0" || out.RootAgent != "example_agent" || out.DeploymentVersion != "9" || out.EndpointURL != "https://h.example/serving-endpoints/ep/invocations" || out.EndpointName != "ep" ||
		out.PlatformName != "DATABRICKS" || out.AgentCount != 2 || out.ToolCount != 3 {
		t.Errorf("summary wrong: %+v", out)
	}
	if len(out.Document) != 0 {
		t.Errorf("document returned although includeDocument was false (%d bytes)", len(out.Document))
	}
}

func TestGetLatestAgentScan_IncludeDocumentReturnsItWhole(t *testing.T) {
	f := newFakeGuardian(t, http.StatusOK, fingerprint)

	out, _ := get_latest_agent_scan.NewTool(f.client).Handler(t.Context(), get_latest_agent_scan.Input{AIAgentID: aiAgentID, IncludeDocument: true})
	var doc struct {
		Agents []struct{ Name string } `json:"agents"`
	}
	if err := json.Unmarshal(out.Document, &doc); err != nil || len(doc.Agents) != 2 {
		t.Fatalf("document not returned intact: %s (%v)", out.Document, err)
	}
}

func TestGetLatestAgentScan_NotFoundIsAnAnswer(t *testing.T) {
	f := newFakeGuardian(t, http.StatusNotFound, `{"statusCode":404,"errorCode":"aiAgentNotFound"}`)

	out, _ := get_latest_agent_scan.NewTool(f.client).Handler(t.Context(), get_latest_agent_scan.Input{AIAgentID: aiAgentID})
	if out.Status != get_latest_agent_scan.StatusNotFound || out.HTTPStatus != http.StatusNotFound || !strings.Contains(out.Guidance, "continue without") {
		t.Fatalf("unexpected output: %+v", out)
	}
}

func TestGetLatestAgentScan_RejectsANonUUIDWithoutCalling(t *testing.T) {
	f := newFakeGuardian(t, http.StatusOK, fingerprint)

	out, _ := get_latest_agent_scan.NewTool(f.client).Handler(t.Context(), get_latest_agent_scan.Input{AIAgentID: "databricks:1:cat.s.max"})
	if out.Status != get_latest_agent_scan.StatusValidationError || f.calls.Load() != 0 {
		t.Fatalf("status %q after %d call(s)", out.Status, f.calls.Load())
	}
}

func TestGetLatestAgentScan_AuthFailuresAreNotRetried(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		f := newFakeGuardian(t, status, `{"statusCode":0,"errorCode":"x"}`)
		out, _ := get_latest_agent_scan.NewTool(f.client).Handler(t.Context(), get_latest_agent_scan.Input{AIAgentID: aiAgentID})
		if out.Status != get_latest_agent_scan.StatusError || out.HTTPStatus != status || !strings.Contains(out.Guidance, "Do not retry") {
			t.Errorf("status %d: output %+v", status, out)
		}
		if f.calls.Load() != 1 {
			t.Errorf("status %d: %d call(s), want 1", status, f.calls.Load())
		}
	}
}

func TestGetLatestAgentScan_SignInPageIsASignInProblem(t *testing.T) {
	f := newFakeGuardian(t, http.StatusOK, `<html>sign in</html>`)
	out, _ := get_latest_agent_scan.NewTool(f.client).Handler(t.Context(), get_latest_agent_scan.Input{AIAgentID: aiAgentID})
	if out.Status != get_latest_agent_scan.StatusError || out.HTTPStatus != http.StatusUnauthorized {
		t.Fatalf("unexpected output: %+v", out)
	}
}
