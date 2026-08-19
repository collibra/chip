package cancel_dq_job_run_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tools "github.com/collibra/chip/pkg/tools/cancel_dq_job_run"
	"github.com/collibra/chip/pkg/tools/testutil"
)

// handlers configures the mocked job-run endpoints. A nil handler means "must not be called" —
// the mux fails the test if that endpoint is hit, which lets each test assert that, e.g., cancel
// is NOT reached on a terminal-state run.
type handlers struct {
	status func(w http.ResponseWriter, r *http.Request) // GET /rest/dq/1.0/jobRuns/{id}
	search func(w http.ResponseWriter, r *http.Request) // GET /rest/dq/1.0/jobRuns
	cancel func(w http.ResponseWriter, r *http.Request) // POST /rest/dq/1.0/jobRuns/{id}/cancel
}

func newServer(t *testing.T, h handlers) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	// Exact path -> the by-name search.
	mux.HandleFunc("/rest/dq/1.0/jobRuns", func(w http.ResponseWriter, r *http.Request) {
		if h.search == nil {
			t.Errorf("unexpected search call: %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		h.search(w, r)
	})
	// Subtree -> status lookup and cancel (distinguished by the /cancel suffix).
	mux.HandleFunc("/rest/dq/1.0/jobRuns/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/cancel") {
			if h.cancel == nil {
				t.Errorf("unexpected cancel call: %s %s", r.Method, r.URL)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			h.cancel(w, r)
			return
		}
		if h.status == nil {
			t.Errorf("unexpected status call: %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		h.status(w, r)
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

// ---- input validation ----

func TestNeitherInputNeedsInput(t *testing.T) {
	server := newServer(t, handlers{}) // no endpoint should be hit
	defer server.Close()

	out := run(t, server, tools.Input{})
	if out.Status != tools.StatusNeedsInput {
		t.Fatalf("expected needs_input when neither id nor name given, got %q (%s)", out.Status, out.Message)
	}
}

func TestBothInputsNeedsInput(t *testing.T) {
	server := newServer(t, handlers{})
	defer server.Close()

	out := run(t, server, tools.Input{JobRunID: "r-1", JobName: "sales.orders"})
	if out.Status != tools.StatusNeedsInput {
		t.Fatalf("expected needs_input when both id and name given, got %q (%s)", out.Status, out.Message)
	}
}

// ---- Path A: by jobRunId ----

func TestByRunIDHappyPath(t *testing.T) {
	var canceledID string
	server := newServer(t, handlers{
		status: jsonHandler(http.StatusOK, map[string]any{"jobRunId": "r-1", "jobName": "sales.orders", "status": "RUNNING"}),
		cancel: func(w http.ResponseWriter, r *http.Request) {
			canceledID = strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/rest/dq/1.0/jobRuns/"), "/cancel")
			w.WriteHeader(http.StatusAccepted)
		},
	})
	defer server.Close()

	out := run(t, server, tools.Input{JobRunID: "r-1"})
	if out.Status != tools.StatusCanceled {
		t.Fatalf("expected canceled, got %q (%s)", out.Status, out.Message)
	}
	if canceledID != "r-1" {
		t.Errorf("expected cancel called for r-1, got %q", canceledID)
	}
	if out.JobName != "sales.orders" {
		t.Errorf("expected jobName resolved from the run, got %q", out.JobName)
	}
	if out.JobDetailsLink == "" {
		t.Errorf("expected a job details link when jobName is known")
	}
}

func TestByRunIDTerminalStateRefused(t *testing.T) {
	// cancel is nil -> the test fails if the handler tries to cancel a terminal run.
	server := newServer(t, handlers{
		status: jsonHandler(http.StatusOK, map[string]any{"jobRunId": "r-1", "jobName": "sales.orders", "status": "FINISHED"}),
	})
	defer server.Close()

	out := run(t, server, tools.Input{JobRunID: "r-1"})
	if out.Status != tools.StatusError {
		t.Fatalf("expected error for a terminal run, got %q (%s)", out.Status, out.Message)
	}
	if out.RunState != "FINISHED" {
		t.Errorf("expected runState FINISHED surfaced, got %q", out.RunState)
	}
}

func TestByRunIDStatusNotFound(t *testing.T) {
	server := newServer(t, handlers{
		status: jsonHandler(http.StatusNotFound, map[string]any{"message": "not found"}),
	})
	defer server.Close()

	out := run(t, server, tools.Input{JobRunID: "missing"})
	if out.Status != tools.StatusError {
		t.Fatalf("expected error for a 404 status lookup, got %q (%s)", out.Status, out.Message)
	}
	if !strings.Contains(out.Message, "404") {
		t.Errorf("expected the 404 surfaced in the message, got %q", out.Message)
	}
}

// ---- Path B: by jobName ----

func TestByNameSingleRun(t *testing.T) {
	var canceledID string
	server := newServer(t, handlers{
		search: jsonHandler(http.StatusOK, map[string]any{"results": []map[string]any{
			{"jobRunId": "r-9", "jobName": "sales.orders", "status": "RUNNING"},
		}}),
		cancel: func(w http.ResponseWriter, r *http.Request) {
			canceledID = strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/rest/dq/1.0/jobRuns/"), "/cancel")
			w.WriteHeader(http.StatusAccepted)
		},
	})
	defer server.Close()

	out := run(t, server, tools.Input{JobName: "sales.orders"})
	if out.Status != tools.StatusCanceled {
		t.Fatalf("expected canceled, got %q (%s)", out.Status, out.Message)
	}
	if canceledID != "r-9" {
		t.Errorf("expected cancel called for r-9, got %q", canceledID)
	}
}

func TestByNameNoRuns(t *testing.T) {
	// cancel nil -> must not be reached when there are no cancellable runs.
	server := newServer(t, handlers{
		search: jsonHandler(http.StatusOK, map[string]any{"results": []map[string]any{}}),
	})
	defer server.Close()

	out := run(t, server, tools.Input{JobName: "sales.orders"})
	if out.Status != tools.StatusError {
		t.Fatalf("expected error when no cancellable runs, got %q (%s)", out.Status, out.Message)
	}
}

func TestByNameMultipleRunsNeedsSelection(t *testing.T) {
	// cancel nil -> must not be reached; the tool should ask the caller to pick one.
	server := newServer(t, handlers{
		search: jsonHandler(http.StatusOK, map[string]any{"results": []map[string]any{
			{"jobRunId": "r-1", "jobName": "sales.orders", "status": "RUNNING", "startTime": "2025-10-22T13:50:05Z"},
			{"jobRunId": "r-2", "jobName": "sales.orders", "status": "SUBMITTED"},
		}}),
	})
	defer server.Close()

	out := run(t, server, tools.Input{JobName: "sales.orders"})
	if out.Status != tools.StatusNeedsInput {
		t.Fatalf("expected needs_input for multiple runs, got %q (%s)", out.Status, out.Message)
	}
	if len(out.CandidateRuns) != 2 {
		t.Fatalf("expected 2 candidate runs, got %d", len(out.CandidateRuns))
	}
	if out.CandidateRuns[0].JobRunID != "r-1" || out.CandidateRuns[1].JobRunID != "r-2" {
		t.Errorf("unexpected candidate ids: %+v", out.CandidateRuns)
	}
	// startedAt comes from the run's startTime as the API returned it; a run without one stays blank.
	if out.CandidateRuns[0].StartedAt != "2025-10-22T13:50:05Z" {
		t.Errorf("expected startedAt from the run's startTime, got %q", out.CandidateRuns[0].StartedAt)
	}
	if out.CandidateRuns[1].StartedAt != "" {
		t.Errorf("expected a blank startedAt when the run has no startTime, got %q", out.CandidateRuns[1].StartedAt)
	}
}

// ---- cancel error mapping ----

func TestCancelErrorMapping(t *testing.T) {
	cases := []struct {
		code      int
		wantInMsg string
	}{
		{http.StatusBadRequest, "400"},
		{http.StatusUnauthorized, "401"},
		{http.StatusForbidden, "403"},
		{http.StatusNotFound, "404"},
		{http.StatusConflict, "409"},
		{http.StatusInternalServerError, "500"},
	}
	for _, tc := range cases {
		t.Run(tc.wantInMsg, func(t *testing.T) {
			server := newServer(t, handlers{
				status: jsonHandler(http.StatusOK, map[string]any{"jobRunId": "r-1", "jobName": "sales.orders", "status": "RUNNING"}),
				cancel: jsonHandler(tc.code, map[string]any{"message": "boom"}),
			})
			defer server.Close()

			out := run(t, server, tools.Input{JobRunID: "r-1"})
			if out.Status != tools.StatusError {
				t.Fatalf("expected error for HTTP %d, got %q (%s)", tc.code, out.Status, out.Message)
			}
			if !strings.Contains(out.Message, tc.wantInMsg) {
				t.Errorf("expected message to mention %q, got %q", tc.wantInMsg, out.Message)
			}
			if out.JobRunID != "r-1" {
				t.Errorf("expected jobRunId echoed on error, got %q", out.JobRunID)
			}
		})
	}
}

func TestTransportError(t *testing.T) {
	server := newServer(t, handlers{
		status: jsonHandler(http.StatusOK, map[string]any{"jobRunId": "r-1", "jobName": "sales.orders", "status": "RUNNING"}),
		cancel: jsonHandler(http.StatusAccepted, nil),
	})
	server.Close() // closed before use -> transport failure

	out := run(t, server, tools.Input{JobRunID: "r-1"})
	if out.Status != tools.StatusError {
		t.Fatalf("expected error on transport failure, got %q (%s)", out.Status, out.Message)
	}
}
