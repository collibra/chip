package delete_dq_job_run_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	tools "github.com/collibra/chip/pkg/tools/delete_dq_job_run"
	"github.com/collibra/chip/pkg/tools/testutil"
)

// handlers configures the mocked job-run endpoints. A nil handler means "must not be called" —
// the mux fails the test if that endpoint is hit, which lets each test assert that, e.g., delete
// is NOT reached without confirm=true.
type handlers struct {
	status func(w http.ResponseWriter, r *http.Request) // GET    /rest/dq/1.0/jobRuns/{id}
	search func(w http.ResponseWriter, r *http.Request) // GET    /rest/dq/1.0/jobRuns
	delete func(w http.ResponseWriter, r *http.Request) // DELETE /rest/dq/1.0/jobRuns/{id}
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
	// Subtree -> status lookup and delete, distinguished by the HTTP method.
	mux.HandleFunc("/rest/dq/1.0/jobRuns/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			if h.delete == nil {
				t.Errorf("unexpected delete call: %s %s", r.Method, r.URL)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			h.delete(w, r)
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

// finishedRun is the canonical terminal-run status payload, shaped like the public API's JobRun.
func finishedRun() map[string]any {
	return map[string]any{
		"jobRunId":  "r-1",
		"jobName":   "sales.orders",
		"status":    "FINISHED",
		"runDate":   map[string]any{"kind": "TIMESTAMP", "value": "2025-10-22T13:00:00Z"},
		"startTime": "2025-10-22T13:50:05Z",
		"endTime":   "2025-10-22T13:54:42Z",
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

	out := run(t, server, tools.Input{JobRunID: "r-1", JobName: "sales.orders", Confirm: true})
	if out.Status != tools.StatusNeedsInput {
		t.Fatalf("expected needs_input when both id and name given, got %q (%s)", out.Status, out.Message)
	}
}

// ---- Path A: by jobRunId ----

func TestByRunIDWithoutConfirmOnlyPreviews(t *testing.T) {
	// delete is nil -> the test fails if the handler deletes without confirm=true.
	server := newServer(t, handlers{status: jsonHandler(http.StatusOK, finishedRun())})
	defer server.Close()

	out := run(t, server, tools.Input{JobRunID: "r-1"})
	if out.Status != tools.StatusConfirmRequired {
		t.Fatalf("expected confirm_required without confirm, got %q (%s)", out.Status, out.Message)
	}
	if out.Run == nil {
		t.Fatal("expected the run summary to be returned for review")
	}
	if out.Run.Status != "FINISHED" {
		t.Errorf("expected status surfaced, got %+v", out.Run)
	}
	// Timestamps are surfaced as the API returned them (UTC, RFC 3339).
	wantTimes := map[string][2]string{
		"runDate":   {out.Run.RunDate, "2025-10-22T13:00:00Z"},
		"startTime": {out.Run.StartTime, "2025-10-22T13:50:05Z"},
		"endTime":   {out.Run.EndTime, "2025-10-22T13:54:42Z"},
	}
	for field, gotWant := range wantTimes {
		if gotWant[0] != gotWant[1] {
			t.Errorf("expected %s to be %q, got %q", field, gotWant[1], gotWant[0])
		}
	}
	if out.JobRunID != "r-1" || out.JobName != "sales.orders" {
		t.Errorf("expected run id and job name echoed, got %q / %q", out.JobRunID, out.JobName)
	}
	if out.JobDetailsLink == "" {
		t.Error("expected a job details link when jobName is known")
	}
}

func TestByRunIDConfirmDeletes(t *testing.T) {
	var deletedPath, deletedMethod string
	server := newServer(t, handlers{
		status: jsonHandler(http.StatusOK, finishedRun()),
		delete: func(w http.ResponseWriter, r *http.Request) {
			deletedPath, deletedMethod = r.URL.Path, r.Method
			w.WriteHeader(http.StatusNoContent) // 204, empty body
		},
	})
	defer server.Close()

	out := run(t, server, tools.Input{JobRunID: "r-1", Confirm: true})
	if out.Status != tools.StatusDeleted {
		t.Fatalf("expected deleted, got %q (%s)", out.Status, out.Message)
	}
	if deletedMethod != http.MethodDelete {
		t.Errorf("expected a DELETE request, got %q", deletedMethod)
	}
	if deletedPath != "/rest/dq/1.0/jobRuns/r-1" {
		t.Errorf("expected delete on the run's path, got %q", deletedPath)
	}
	if out.JobName != "sales.orders" || out.JobDetailsLink == "" {
		t.Errorf("expected job name and details link on success, got %q / %q", out.JobName, out.JobDetailsLink)
	}
}

func TestByRunIDInProgressRefused(t *testing.T) {
	// delete is nil -> the test fails if the handler tries to delete an in-progress run.
	server := newServer(t, handlers{
		status: jsonHandler(http.StatusOK, map[string]any{"jobRunId": "r-1", "jobName": "sales.orders", "status": "RUNNING"}),
	})
	defer server.Close()

	out := run(t, server, tools.Input{JobRunID: "r-1", Confirm: true})
	if out.Status != tools.StatusError {
		t.Fatalf("expected error for an in-progress run, got %q (%s)", out.Status, out.Message)
	}
	if out.RunState != "RUNNING" {
		t.Errorf("expected runState RUNNING surfaced, got %q", out.RunState)
	}
	if !strings.Contains(out.Guidance, "dq_cancel_job_run") {
		t.Errorf("expected the guidance to point at dq_cancel_job_run, got %q", out.Guidance)
	}
}

func TestByRunIDStatusNotFound(t *testing.T) {
	server := newServer(t, handlers{
		status: jsonHandler(http.StatusNotFound, map[string]any{"message": "not found"}),
	})
	defer server.Close()

	out := run(t, server, tools.Input{JobRunID: "missing", Confirm: true})
	if out.Status != tools.StatusError {
		t.Fatalf("expected error for a 404 status lookup, got %q (%s)", out.Status, out.Message)
	}
	if !strings.Contains(out.Message, "404") {
		t.Errorf("expected the 404 surfaced in the message, got %q", out.Message)
	}
}

// ---- Path B: by jobName ----

func TestByNameSingleRunNeedsConfirmation(t *testing.T) {
	// delete is nil -> a name must never delete directly, even with confirm=true.
	server := newServer(t, handlers{
		search: jsonHandler(http.StatusOK, map[string]any{"results": []map[string]any{finishedRun()}}),
	})
	defer server.Close()

	out := run(t, server, tools.Input{JobName: "sales.orders", Confirm: true})
	if out.Status != tools.StatusConfirmRequired {
		t.Fatalf("expected confirm_required for a single named run, got %q (%s)", out.Status, out.Message)
	}
	if out.JobRunID != "r-1" {
		t.Errorf("expected the resolved run id echoed, got %q", out.JobRunID)
	}
}

func TestByNameSearchQuery(t *testing.T) {
	var query url.Values
	server := newServer(t, handlers{
		search: func(w http.ResponseWriter, r *http.Request) {
			query = r.URL.Query()
			jsonHandler(http.StatusOK, map[string]any{"results": []map[string]any{}})(w, r)
		},
	})
	defer server.Close()

	run(t, server, tools.Input{JobName: "sales.orders"})

	if got := query.Get("nameMatchMode"); got != "CONTAINS" {
		t.Errorf("expected nameMatchMode=CONTAINS, got %q", got)
	}
	if got := query.Get("jobName"); got != "sales.orders" {
		t.Errorf("expected the job name filter, got %q", got)
	}
	want := map[string]bool{"FINISHED": true, "CANCELLED": true, "FAILED": true}
	got := query["status"]
	if len(got) != len(want) {
		t.Fatalf("expected %d status filters, got %v", len(want), got)
	}
	for _, s := range got {
		if !want[s] {
			t.Errorf("unexpected status filter %q (want only terminal states)", s)
		}
	}
}

func TestByNameNoRuns(t *testing.T) {
	// delete nil -> must not be reached when there are no deletable runs.
	server := newServer(t, handlers{
		search: jsonHandler(http.StatusOK, map[string]any{"results": []map[string]any{}}),
	})
	defer server.Close()

	out := run(t, server, tools.Input{JobName: "sales.orders", Confirm: true})
	if out.Status != tools.StatusError {
		t.Fatalf("expected error when no deletable runs, got %q (%s)", out.Status, out.Message)
	}
	if !strings.Contains(out.Guidance, "dq_cancel_job_run") {
		t.Errorf("expected the guidance to mention cancelling in-progress runs, got %q", out.Guidance)
	}
}

func TestByNameMultipleRunsNeedsSelection(t *testing.T) {
	// delete nil -> must not be reached; the tool should ask the caller to pick one.
	server := newServer(t, handlers{
		search: jsonHandler(http.StatusOK, map[string]any{"results": []map[string]any{
			{"jobRunId": "r-1", "jobName": "sales.orders", "status": "FINISHED"},
			{"jobRunId": "r-2", "jobName": "sales.orders", "status": "FAILED"},
		}}),
	})
	defer server.Close()

	out := run(t, server, tools.Input{JobName: "sales.orders", Confirm: true})
	if out.Status != tools.StatusNeedsInput {
		t.Fatalf("expected needs_input for multiple runs, got %q (%s)", out.Status, out.Message)
	}
	if len(out.CandidateRuns) != 2 {
		t.Fatalf("expected 2 candidate runs, got %d", len(out.CandidateRuns))
	}
	if out.CandidateRuns[0].JobRunID != "r-1" || out.CandidateRuns[1].JobRunID != "r-2" {
		t.Errorf("unexpected candidate ids: %+v", out.CandidateRuns)
	}
}

// ---- delete error mapping ----

func TestDeleteErrorMapping(t *testing.T) {
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
				status: jsonHandler(http.StatusOK, finishedRun()),
				delete: jsonHandler(tc.code, map[string]any{"message": "boom"}),
			})
			defer server.Close()

			out := run(t, server, tools.Input{JobRunID: "r-1", Confirm: true})
			if out.Status != tools.StatusError {
				t.Fatalf("expected error for HTTP %d, got %q (%s)", tc.code, out.Status, out.Message)
			}
			if !strings.Contains(out.Message, tc.wantInMsg) {
				t.Errorf("expected message to mention %q, got %q", tc.wantInMsg, out.Message)
			}
			if out.JobRunID != "r-1" {
				t.Errorf("expected jobRunId echoed on error, got %q", out.JobRunID)
			}
			if tc.code == http.StatusConflict && !strings.Contains(out.Guidance, "dq_cancel_job_run") {
				t.Errorf("expected the 409 guidance to point at dq_cancel_job_run, got %q", out.Guidance)
			}
		})
	}
}

func TestTransportError(t *testing.T) {
	server := newServer(t, handlers{
		status: jsonHandler(http.StatusOK, finishedRun()),
		delete: jsonHandler(http.StatusNoContent, nil),
	})
	server.Close() // closed before use -> transport failure

	out := run(t, server, tools.Input{JobRunID: "r-1", Confirm: true})
	if out.Status != tools.StatusError {
		t.Fatalf("expected error on transport failure, got %q (%s)", out.Status, out.Message)
	}
}
