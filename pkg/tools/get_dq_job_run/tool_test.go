package get_dq_job_run_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tools "github.com/collibra/chip/pkg/tools/get_dq_job_run"
	"github.com/collibra/chip/pkg/tools/testutil"
)

// handlers configures the mocked job-run endpoints. A nil handler means "must not be called" — the
// mux fails the test if that endpoint is hit.
type handlers struct {
	run      func(w http.ResponseWriter, r *http.Request) // GET /rest/dq/1.0/jobRuns/{id}
	monitors func(w http.ResponseWriter, r *http.Request) // GET /rest/dq/1.0/jobRuns/{id}/monitors
	profile  func(w http.ResponseWriter, r *http.Request) // GET /rest/dq/1.0/jobRuns/{id}/profile
}

func newServer(t *testing.T, h handlers) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/dq/1.0/jobRuns/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/monitors"):
			if h.monitors == nil {
				t.Errorf("unexpected monitors call: %s %s", r.Method, r.URL)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			h.monitors(w, r)
		case strings.HasSuffix(r.URL.Path, "/profile"):
			if h.profile == nil {
				t.Errorf("unexpected profile call: %s %s", r.Method, r.URL)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			h.profile(w, r)
		default:
			if h.run == nil {
				t.Errorf("unexpected run call: %s %s", r.Method, r.URL)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			h.run(w, r)
		}
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

// finishedRun is a terminal-run payload shaped like the public API's JobRun, with the
// terminal-only fields populated.
func finishedRun() map[string]any {
	return map[string]any{
		"jobRunId":             "9c1d1e3a-2b4f-4a6c-8d90-111213141516",
		"jobName":              "public.nyse",
		"status":               "FINISHED",
		"activity":             "",
		"runDate":              map[string]any{"kind": "TIMESTAMP", "value": "2025-10-22T13:00:00Z"},
		"startTime":            "2025-10-22T13:50:05Z",
		"endTime":              "2025-10-22T13:54:42Z",
		"executionTimeSeconds": 277,
		"score":                95.0,
		"activeMonitors":       20,
		"breakingMonitors":     1,
		"rowCount":             150000,
		"executedQuery":        "SELECT * FROM PUBLIC.NYSE",
	}
}

func runningRun() map[string]any {
	return map[string]any{
		"jobRunId":  "r-2",
		"jobName":   "public.nyse",
		"status":    "RUNNING",
		"activity":  "PROFILE",
		"startTime": "2025-10-22T13:50:05Z",
	}
}

func monitorsResult() map[string]any {
	return map[string]any{
		"adaptiveMonitors": []map[string]any{
			{"monitorName": "ROW_COUNT", "monitorType": "ROW_COUNT", "state": "PASSING", "observedValue": "150000"},
		},
		"customMonitors": []map[string]any{
			{"monitorName": "NAME_NOT_NULL_MONITOR", "state": "BREAKING", "score": 40, "rowsPassing": 80, "rowsBreaking": 20, "rowsTotal": 100},
		},
	}
}

func profileResult() map[string]any {
	return map[string]any{
		"jobRunId": "9c1d1e3a-2b4f-4a6c-8d90-111213141516",
		"jobName":  "public.nyse",
		"offset":   0,
		"limit":    100,
		"total":    2,
		"results": []map[string]any{
			{
				"columnName":  "order_id",
				"definedType": "BIGINT",
				"valueCount":  150000,
				"nullCount":   0,
				"emptyCount":  0,
				"uniqueCount": 150000,
				"min":         "1000",
				"max":         "999999",
				"topShapes": []map[string]any{
					{"pattern": "######", "count": 148500, "percentage": 99.0},
				},
			},
			{
				"columnName":  "amount",
				"definedType": "DECIMAL",
				"valueCount":  149880,
				"nullCount":   120,
				"emptyCount":  0,
				"uniqueCount": 4320,
				"min":         "0.99",
				"max":         "9999.99",
				"median":      "47.5",
				"q1":          "12.99",
				"q3":          "199.0",
				"topShapes":   nil,
			},
		},
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

func TestMissingRunIDNeedsInput(t *testing.T) {
	server := newServer(t, handlers{}) // no endpoint should be hit
	defer server.Close()

	out := run(t, server, tools.Input{})
	if out.Status != tools.StatusNeedsInput {
		t.Fatalf("expected needs_input when run_id is empty, got %q (%s)", out.Status, out.Message)
	}
}

// ---- happy path ----

func TestFinishedRunWithMonitorsSuccess(t *testing.T) {
	server := newServer(t, handlers{
		run:      jsonHandler(http.StatusOK, finishedRun()),
		monitors: jsonHandler(http.StatusOK, monitorsResult()),
		profile:  jsonHandler(http.StatusOK, profileResult()),
	})
	defer server.Close()

	out := run(t, server, tools.Input{RunID: "9c1d1e3a-2b4f-4a6c-8d90-111213141516"})
	if out.Status != tools.StatusSuccess {
		t.Fatalf("expected success, got %q (%s)", out.Status, out.Message)
	}
	if out.Run == nil {
		t.Fatal("expected run details")
	}
	if out.Run.Status != "FINISHED" || out.Run.JobName != "public.nyse" {
		t.Errorf("unexpected run identity: %+v", out.Run)
	}
	if out.Run.Score == nil || *out.Run.Score != 95.0 {
		t.Errorf("expected score 95.0, got %+v", out.Run.Score)
	}
	if out.Run.RowCount == nil || *out.Run.RowCount != 150000 {
		t.Errorf("expected rowCount 150000, got %+v", out.Run.RowCount)
	}
	if out.Run.ExecutionTimeSeconds == nil || *out.Run.ExecutionTimeSeconds != 277 {
		t.Errorf("expected executionTimeSeconds 277, got %+v", out.Run.ExecutionTimeSeconds)
	}
	if len(out.Run.AdaptiveMonitors) != 1 || out.Run.AdaptiveMonitors[0].MonitorName != "ROW_COUNT" {
		t.Errorf("expected adaptive monitor result surfaced, got %+v", out.Run.AdaptiveMonitors)
	}
	if len(out.Run.CustomMonitors) != 1 || out.Run.CustomMonitors[0].MonitorName != "NAME_NOT_NULL_MONITOR" || out.Run.CustomMonitors[0].Score != 40 {
		t.Errorf("expected custom monitor result surfaced, got %+v", out.Run.CustomMonitors)
	}
	if out.Run.JobDetailsLink == "" {
		t.Error("expected a job details link when jobName is known")
	}
	if len(out.Run.Profile) != 2 || out.Run.Profile[0].ColumnName != "order_id" {
		t.Errorf("expected column profile results surfaced, got %+v", out.Run.Profile)
	}
	if out.Run.Profile[0].TopShapes == nil || len(out.Run.Profile[0].TopShapes) != 1 || out.Run.Profile[0].TopShapes[0].Pattern != "######" {
		t.Errorf("expected top shapes surfaced for order_id, got %+v", out.Run.Profile[0].TopShapes)
	}
	if out.Run.Profile[1].Median != "47.5" || out.Run.Profile[1].Q1 != "12.99" {
		t.Errorf("expected numeric stats surfaced for amount, got %+v", out.Run.Profile[1])
	}
	if out.Run.TotalProfiledColumns == nil || *out.Run.TotalProfiledColumns != 2 {
		t.Errorf("expected totalProfiledColumns 2, got %+v", out.Run.TotalProfiledColumns)
	}
}

func TestInProgressRunHasNoTerminalFields(t *testing.T) {
	server := newServer(t, handlers{
		run:      jsonHandler(http.StatusOK, runningRun()),
		monitors: jsonHandler(http.StatusOK, map[string]any{"adaptiveMonitors": []map[string]any{}, "customMonitors": []map[string]any{}}),
		profile:  jsonHandler(http.StatusOK, map[string]any{"jobRunId": "r-2", "jobName": "public.nyse", "offset": 0, "limit": 100, "total": 0, "results": []map[string]any{}}),
	})
	defer server.Close()

	out := run(t, server, tools.Input{RunID: "r-2"})
	if out.Status != tools.StatusSuccess {
		t.Fatalf("expected success, got %q (%s)", out.Status, out.Message)
	}
	if out.Run.Status != "RUNNING" || out.Run.Activity != "PROFILE" {
		t.Errorf("unexpected in-progress run: %+v", out.Run)
	}
	if out.Run.Score != nil || out.Run.RowCount != nil || out.Run.ExecutionTimeSeconds != nil {
		t.Errorf("expected terminal-only fields to be absent while in progress, got %+v", out.Run)
	}
}

// ---- run lookup errors ----

func TestRunLookupErrorMapping(t *testing.T) {
	cases := []struct {
		code      int
		wantInMsg string
	}{
		{http.StatusNotFound, "404"},
		{http.StatusUnauthorized, "401"},
		{http.StatusForbidden, "403"},
		{http.StatusBadRequest, "400"},
		{http.StatusInternalServerError, "500"},
	}
	for _, tc := range cases {
		t.Run(tc.wantInMsg, func(t *testing.T) {
			server := newServer(t, handlers{run: jsonHandler(tc.code, map[string]any{"message": "boom"})})
			defer server.Close()

			out := run(t, server, tools.Input{RunID: "r-1"})
			if out.Status != tools.StatusError {
				t.Fatalf("expected error for HTTP %d, got %q (%s)", tc.code, out.Status, out.Message)
			}
			if !strings.Contains(out.Message, tc.wantInMsg) {
				t.Errorf("expected message to mention %q, got %q", tc.wantInMsg, out.Message)
			}
		})
	}
}

// ---- monitors lookup degrades gracefully ----

func TestMonitorsLookupFailureStillReturnsRun(t *testing.T) {
	server := newServer(t, handlers{
		run:      jsonHandler(http.StatusOK, finishedRun()),
		monitors: jsonHandler(http.StatusForbidden, map[string]any{"message": "boom"}),
		profile:  jsonHandler(http.StatusOK, profileResult()),
	})
	defer server.Close()

	out := run(t, server, tools.Input{RunID: "9c1d1e3a-2b4f-4a6c-8d90-111213141516"})
	if out.Status != tools.StatusSuccess {
		t.Fatalf("expected success (run details still returned), got %q (%s)", out.Status, out.Message)
	}
	if out.Run == nil || out.Run.Score == nil {
		t.Fatal("expected run details with the aggregate score even when per-monitor results fail")
	}
	if len(out.Run.AdaptiveMonitors) != 0 || len(out.Run.CustomMonitors) != 0 {
		t.Errorf("expected no monitor results when the monitors call failed, got %+v", out.Run)
	}
	if len(out.Run.Profile) != 2 {
		t.Errorf("expected profile results still surfaced when only monitors failed, got %+v", out.Run.Profile)
	}
	if !strings.Contains(out.Guidance, "403") {
		t.Errorf("expected the guidance to mention the monitors-call failure, got %q", out.Guidance)
	}
}

func TestProfileLookupFailureStillReturnsRun(t *testing.T) {
	server := newServer(t, handlers{
		run:      jsonHandler(http.StatusOK, finishedRun()),
		monitors: jsonHandler(http.StatusOK, monitorsResult()),
		profile:  jsonHandler(http.StatusForbidden, map[string]any{"message": "boom"}),
	})
	defer server.Close()

	out := run(t, server, tools.Input{RunID: "9c1d1e3a-2b4f-4a6c-8d90-111213141516"})
	if out.Status != tools.StatusSuccess {
		t.Fatalf("expected success (run details still returned), got %q (%s)", out.Status, out.Message)
	}
	if out.Run == nil || out.Run.Score == nil {
		t.Fatal("expected run details with the aggregate score even when the profile lookup fails")
	}
	if len(out.Run.Profile) != 0 {
		t.Errorf("expected no profile results when the profile call failed, got %+v", out.Run.Profile)
	}
	if len(out.Run.AdaptiveMonitors) != 1 {
		t.Errorf("expected monitor results still surfaced when only profile failed, got %+v", out.Run.AdaptiveMonitors)
	}
	if !strings.Contains(out.Guidance, "403") {
		t.Errorf("expected the guidance to mention the profile-call failure, got %q", out.Guidance)
	}
}

func TestTransportError(t *testing.T) {
	server := newServer(t, handlers{run: jsonHandler(http.StatusOK, finishedRun())})
	server.Close() // closed before use -> transport failure

	out := run(t, server, tools.Input{RunID: "r-1"})
	if out.Status != tools.StatusError {
		t.Fatalf("expected error on transport failure, got %q (%s)", out.Status, out.Message)
	}
}
