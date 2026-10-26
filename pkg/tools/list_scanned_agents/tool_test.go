package list_scanned_agents_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/collibra/chip/pkg/tools/list_scanned_agents"
	"github.com/collibra/chip/pkg/tools/testutil"
)

const importedAgentsPath = "/rest/guardianFingerprint/v1/importedAgents"

type fakeGuardian struct {
	client *http.Client
	calls  atomic.Int32
	last   *http.Request
}

func newFakeGuardian(t *testing.T, handle func(w http.ResponseWriter, r *http.Request)) *fakeGuardian {
	t.Helper()
	f := &fakeGuardian{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+importedAgentsPath, func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		f.last = r
		w.Header().Set("Content-Type", "application/json")
		handle(w, r)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	f.client = testutil.NewClient(srv)
	return f
}

func answer(status int, body string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

const onePage = `{"results":[{"externalId":"databricks:1:cat.s.max","aiAgentId":"a1","aiAgentVersionId":"v1"},{"externalId":"databricks:1:cat.s.other","aiAgentId":"a2"}],"nextCursor":"next"}`

func TestListScannedAgents_ReturnsThePageAndSendsTheSourceSystem(t *testing.T) {
	f := newFakeGuardian(t, answer(http.StatusOK, onePage))

	out, err := list_scanned_agents.NewTool(f.client).Handler(t.Context(), list_scanned_agents.Input{SourceSystem: "databricks:1", Limit: 50, Cursor: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != list_scanned_agents.StatusSuccess || len(out.Agents) != 2 || out.NextCursor != "next" {
		t.Fatalf("unexpected output: %+v", out)
	}
	if out.Agents[0].AIAgentVersionID != "v1" || out.Agents[1].AIAgentVersionID != "" {
		t.Errorf("version ids not carried through: %+v", out.Agents)
	}
	q := f.last.URL.Query()
	if q.Get("sourceSystem") != "databricks:1" || q.Get("limit") != "50" || q.Get("cursor") != "c1" {
		t.Errorf("query = %v", q)
	}
}

func TestListScanned_EmptyPageIsAnAnswer(t *testing.T) {
	f := newFakeGuardian(t, answer(http.StatusOK, `{"results":[]}`))
	out, _ := list_scanned_agents.NewTool(f.client).Handler(t.Context(), list_scanned_agents.Input{SourceSystem: "databricks:9"})
	if out.Status != list_scanned_agents.StatusSuccess || len(out.Agents) != 0 {
		t.Fatalf("unexpected output: %+v", out)
	}
}

func TestListScannedAgents_ExternalIDFindsTheAgentOnALaterPage(t *testing.T) {
	f := newFakeGuardian(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("cursor") == "" {
			_, _ = w.Write([]byte(`{"results":[{"externalId":"databricks:1:x","aiAgentId":"a0"}],"nextCursor":"p2"}`))
			return
		}
		_, _ = w.Write([]byte(`{"results":[{"externalId":"databricks:1:cat.s.max","aiAgentId":"a1","aiAgentVersionId":"v1"}]}`))
	})
	out, _ := list_scanned_agents.NewTool(f.client).Handler(t.Context(), list_scanned_agents.Input{SourceSystem: "databricks:1", ExternalID: "databricks:1:cat.s.max"})
	if len(out.Agents) != 1 || out.Agents[0].AIAgentID != "a1" || f.calls.Load() != 2 {
		t.Fatalf("output %+v after %d call(s)", out, f.calls.Load())
	}
}

func TestListScannedAgents_ExternalIDNotImportedIsAnAnswer(t *testing.T) {
	f := newFakeGuardian(t, answer(http.StatusOK, `{"results":[{"externalId":"databricks:1:other","aiAgentId":"a0"}]}`))
	out, _ := list_scanned_agents.NewTool(f.client).Handler(t.Context(), list_scanned_agents.Input{SourceSystem: "databricks:1", ExternalID: "databricks:1:missing"})
	if out.Status != list_scanned_agents.StatusSuccess || len(out.Agents) != 0 || out.Truncated || !strings.Contains(out.Message, "not imported") {
		t.Fatalf("unexpected output: %+v", out)
	}
}

func TestListScannedAgents_ExternalIDSearchStopsAtThePageLimit(t *testing.T) {
	f := newFakeGuardian(t, answer(http.StatusOK, `{"results":[],"nextCursor":"more"}`))
	out, _ := list_scanned_agents.NewTool(f.client).Handler(t.Context(), list_scanned_agents.Input{SourceSystem: "databricks:1", ExternalID: "databricks:1:missing"})
	if !out.Truncated || f.calls.Load() != 10 {
		t.Fatalf("truncated=%v after %d call(s), want true after 10", out.Truncated, f.calls.Load())
	}
}

// An id without a route (a repository address plus a name, or a bare name) is
// looked up by a prefix of itself: nothing requires a colon.
func TestListScannedAgents_AnIDWithoutARouteIsLookedUpByItsOwnPrefix(t *testing.T) {
	const id = "git@github.com:acme/patient-calls.git#call_analysis"
	for _, system := range []string{"git@github.com:acme/patient-calls.git", id, "call_analysis"} {
		f := newFakeGuardian(t, answer(http.StatusOK, `{"results":[{"externalId":"`+id+`","aiAgentId":"a9"}]}`))
		externalID := id
		if system == "call_analysis" {
			externalID = ""
		}
		out, _ := list_scanned_agents.NewTool(f.client).Handler(t.Context(), list_scanned_agents.Input{SourceSystem: system, ExternalID: externalID})
		if out.Status != list_scanned_agents.StatusSuccess || len(out.Agents) != 1 || f.last.URL.Query().Get("sourceSystem") != system {
			t.Errorf("sourceSystem %q: output %+v", system, out)
		}
	}
}

func TestListScannedAgents_ContainsFindsAModelAcrossInstances(t *testing.T) {
	f := newFakeGuardian(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("cursor") == "" {
			_, _ = w.Write([]byte(`{"results":[{"externalId":"databricks:1:cat.s.other","aiAgentId":"a0"},{"externalId":"databricks:2:example_catalog.example_schema.example_agent","aiAgentId":"a1"}],"nextCursor":"p2"}`))
			return
		}
		_, _ = w.Write([]byte(`{"results":[{"externalId":"databricks:3:example_catalog.example_schema.example_agent","aiAgentId":"a2"}]}`))
	})
	out, _ := list_scanned_agents.NewTool(f.client).Handler(t.Context(), list_scanned_agents.Input{SourceSystem: "databricks:", Contains: "example_schema.example_agent"})
	if out.Status != list_scanned_agents.StatusSuccess || len(out.Agents) != 2 || out.Agents[0].AIAgentID != "a1" || out.Agents[1].AIAgentID != "a2" || out.Truncated {
		t.Fatalf("unexpected output: %+v", out)
	}
	if f.last.URL.Query().Get("sourceSystem") != "databricks" {
		t.Errorf("sourceSystem = %q, want the trailing colon dropped", f.last.URL.Query().Get("sourceSystem"))
	}
}

func TestListScannedAgents_ContainsStopsAtTheLimit(t *testing.T) {
	f := newFakeGuardian(t, answer(http.StatusOK, `{"results":[{"externalId":"databricks:1:m","aiAgentId":"a1"},{"externalId":"databricks:2:m","aiAgentId":"a2"},{"externalId":"databricks:3:m","aiAgentId":"a3"}]}`))
	out, _ := list_scanned_agents.NewTool(f.client).Handler(t.Context(), list_scanned_agents.Input{SourceSystem: "databricks:", Contains: ":m", Limit: 2})
	if len(out.Agents) != 2 || !out.Truncated {
		t.Fatalf("unexpected output: %+v", out)
	}
}

func TestListScannedAgents_RejectsBadInputWithoutCalling(t *testing.T) {
	f := newFakeGuardian(t, answer(http.StatusOK, onePage))
	for name, in := range map[string]list_scanned_agents.Input{
		"blank":            {SourceSystem: " "},
		"padded":           {SourceSystem: " databricks:1"},
		"separator":        {SourceSystem: "databricks:1>x"},
		"mismatched exact": {SourceSystem: "databricks:1", ExternalID: "databricks:2:x"},
		"limit too big":    {SourceSystem: "databricks:1", Limit: 501},
		"padded contains":  {SourceSystem: "databricks:1", Contains: " max"},
	} {
		out, _ := list_scanned_agents.NewTool(f.client).Handler(t.Context(), in)
		if out.Status != list_scanned_agents.StatusValidationError {
			t.Errorf("%s: status = %q, want validation_error", name, out.Status)
		}
	}
	if f.calls.Load() != 0 {
		t.Errorf("bad input reached the API %d time(s)", f.calls.Load())
	}
}

func TestListScannedAgents_ReadsFailuresPlainly(t *testing.T) {
	for _, tc := range []struct {
		status   int
		contains string
	}{
		{http.StatusUnauthorized, "401"},
		{http.StatusForbidden, "403"},
		{http.StatusInternalServerError, "HTTP 500"},
	} {
		f := newFakeGuardian(t, answer(tc.status, fmt.Sprintf(`{"statusCode":%d,"errorCode":"x"}`, tc.status)))
		out, _ := list_scanned_agents.NewTool(f.client).Handler(t.Context(), list_scanned_agents.Input{SourceSystem: "databricks:1"})
		if out.Status != list_scanned_agents.StatusError || !strings.Contains(out.Message, tc.contains) || out.Guidance == "" || out.HTTPStatus != tc.status {
			t.Errorf("status %d: output %+v", tc.status, out)
		}
		if f.calls.Load() != 1 {
			t.Errorf("status %d: %d call(s), want 1 (no retry)", tc.status, f.calls.Load())
		}
	}
}

func TestListScannedAgents_SignInPageIsASignInProblem(t *testing.T) {
	f := newFakeGuardian(t, answer(http.StatusOK, `<html>please sign in</html>`))
	out, _ := list_scanned_agents.NewTool(f.client).Handler(t.Context(), list_scanned_agents.Input{SourceSystem: "databricks:1"})
	if out.Status != list_scanned_agents.StatusError || out.HTTPStatus != http.StatusUnauthorized {
		t.Fatalf("unexpected output: %+v", out)
	}
}

// realService answers like the Fingerprint API does: it selects the agents
// whose external id starts with "<sourceSystem>:", on whole leading segments,
// so a prefix that ends inside a segment selects nothing. Recorded from the
// real service (see the id below, which exists in Collibra).
func newRealService(t *testing.T, externalIDs ...string) *fakeGuardian {
	t.Helper()
	return newFakeGuardian(t, func(w http.ResponseWriter, r *http.Request) {
		system := r.URL.Query().Get("sourceSystem")
		var rows []string
		for i, id := range externalIDs {
			if strings.HasPrefix(id, system+":") {
				rows = append(rows, fmt.Sprintf(`{"externalId":%q,"aiAgentId":"agent-%d"}`, id, i))
			}
		}
		_, _ = fmt.Fprintf(w, `{"results":[%s]}`, strings.Join(rows, ","))
	})
}

const (
	repoID       = "git@github.com:collibra/ai-governance-shape-up.git#call-analysis-orchestrator"
	repoAddress  = "git@github.com:collibra/ai-governance-shape-up.git"
	databricksID = "databricks:1234567890123456:example_catalog.example_schema.example_agent"
)

func TestListScannedAgents_FindsARepositoryShapedIDFromItsRepositoryAddress(t *testing.T) {
	f := newRealService(t, databricksID, repoID)
	tool := list_scanned_agents.NewTool(f.client)

	for name, in := range map[string]list_scanned_agents.Input{
		"exact id, repository address as source": {SourceSystem: repoAddress, ExternalID: repoID},
		"exact id, whole id as source":           {SourceSystem: repoID, ExternalID: repoID},
		"list by repository address":             {SourceSystem: repoAddress},
		"contains, repository address":           {SourceSystem: repoAddress, Contains: "call-analysis"},
		"provider alone":                         {SourceSystem: "git@github.com"},
		"provider with a trailing colon":         {SourceSystem: "git@github.com:", ExternalID: repoID},
	} {
		out, _ := tool.Handler(t.Context(), in)
		if out.Status != list_scanned_agents.StatusSuccess || len(out.Agents) != 1 || out.Agents[0].ExternalID != repoID || out.Agents[0].AIAgentID != "agent-1" {
			t.Errorf("%s: output %+v", name, out)
		}
	}
}

func TestListScannedAgents_RouteAloneFindsAModelInAnyInstance(t *testing.T) {
	f := newRealService(t, databricksID, "databricks:111:other.model", repoID)

	out, _ := list_scanned_agents.NewTool(f.client).Handler(t.Context(), list_scanned_agents.Input{SourceSystem: "databricks:", Contains: ":example_catalog.example_schema.example_agent"})
	if len(out.Agents) != 1 || out.Agents[0].ExternalID != databricksID {
		t.Fatalf("unexpected output: %+v", out)
	}
}

func TestListScannedAgents_AnUnknownRepositoryIsAnEmptyCompleteAnswer(t *testing.T) {
	f := newRealService(t, databricksID, repoID)

	out, _ := list_scanned_agents.NewTool(f.client).Handler(t.Context(), list_scanned_agents.Input{SourceSystem: "git@github.com:acme/other.git", ExternalID: "git@github.com:acme/other.git#x"})
	if out.Status != list_scanned_agents.StatusSuccess || len(out.Agents) != 0 || out.Truncated || !strings.Contains(out.Message, "not imported") {
		t.Fatalf("unexpected output: %+v", out)
	}
}

func TestListScannedAgents_AWholeLeadingSegmentPrefixIsAnsweredByTheServiceDirectly(t *testing.T) {
	f := newRealService(t, databricksID)

	out, _ := list_scanned_agents.NewTool(f.client).Handler(t.Context(), list_scanned_agents.Input{SourceSystem: "databricks:1234567890123456", ExternalID: databricksID})
	if len(out.Agents) != 1 || f.calls.Load() != 1 {
		t.Fatalf("output %+v after %d call(s), want the agent after 1", out, f.calls.Load())
	}
}

func newCatalog(t *testing.T, names ...string) *fakeGuardian {
	t.Helper()
	f := &fakeGuardian{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/2.0/assets", func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		f.last = r
		q := strings.ToLower(r.URL.Query().Get("name"))
		var rows []string
		for i, name := range names {
			if strings.Contains(strings.ToLower(name), q) {
				rows = append(rows, fmt.Sprintf(`{"id":"asset-%d","name":%q,"domain":{"name":"AI Models and Agents"}}`, i, name))
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"results":[%s]}`, strings.Join(rows, ","))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	f.client = testutil.NewClient(srv)
	return f
}

func TestListScannedAgents_FindsAnAgentByTheNameOfItsAsset(t *testing.T) {
	f := newCatalog(t,
		repoID+">agent>call-analysis-orchestrator",
		repoID+">subAgent>sentiment_analysis",
		repoID+">agentVersion>call-analysis-orchestrator_v1",
		repoID+">agentTool>web_search",
		databricksID+">agent>example_agent",
	)

	out, _ := list_scanned_agents.NewTool(f.client).Handler(t.Context(), list_scanned_agents.Input{NameContains: "call-analysis"})
	if out.Status != list_scanned_agents.StatusSuccess || len(out.Agents) != 1 {
		t.Fatalf("unexpected output: %+v", out)
	}
	got := out.Agents[0]
	if got.AIAgentID != "asset-0" || got.ExternalID != repoID || got.AgentName != "call-analysis-orchestrator" || got.Domain != "AI Models and Agents" {
		t.Errorf("agent = %+v", got)
	}
	if q := f.last.URL.Query(); q.Get("name") != "call-analysis" || q.Get("nameMatchMode") != "ANYWHERE" {
		t.Errorf("query = %v", q)
	}
}

func TestListScannedAgents_NameSearchCanBeNarrowedByASourceSystem(t *testing.T) {
	f := newCatalog(t, repoID+">agent>example_agent", databricksID+">agent>example_agent")

	out, _ := list_scanned_agents.NewTool(f.client).Handler(t.Context(), list_scanned_agents.Input{NameContains: "example_agent", SourceSystem: "databricks:"})
	if len(out.Agents) != 1 || out.Agents[0].ExternalID != databricksID {
		t.Fatalf("unexpected output: %+v", out)
	}
}

func TestListScannedAgents_NameSearchWithNoMatchIsAnEmptyAnswer(t *testing.T) {
	f := newCatalog(t, repoID+">agent>call-analysis-orchestrator")

	out, _ := list_scanned_agents.NewTool(f.client).Handler(t.Context(), list_scanned_agents.Input{NameContains: "nothing-like-this"})
	if out.Status != list_scanned_agents.StatusSuccess || len(out.Agents) != 0 {
		t.Fatalf("unexpected output: %+v", out)
	}
}

func TestListScannedAgents_NameSearchRejectsAPaddedNameWithoutCalling(t *testing.T) {
	f := newCatalog(t)

	out, _ := list_scanned_agents.NewTool(f.client).Handler(t.Context(), list_scanned_agents.Input{NameContains: " call"})
	if out.Status != list_scanned_agents.StatusValidationError || f.calls.Load() != 0 {
		t.Fatalf("status %q after %d call(s)", out.Status, f.calls.Load())
	}
}
