package get_dq_job_run_monitors_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tools "github.com/collibra/chip/pkg/tools/get_dq_job_run_monitors"
	"github.com/collibra/chip/pkg/tools/testutil"
)

const runID = "9c1d1e3a-2b4f-4a6c-8d90-111213141516"

// newServer mocks GET /rest/dq/1.0/jobRuns/{id}/monitors. A nil handler means
// "must not be called".
func newServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/dq/1.0/jobRuns/", func(w http.ResponseWriter, r *http.Request) {
		if handler == nil {
			t.Errorf("unexpected monitors call: %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if !strings.HasSuffix(r.URL.Path, "/monitors") {
			t.Errorf("path = %q, want it to end in /monitors", r.URL.Path)
		}
		handler(w, r)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
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

// monitorsPayload is shaped like the public API's JobRunMonitorsResult: two adaptive
// monitors (one breaking) and two custom rules (one erroring).
func monitorsPayload() map[string]any {
	return map[string]any{
		"adaptiveMonitors": []map[string]any{
			{
				"monitorName":   "amount__NULL",
				"monitorType":   "NULL",
				"primaryColumn": "amount",
				"state":         "BREAKING",
				"observedValue": "120",
				"expectedMin":   "0",
				"expectedMax":   "15",
				"tolerance":     "NARROW",
				"dimensions":    []string{"Completeness"},
			},
			{
				"monitorName":   "ROW_COUNT",
				"monitorType":   "ROW_COUNT",
				"state":         "PASSING",
				"observedValue": "150000",
				"expectedMin":   "140000",
				"expectedMax":   "160000",
				"tolerance":     "NEUTRAL",
			},
		},
		"customMonitors": []map[string]any{
			{
				"monitorName":        "amount_is_positive",
				"state":              "PASSING",
				"score":              98.5,
				"breakingPercentage": 1.5,
				"rowsPassing":        147750,
				"rowsBreaking":       2250,
				"rowsTotal":          150000,
				"tolerance":          5000,
				"dimensions":         []string{"Validity"},
			},
			{
				"monitorName": "order_id_join_check",
				"state":       "EXCEPTION",
				"score":       0,
				"exception":   "SQL compilation error",
			},
		},
	}
}

func TestMissingRunIDNeedsInput(t *testing.T) {
	srv := newServer(t, nil)
	out, err := tools.NewTool(testutil.NewClient(srv)).Handler(t.Context(), tools.Input{RunID: "   "})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != tools.StatusNeedsInput {
		t.Fatalf("status = %q, want needs_input (%s)", out.Status, out.Message)
	}
	if out.Guidance == "" {
		t.Error("guidance is empty, want an explanation of what to supply")
	}
}

func TestMonitorsSuccess(t *testing.T) {
	srv := newServer(t, jsonHandler(http.StatusOK, monitorsPayload()))

	out, err := tools.NewTool(testutil.NewClient(srv)).Handler(t.Context(), tools.Input{RunID: runID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != tools.StatusSuccess {
		t.Fatalf("status = %q, want success (%s)", out.Status, out.Message)
	}
	monitors := out.Monitors
	if monitors == nil {
		t.Fatal("monitors = nil, want the run's breakdown")
	}
	if len(monitors.AdaptiveMonitors) != 2 || len(monitors.CustomMonitors) != 2 {
		t.Fatalf("monitors = %d adaptive / %d custom, want 2 and 2", len(monitors.AdaptiveMonitors), len(monitors.CustomMonitors))
	}

	adaptive := monitors.AdaptiveMonitors[0]
	if adaptive.ObservedValue != "120" || adaptive.ExpectedMin != "0" || adaptive.ExpectedMax != "15" {
		t.Errorf("adaptive observed/expected = %q in [%q,%q], want 120 in [0,15]", adaptive.ObservedValue, adaptive.ExpectedMin, adaptive.ExpectedMax)
	}
	if adaptive.Tolerance != "NARROW" {
		t.Errorf("adaptive tolerance = %q, want NARROW", adaptive.Tolerance)
	}

	custom := monitors.CustomMonitors[0]
	if custom.Score != 98.5 || custom.RowsBreaking != 2250 {
		t.Errorf("custom score/rowsBreaking = %v/%v, want 98.5/2250", custom.Score, custom.RowsBreaking)
	}
	if custom.Tolerance != 5000 {
		t.Errorf("custom tolerance = %d, want 5000", custom.Tolerance)
	}
	if got := monitors.CustomMonitors[1].Exception; got != "SQL compilation error" {
		t.Errorf("exception = %q, want the SQL compilation error", got)
	}
}

func TestMonitorsSummaryCountsStates(t *testing.T) {
	srv := newServer(t, jsonHandler(http.StatusOK, monitorsPayload()))

	out, _ := tools.NewTool(testutil.NewClient(srv)).Handler(t.Context(), tools.Input{RunID: runID})
	summary := out.Monitors.Summary
	if summary.Total != 4 || summary.Adaptive != 2 || summary.Custom != 2 {
		t.Errorf("summary totals = %+v, want 4 total (2 adaptive, 2 custom)", summary)
	}
	if summary.Passing != 2 || summary.Breaking != 1 || summary.Exception != 1 {
		t.Errorf("summary states = %+v, want 2 passing, 1 breaking, 1 exception", summary)
	}
	if !strings.Contains(out.Message, "1 breaking, 1 exception") {
		t.Errorf("message = %q, want it to call out the failing monitors", out.Message)
	}
}

func TestMonitorsNoResultsExplainsWhy(t *testing.T) {
	srv := newServer(t, jsonHandler(http.StatusOK, map[string]any{
		"adaptiveMonitors": []map[string]any{},
		"customMonitors":   []map[string]any{},
	}))

	out, _ := tools.NewTool(testutil.NewClient(srv)).Handler(t.Context(), tools.Input{RunID: runID})
	if out.Status != tools.StatusError {
		t.Fatalf("status = %q, want error", out.Status)
	}
	for _, want := range []string{"did not complete", "no monitors configured", "dq_get_job_run"} {
		if !strings.Contains(out.Guidance, want) {
			t.Errorf("guidance = %q, want it to mention %q", out.Guidance, want)
		}
	}
}

func TestMonitorsLookupErrorMapping(t *testing.T) {
	for _, tc := range []struct {
		name     string
		code     int
		wantText string
	}{
		{"not found", http.StatusNotFound, "404"},
		{"unauthorized", http.StatusUnauthorized, "401"},
		{"forbidden", http.StatusForbidden, "403"},
		{"bad request", http.StatusBadRequest, "400"},
		{"server error", http.StatusInternalServerError, "500"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := newServer(t, jsonHandler(tc.code, map[string]any{"message": "boom"}))
			out, err := tools.NewTool(testutil.NewClient(srv)).Handler(t.Context(), tools.Input{RunID: runID})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if out.Status != tools.StatusError {
				t.Fatalf("status = %q, want error", out.Status)
			}
			if !strings.Contains(out.Message, tc.wantText) {
				t.Errorf("message = %q, want it to report HTTP %s", out.Message, tc.wantText)
			}
			if out.Guidance == "" {
				t.Error("guidance is empty, want actionable next steps")
			}
		})
	}
}

func TestMonitorsTransportError(t *testing.T) {
	srv := newServer(t, jsonHandler(http.StatusOK, nil))
	client := testutil.NewClient(srv)
	srv.Close()

	out, err := tools.NewTool(client).Handler(t.Context(), tools.Input{RunID: runID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != tools.StatusError {
		t.Fatalf("status = %q, want error", out.Status)
	}
	if !strings.Contains(out.Guidance, "Retry") {
		t.Errorf("guidance = %q, want a retry suggestion", out.Guidance)
	}
}
