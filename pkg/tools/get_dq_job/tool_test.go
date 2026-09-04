package get_dq_job_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	tools "github.com/collibra/chip/pkg/tools/get_dq_job"
	"github.com/collibra/chip/pkg/tools/testutil"
)

// handlers configures the mocked job endpoints. A nil handler means "must not be called" — the mux
// fails the test if that endpoint is hit.
type handlers struct {
	get    func(w http.ResponseWriter, r *http.Request) // GET /rest/dq/1.0/jobs/{name}
	search func(w http.ResponseWriter, r *http.Request) // GET /rest/dq/1.0/jobs
}

func newServer(t *testing.T, h handlers) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/dq/1.0/jobs", func(w http.ResponseWriter, r *http.Request) {
		if h.search == nil {
			t.Errorf("unexpected search call: %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		h.search(w, r)
	})
	mux.HandleFunc("/rest/dq/1.0/jobs/", func(w http.ResponseWriter, r *http.Request) {
		if h.get == nil {
			t.Errorf("unexpected get call: %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		h.get(w, r)
	})
	return httptest.NewServer(mux)
}

func jsonHandler(code int, body any) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		if body != nil {
			_ = json.NewEncoder(w).Encode(body)
		}
	}
}

// sampleJob is a job definition shaped like the public API's JobDefinition.
func sampleJob() map[string]any {
	return map[string]any{
		"jobName":     "public.nyse",
		"jobType":     "PUSHDOWN",
		"sourceQuery": "SELECT * FROM PUBLIC.NYSE",
		"dataLocation": map[string]any{
			"edgeSiteName":       "US-East-1-Prod-Edge",
			"edgeConnectionName": "Finance-Snowflake-US",
			"dataSourceName":     "PRODSALESDB",
			"schemaName":         "PUBLIC",
			"tableName":          "NYSE",
		},
		"monitoringSettings": map[string]any{
			"adaptiveMonitors": map[string]any{"rowCount": true, "nullValues": true},
			"customMonitors":   []map[string]any{{"monitorName": "NAME_NOT_NULL", "customQuery": "SELECT 1"}},
		},
		"schedulingSettings": map[string]any{"isActive": true, "schedulerMode": "DAILY"},
	}
}

func run(t *testing.T, server *httptest.Server, in tools.Input) tools.Output {
	t.Helper()
	out, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), in)
	if err != nil {
		t.Fatalf("handler returned a Go error (should surface via Output): %v", err)
	}
	return out
}

// ---- input validation ----

func TestMissingNameNeedsInput(t *testing.T) {
	server := newServer(t, handlers{}) // no endpoint should be hit
	defer server.Close()

	out := run(t, server, tools.Input{})
	if out.Status != tools.StatusNeedsInput {
		t.Fatalf("expected needs_input when name is empty, got %q (%s)", out.Status, out.Message)
	}
}

// ---- exact match ----

func TestExactMatchSuccess(t *testing.T) {
	server := newServer(t, handlers{get: jsonHandler(http.StatusOK, sampleJob())})
	defer server.Close()

	out := run(t, server, tools.Input{Name: "public.nyse"})
	if out.Status != tools.StatusSuccess {
		t.Fatalf("expected success, got %q (%s)", out.Status, out.Message)
	}
	if out.Job == nil {
		t.Fatal("expected job details")
	}
	if out.Job.JobName != "public.nyse" || out.Job.JobType != "PUSHDOWN" {
		t.Errorf("unexpected job identity: %+v", out.Job)
	}
	if out.Job.SchemaName != "PUBLIC" || out.Job.TableName != "NYSE" || out.Job.EdgeSiteName != "US-East-1-Prod-Edge" {
		t.Errorf("unexpected data location: %+v", out.Job)
	}
	if len(out.Job.AdaptiveMonitors) != 2 {
		t.Errorf("expected 2 enabled adaptive monitors, got %v", out.Job.AdaptiveMonitors)
	}
	if len(out.Job.CustomMonitors) != 1 || out.Job.CustomMonitors[0] != "NAME_NOT_NULL" {
		t.Errorf("expected custom monitor name surfaced, got %v", out.Job.CustomMonitors)
	}
	if !out.Job.ScheduleEnabled || out.Job.ScheduleMode != "DAILY" {
		t.Errorf("expected schedule surfaced, got %+v", out.Job)
	}
	if out.Job.JobDetailsLink == "" {
		t.Error("expected a job details link")
	}
}

func TestExactMatchLookupError(t *testing.T) {
	cases := []struct {
		code      int
		wantInMsg string
	}{
		{http.StatusUnauthorized, "401"},
		{http.StatusForbidden, "403"},
		{http.StatusBadRequest, "400"},
		{http.StatusInternalServerError, "500"},
	}
	for _, tc := range cases {
		t.Run(tc.wantInMsg, func(t *testing.T) {
			server := newServer(t, handlers{get: jsonHandler(tc.code, map[string]any{"message": "boom"})})
			defer server.Close()

			out := run(t, server, tools.Input{Name: "public.nyse"})
			if out.Status != tools.StatusError {
				t.Fatalf("expected error for HTTP %d, got %q (%s)", tc.code, out.Status, out.Message)
			}
			if !strings.Contains(out.Message, tc.wantInMsg) {
				t.Errorf("expected message to mention %q, got %q", tc.wantInMsg, out.Message)
			}
		})
	}
}

// ---- fuzzy fallback on 404 ----

func TestNotFoundFallsBackToSearch(t *testing.T) {
	var query url.Values
	server := newServer(t, handlers{
		get: jsonHandler(http.StatusNotFound, map[string]any{"message": "not found"}),
		search: func(w http.ResponseWriter, r *http.Request) {
			query = r.URL.Query()
			jsonHandler(http.StatusOK, map[string]any{"results": []map[string]any{}})(w, r)
		},
	})
	defer server.Close()

	out := run(t, server, tools.Input{Name: "nyse"})
	if out.Status != tools.StatusError {
		t.Fatalf("expected error when the fuzzy search also returns nothing, got %q (%s)", out.Status, out.Message)
	}
	if got := query.Get("jobName"); got != "nyse" {
		t.Errorf("expected the fuzzy filter to be the requested name, got %q", got)
	}
}

func TestNotFoundSingleFuzzyMatchIsFetchedAndReturned(t *testing.T) {
	calls := 0
	server := newServer(t, handlers{
		get: func(w http.ResponseWriter, r *http.Request) {
			calls++
			if strings.HasSuffix(r.URL.Path, "/nyse") {
				jsonHandler(http.StatusNotFound, map[string]any{"message": "not found"})(w, r)
				return
			}
			jsonHandler(http.StatusOK, sampleJob())(w, r)
		},
		search: jsonHandler(http.StatusOK, map[string]any{"results": []map[string]any{{"jobName": "public.nyse"}}}),
	})
	defer server.Close()

	out := run(t, server, tools.Input{Name: "nyse"})
	if out.Status != tools.StatusSuccess {
		t.Fatalf("expected success after resolving the single fuzzy match, got %q (%s)", out.Status, out.Message)
	}
	if out.Job == nil || out.Job.JobName != "public.nyse" {
		t.Fatalf("expected the resolved job returned, got %+v", out.Job)
	}
	if calls != 2 {
		t.Errorf("expected exactly 2 get calls (exact miss + resolved fetch), got %d", calls)
	}
}

func TestNotFoundMultipleFuzzyMatchesNeedsSelection(t *testing.T) {
	server := newServer(t, handlers{
		get: jsonHandler(http.StatusNotFound, map[string]any{"message": "not found"}),
		search: jsonHandler(http.StatusOK, map[string]any{"results": []map[string]any{
			{"jobName": "public.nyse"},
			{"jobName": "public.nyse_1"},
		}}),
	})
	defer server.Close()

	out := run(t, server, tools.Input{Name: "nyse"})
	if out.Status != tools.StatusNeedsInput {
		t.Fatalf("expected needs_input for multiple fuzzy matches, got %q (%s)", out.Status, out.Message)
	}
	if len(out.CandidateJobNames) != 2 {
		t.Fatalf("expected 2 candidate names, got %v", out.CandidateJobNames)
	}
}

func TestTransportError(t *testing.T) {
	server := newServer(t, handlers{get: jsonHandler(http.StatusOK, sampleJob())})
	server.Close() // closed before use -> transport failure

	out := run(t, server, tools.Input{Name: "public.nyse"})
	if out.Status != tools.StatusError {
		t.Fatalf("expected error on transport failure, got %q (%s)", out.Status, out.Message)
	}
}

// ---- table-based lookup ----

func TestMissingNameAndTableNameNeedsInput(t *testing.T) {
	server := newServer(t, handlers{}) // no endpoint should be hit
	defer server.Close()

	out := run(t, server, tools.Input{})
	if out.Status != tools.StatusNeedsInput {
		t.Fatalf("expected needs_input when both name and tableName are empty, got %q (%s)", out.Status, out.Message)
	}
}

func TestTableNoMatchIsError(t *testing.T) {
	var query url.Values
	server := newServer(t, handlers{
		search: func(w http.ResponseWriter, r *http.Request) {
			query = r.URL.Query()
			jsonHandler(http.StatusOK, map[string]any{"results": []map[string]any{}})(w, r)
		},
	})
	defer server.Close()

	out := run(t, server, tools.Input{TableName: "NYSE"})
	if out.Status != tools.StatusError {
		t.Fatalf("expected error when no jobs match the table, got %q (%s)", out.Status, out.Message)
	}
	if got := query.Get("tableName"); got != "NYSE" {
		t.Errorf("expected the tableName filter to be sent, got %q", got)
	}
}

func TestTableSingleMatchIsFetchedAndReturned(t *testing.T) {
	server := newServer(t, handlers{
		get: jsonHandler(http.StatusOK, sampleJob()),
		search: jsonHandler(http.StatusOK, map[string]any{"results": []map[string]any{
			{"jobName": "public.nyse", "dataLocation": map[string]any{"schemaName": "PUBLIC"}},
		}}),
	})
	defer server.Close()

	out := run(t, server, tools.Input{TableName: "NYSE"})
	if out.Status != tools.StatusSuccess {
		t.Fatalf("expected success, got %q (%s)", out.Status, out.Message)
	}
	if out.Job == nil || out.Job.JobName != "public.nyse" {
		t.Fatalf("expected the resolved job returned, got %+v", out.Job)
	}
}

func TestTableMultipleMatchesNeedsSelection(t *testing.T) {
	server := newServer(t, handlers{
		search: jsonHandler(http.StatusOK, map[string]any{"results": []map[string]any{
			{"jobName": "public.nyse", "dataLocation": map[string]any{"schemaName": "PUBLIC", "dataSourceName": "PRODSALESDB"}},
			{"jobName": "sandbox.nyse_copy", "dataLocation": map[string]any{"schemaName": "SANDBOX", "dataSourceName": "DEVDB"}},
		}}),
	})
	defer server.Close()

	out := run(t, server, tools.Input{TableName: "NYSE"})
	if out.Status != tools.StatusNeedsInput {
		t.Fatalf("expected needs_input for multiple matches, got %q (%s)", out.Status, out.Message)
	}
	if len(out.Candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %v", out.Candidates)
	}
	if out.Candidates[0].SchemaName != "PUBLIC" || out.Candidates[1].DataSourceName != "DEVDB" {
		t.Errorf("expected data-location context on candidates, got %+v", out.Candidates)
	}
}

func TestTableSearchTransportError(t *testing.T) {
	server := newServer(t, handlers{})
	server.Close()

	out := run(t, server, tools.Input{TableName: "NYSE"})
	if out.Status != tools.StatusError {
		t.Fatalf("expected error on transport failure, got %q (%s)", out.Status, out.Message)
	}
}

func TestTableSingleMatchGetErrorSurfaces(t *testing.T) {
	server := newServer(t, handlers{
		get: jsonHandler(http.StatusForbidden, map[string]any{"message": "boom"}),
		search: jsonHandler(http.StatusOK, map[string]any{"results": []map[string]any{
			{"jobName": "public.nyse"},
		}}),
	})
	defer server.Close()

	out := run(t, server, tools.Input{TableName: "NYSE"})
	if out.Status != tools.StatusError {
		t.Fatalf("expected error, got %q (%s)", out.Status, out.Message)
	}
	if !strings.Contains(out.Message, "403") {
		t.Errorf("expected message to mention 403, got %q", out.Message)
	}
}
