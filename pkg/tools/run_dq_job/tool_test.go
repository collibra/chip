package run_dq_job_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	tools "github.com/collibra/chip/pkg/tools/run_dq_job"
	"github.com/collibra/chip/pkg/tools/testutil"
)

// handlers configures the mocked job endpoints. A nil handler means "must not be called" — the
// mux fails the test if that endpoint is hit, which lets each test assert that, e.g., run is NOT
// reached when confirm is false or the job lookup fails.
type handlers struct {
	get    func(w http.ResponseWriter, r *http.Request) // GET  /rest/dq/1.0/jobs/{jobName}
	search func(w http.ResponseWriter, r *http.Request) // GET  /rest/dq/1.0/jobs
	run    func(w http.ResponseWriter, r *http.Request) // POST /rest/dq/1.0/jobs/{jobName}/run
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
		if strings.HasSuffix(r.URL.Path, "/run") {
			if h.run == nil {
				t.Errorf("unexpected run call: %s %s", r.Method, r.URL)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			h.run(w, r)
			return
		}
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

func run(t *testing.T, server *httptest.Server, in tools.Input) tools.Output {
	t.Helper()
	out, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), in)
	if err != nil {
		t.Fatalf("handler returned a Go error (should surface via Output): %v", err)
	}
	return out
}

// ---- input validation (no network call may happen) ----

func TestMissingJobNameNeedsInput(t *testing.T) {
	server := newServer(t, handlers{}) // no endpoint should be hit
	defer server.Close()

	out := run(t, server, tools.Input{})
	if out.Status != tools.StatusNeedsInput {
		t.Fatalf("expected needs_input when jobName is missing, got %q (%s)", out.Status, out.Message)
	}
}

func TestInvalidJobNameNeedsInput(t *testing.T) {
	server := newServer(t, handlers{})
	defer server.Close()

	out := run(t, server, tools.Input{JobName: "-bad name!"})
	if out.Status != tools.StatusNeedsInput {
		t.Fatalf("expected needs_input for an invalid jobName, got %q (%s)", out.Status, out.Message)
	}
}

func TestHalfSuppliedRunDateNeedsInput(t *testing.T) {
	server := newServer(t, handlers{}) // no endpoint should be hit — validated before lookup
	defer server.Close()

	out := run(t, server, tools.Input{JobName: "sales.orders", RunDateKind: "DATE", Confirm: true})
	if out.Status != tools.StatusNeedsInput {
		t.Fatalf("expected needs_input when runDateValue is missing, got %q (%s)", out.Status, out.Message)
	}
}

func TestHalfSuppliedBackrunNeedsInput(t *testing.T) {
	server := newServer(t, handlers{})
	defer server.Close()

	out := run(t, server, tools.Input{JobName: "sales.orders", BackrunTimeBin: "DAY", Confirm: true})
	if out.Status != tools.StatusNeedsInput {
		t.Fatalf("expected needs_input when backrunBinValue is missing, got %q (%s)", out.Status, out.Message)
	}
}

// ---- confirm checkpoint ----

func TestPreviewByDefaultQueuesNothing(t *testing.T) {
	// run is nil: the mux fails the test if the run endpoint is touched.
	server := newServer(t, handlers{
		get: jsonHandler(http.StatusOK, map[string]any{"jobName": "sales.orders"}),
	})
	defer server.Close()

	out := run(t, server, tools.Input{JobName: "sales.orders"})
	if out.Status != tools.StatusPreview {
		t.Fatalf("expected preview without confirm, got %q (%s)", out.Status, out.Message)
	}
	if out.JobRunID != "" {
		t.Fatalf("preview must not return a jobRunId, got %q", out.JobRunID)
	}
	if out.RunPlan == nil || !out.RunPlan.UsesJobDefaults {
		t.Fatalf("expected a runPlan using job defaults, got %+v", out.RunPlan)
	}
}

// The preview must echo every field that confirm=true would send (STANDARDS §5.2).
func TestPreviewEchoesEveryOverride(t *testing.T) {
	server := newServer(t, handlers{
		get: jsonHandler(http.StatusOK, map[string]any{"jobName": "sales.orders"}),
	})
	defer server.Close()

	out := run(t, server, tools.Input{
		JobName:         "sales.orders",
		RunDateKind:     "DATE",
		RunDateValue:    "2025-10-22",
		RunDateEndKind:  "DATE",
		RunDateEndValue: "2025-10-23",
		BackrunTimeBin:  "DAY",
		BackrunBinValue: 10,
	})
	if out.Status != tools.StatusPreview {
		t.Fatalf("expected preview, got %q (%s)", out.Status, out.Message)
	}
	plan := out.RunPlan
	if plan == nil {
		t.Fatal("expected a runPlan on preview")
	}
	if plan.UsesJobDefaults {
		t.Fatal("expected usesJobDefaults=false when overrides are supplied")
	}
	if !strings.Contains(plan.RunDate, "2025-10-22") || !strings.Contains(plan.RunDate, "DATE") {
		t.Fatalf("runDate not echoed: %q", plan.RunDate)
	}
	if !strings.Contains(plan.RunDateEnd, "2025-10-23") {
		t.Fatalf("runDateEnd not echoed: %q", plan.RunDateEnd)
	}
	if !strings.Contains(plan.Backrun, "10") || !strings.Contains(plan.Backrun, "DAY") {
		t.Fatalf("backrun not echoed: %q", plan.Backrun)
	}
}

// A backfill can queue many runs, so its preview must say so out loud.
func TestPreviewWarnsAboutBackfill(t *testing.T) {
	server := newServer(t, handlers{
		get: jsonHandler(http.StatusOK, map[string]any{"jobName": "sales.orders"}),
	})
	defer server.Close()

	out := run(t, server, tools.Input{JobName: "sales.orders", BackrunTimeBin: "DAY", BackrunBinValue: 30})
	if out.Status != tools.StatusPreview {
		t.Fatalf("expected preview, got %q (%s)", out.Status, out.Message)
	}
	if !strings.Contains(out.Message, "large number of runs") {
		t.Fatalf("expected the backfill preview to warn about run volume, got %q", out.Message)
	}
}

// ---- happy path (confirm=true) ----

func TestRunWithNoOverridesHappyPath(t *testing.T) {
	var gotGetPath, gotRunPath, gotBody string
	server := newServer(t, handlers{
		get: func(w http.ResponseWriter, r *http.Request) {
			gotGetPath = r.URL.Path
			jsonHandler(http.StatusOK, map[string]any{"jobName": "sales.orders", "jobType": "PULLUP"})(w, r)
		},
		run: func(w http.ResponseWriter, r *http.Request) {
			gotRunPath = r.URL.Path
			body := make([]byte, r.ContentLength)
			_, _ = r.Body.Read(body)
			gotBody = string(body)
			jsonHandler(http.StatusAccepted, map[string]any{"jobRunId": "run-1"})(w, r)
		},
	})
	defer server.Close()

	out := run(t, server, tools.Input{JobName: "sales.orders", Confirm: true})
	if out.Status != tools.StatusRunning {
		t.Fatalf("expected running, got %q (%s)", out.Status, out.Message)
	}
	if out.JobRunID != "run-1" {
		t.Fatalf("expected jobRunId=run-1, got %q", out.JobRunID)
	}
	if gotGetPath != "/rest/dq/1.0/jobs/sales.orders" {
		t.Fatalf("unexpected GET path: %s", gotGetPath)
	}
	if gotRunPath != "/rest/dq/1.0/jobs/sales.orders/run" {
		t.Fatalf("unexpected run path: %s", gotRunPath)
	}
	if gotBody != "{}" {
		t.Fatalf("expected an empty run request body with no overrides, got %q", gotBody)
	}
}

func TestRunWithOverridesHappyPath(t *testing.T) {
	var gotBody struct {
		RunDate struct {
			Kind  string `json:"kind"`
			Value string `json:"value"`
		} `json:"runDate"`
		Backrun struct {
			TimeBin  string `json:"timeBin"`
			BinValue int    `json:"binValue"`
		} `json:"backrun"`
	}
	server := newServer(t, handlers{
		get: jsonHandler(http.StatusOK, map[string]any{"jobName": "sales.orders"}),
		run: func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			jsonHandler(http.StatusAccepted, map[string]any{"jobRunId": "run-2"})(w, r)
		},
	})
	defer server.Close()

	out := run(t, server, tools.Input{
		JobName:         "sales.orders",
		Confirm:         true,
		RunDateKind:     "DATE",
		RunDateValue:    "2025-10-22",
		BackrunTimeBin:  "DAY",
		BackrunBinValue: 10,
	})
	if out.Status != tools.StatusRunning {
		t.Fatalf("expected running, got %q (%s)", out.Status, out.Message)
	}
	if gotBody.RunDate.Kind != "DATE" || gotBody.RunDate.Value != "2025-10-22" {
		t.Fatalf("expected runDate override in request body, got %+v", gotBody.RunDate)
	}
	if gotBody.Backrun.TimeBin != "DAY" || gotBody.Backrun.BinValue != 10 {
		t.Fatalf("expected backrun override in request body, got %+v", gotBody.Backrun)
	}
}

// ---- job-name resolution ----

func TestFuzzyMatchResolvesSingleCandidate(t *testing.T) {
	var searchQuery url.Values
	var gotRunPath string
	server := newServer(t, handlers{
		get: func(w http.ResponseWriter, r *http.Request) {
			// Exact lookup misses; the fuzzy search then resolves the real name.
			if strings.HasSuffix(r.URL.Path, "/orders") {
				jsonHandler(http.StatusNotFound, nil)(w, r)
				return
			}
			jsonHandler(http.StatusOK, map[string]any{"jobName": "sales.orders"})(w, r)
		},
		search: func(w http.ResponseWriter, r *http.Request) {
			searchQuery = r.URL.Query()
			jsonHandler(http.StatusOK, map[string]any{"results": []map[string]any{{"jobName": "sales.orders"}}})(w, r)
		},
		run: func(w http.ResponseWriter, r *http.Request) {
			gotRunPath = r.URL.Path
			jsonHandler(http.StatusAccepted, map[string]any{"jobRunId": "run-3"})(w, r)
		},
	})
	defer server.Close()

	out := run(t, server, tools.Input{JobName: "orders", Confirm: true})
	if out.Status != tools.StatusRunning {
		t.Fatalf("expected running after resolving the single fuzzy match, got %q (%s)", out.Status, out.Message)
	}
	if searchQuery.Get("jobName") != "orders" {
		t.Fatalf("expected the fuzzy search to filter on the supplied name, got %q", searchQuery.Get("jobName"))
	}
	if gotRunPath != "/rest/dq/1.0/jobs/sales.orders/run" {
		t.Fatalf("expected the run to target the resolved name, got %s", gotRunPath)
	}
	if out.JobName != "sales.orders" {
		t.Fatalf("expected the resolved job name in the output, got %q", out.JobName)
	}
}

func TestAmbiguousNameReturnsCandidates(t *testing.T) {
	// run is nil: an ambiguous name must never queue anything.
	server := newServer(t, handlers{
		get: jsonHandler(http.StatusNotFound, nil),
		search: jsonHandler(http.StatusOK, map[string]any{"results": []map[string]any{
			{"jobName": "sales.orders"},
			{"jobName": "sales.orders_eu"},
		}}),
	})
	defer server.Close()

	out := run(t, server, tools.Input{JobName: "orders", Confirm: true})
	if out.Status != tools.StatusNeedsInput {
		t.Fatalf("expected needs_input for an ambiguous name, got %q (%s)", out.Status, out.Message)
	}
	if len(out.CandidateJobNames) != 2 {
		t.Fatalf("expected 2 candidates, got %v", out.CandidateJobNames)
	}
	if out.JobRunID != "" {
		t.Fatalf("an ambiguous name must not queue a run, got jobRunId %q", out.JobRunID)
	}
}

// ---- errors ----

func TestJobNotFoundAfterFuzzySearch(t *testing.T) {
	server := newServer(t, handlers{
		get:    jsonHandler(http.StatusNotFound, nil),
		search: jsonHandler(http.StatusOK, map[string]any{"results": []map[string]any{}}),
	})
	defer server.Close()

	out := run(t, server, tools.Input{JobName: "missing.job", Confirm: true})
	if out.Status != tools.StatusError {
		t.Fatalf("expected error, got %q (%s)", out.Status, out.Message)
	}
	if out.JobRunID != "" {
		t.Fatalf("expected no jobRunId, got %q", out.JobRunID)
	}
}

func TestRunRejectedByServer(t *testing.T) {
	server := newServer(t, handlers{
		get: jsonHandler(http.StatusOK, map[string]any{"jobName": "sales.orders"}),
		run: jsonHandler(http.StatusForbidden, nil),
	})
	defer server.Close()

	out := run(t, server, tools.Input{JobName: "sales.orders", Confirm: true})
	if out.Status != tools.StatusError {
		t.Fatalf("expected error, got %q (%s)", out.Status, out.Message)
	}
	if !strings.Contains(out.Message, "403") {
		t.Fatalf("expected message to mention 403, got %q", out.Message)
	}
	if !strings.Contains(out.Guidance, "DATA_QUALITY_JOB_RUN") {
		t.Fatalf("expected 403 guidance to name the DQ permission, got %q", out.Guidance)
	}
}
