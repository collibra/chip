package delete_dq_job_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tools "github.com/collibra/chip/pkg/tools/delete_dq_job"
	"github.com/collibra/chip/pkg/tools/testutil"
)

// handlers configures the mocked job endpoints. A nil handler means "must not be called" — the mux
// fails the test if that endpoint is hit, which is how the tests assert that the DELETE is NOT
// reached without confirm=true.
type handlers struct {
	get    func(w http.ResponseWriter, r *http.Request) // GET    /rest/dq/1.0/jobs/{jobName}
	delete func(w http.ResponseWriter, r *http.Request) // DELETE /rest/dq/1.0/jobs/{jobName}
}

func newServer(t *testing.T, h handlers) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/dq/1.0/jobs/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			if h.delete == nil {
				t.Errorf("unexpected delete call: %s %s", r.Method, r.URL)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			h.delete(w, r)
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

// dqJob is the canonical job-definition payload, shaped like the public API's JobDefinition.
func dqJob() map[string]any {
	return map[string]any{
		"jobName": "sales.orders",
		"jobType": "PUSHDOWN",
		"dataLocation": map[string]any{
			"edgeSiteName":       "edge-eu",
			"edgeConnectionName": "snowflake-prod",
			"dataSourceName":     "SALES_DB",
			"schemaName":         "sales",
			"tableName":          "orders",
		},
		"sourceQuery":        "SELECT * FROM sales.orders",
		"runDate":            map[string]any{"kind": "DATE", "value": "2025-10-22"},
		"schedulingSettings": map[string]any{"schedulerMode": "DAILY", "isActive": true},
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

func TestMissingJobNameNeedsInput(t *testing.T) {
	server := newServer(t, handlers{}) // no endpoint should be hit
	defer server.Close()

	for _, name := range []string{"", "   "} {
		out := run(t, server, tools.Input{JobName: name, Confirm: true})
		if out.Status != tools.StatusNeedsInput {
			t.Fatalf("expected needs_input for jobName %q, got %q (%s)", name, out.Status, out.Message)
		}
	}
}

func TestInvalidJobNameNeedsInput(t *testing.T) {
	server := newServer(t, handlers{}) // rejected before any API call
	defer server.Close()

	out := run(t, server, tools.Input{JobName: "sales orders!", Confirm: true})
	if out.Status != tools.StatusNeedsInput {
		t.Fatalf("expected needs_input for an invalid job name, got %q (%s)", out.Status, out.Message)
	}
}

// ---- preview / confirm ----

func TestWithoutConfirmOnlyPreviews(t *testing.T) {
	// delete is nil -> the test fails if the handler deletes without confirm=true.
	server := newServer(t, handlers{get: jsonHandler(http.StatusOK, dqJob())})
	defer server.Close()

	out := run(t, server, tools.Input{JobName: "sales.orders"})
	if out.Status != tools.StatusConfirmRequired {
		t.Fatalf("expected confirm_required without confirm, got %q (%s)", out.Status, out.Message)
	}
	if out.Job == nil {
		t.Fatal("expected the job summary to be returned for review")
	}
	want := tools.JobSummary{
		JobName:         "sales.orders",
		JobType:         "PUSHDOWN",
		EdgeSiteName:    "edge-eu",
		ConnectionName:  "snowflake-prod",
		SchemaName:      "sales",
		TableName:       "orders",
		SourceQuery:     "SELECT * FROM sales.orders",
		ScheduleEnabled: true,
		ScheduleMode:    "DAILY",
		RunDate:         "2025-10-22",
	}
	if *out.Job != want {
		t.Errorf("unexpected job summary:\n got %+v\nwant %+v", *out.Job, want)
	}
	if out.JobName != "sales.orders" {
		t.Errorf("expected job name echoed, got %q", out.JobName)
	}
	if out.JobDetailsLink == "" {
		t.Error("expected a job details link")
	}
	if !strings.Contains(out.Guidance, "confirm=true") {
		t.Errorf("expected the guidance to explain the confirm step, got %q", out.Guidance)
	}
}

func TestConfirmDeletes(t *testing.T) {
	var deletedPath, deletedMethod string
	server := newServer(t, handlers{
		get: jsonHandler(http.StatusOK, dqJob()),
		delete: func(w http.ResponseWriter, r *http.Request) {
			deletedPath, deletedMethod = r.URL.Path, r.Method
			w.WriteHeader(http.StatusNoContent) // 204, empty body
		},
	})
	defer server.Close()

	out := run(t, server, tools.Input{JobName: "sales.orders", Confirm: true})
	if out.Status != tools.StatusDeleted {
		t.Fatalf("expected deleted, got %q (%s)", out.Status, out.Message)
	}
	if deletedMethod != http.MethodDelete {
		t.Errorf("expected a DELETE request, got %q", deletedMethod)
	}
	if deletedPath != "/rest/dq/1.0/jobs/sales.orders" {
		t.Errorf("expected delete on the job's path, got %q", deletedPath)
	}
	if out.JobName != "sales.orders" {
		t.Errorf("expected job name echoed on success, got %q", out.JobName)
	}
}

func TestJobNameIsPathEscaped(t *testing.T) {
	var gotPath string
	server := newServer(t, handlers{
		get: func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.EscapedPath()
			jsonHandler(http.StatusOK, dqJob())(w, r)
		},
	})
	defer server.Close()

	// '.' and '-' are legal in a job name and must survive escaping unchanged.
	run(t, server, tools.Input{JobName: "sales.orders-v2"})
	if gotPath != "/rest/dq/1.0/jobs/sales.orders-v2" {
		t.Errorf("unexpected escaped path: %q", gotPath)
	}
}

// ---- error mapping ----

func TestLookupErrorMapping(t *testing.T) {
	cases := []struct {
		code      int
		wantInMsg string
	}{
		{http.StatusBadRequest, "400"},
		{http.StatusUnauthorized, "401"},
		{http.StatusForbidden, "403"},
		{http.StatusNotFound, "404"},
		{http.StatusInternalServerError, "500"},
	}
	for _, tc := range cases {
		t.Run(tc.wantInMsg, func(t *testing.T) {
			// delete is nil -> a failed lookup must never reach the DELETE.
			server := newServer(t, handlers{get: jsonHandler(tc.code, map[string]any{"message": "boom"})})
			defer server.Close()

			out := run(t, server, tools.Input{JobName: "sales.orders", Confirm: true})
			if out.Status != tools.StatusError {
				t.Fatalf("expected error for HTTP %d, got %q (%s)", tc.code, out.Status, out.Message)
			}
			if !strings.Contains(out.Message, tc.wantInMsg) {
				t.Errorf("expected message to mention %q, got %q", tc.wantInMsg, out.Message)
			}
			if out.JobName != "sales.orders" {
				t.Errorf("expected jobName echoed on error, got %q", out.JobName)
			}
		})
	}
}

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
				get:    jsonHandler(http.StatusOK, dqJob()),
				delete: jsonHandler(tc.code, map[string]any{"message": "boom"}),
			})
			defer server.Close()

			out := run(t, server, tools.Input{JobName: "sales.orders", Confirm: true})
			if out.Status != tools.StatusError {
				t.Fatalf("expected error for HTTP %d, got %q (%s)", tc.code, out.Status, out.Message)
			}
			if !strings.Contains(out.Message, tc.wantInMsg) {
				t.Errorf("expected message to mention %q, got %q", tc.wantInMsg, out.Message)
			}
			if tc.code == http.StatusConflict && !strings.Contains(out.Guidance, "dq_cancel_job_run") {
				t.Errorf("expected the 409 guidance to point at dq_cancel_job_run, got %q", out.Guidance)
			}
		})
	}
}

func TestTransportError(t *testing.T) {
	server := newServer(t, handlers{
		get:    jsonHandler(http.StatusOK, dqJob()),
		delete: jsonHandler(http.StatusNoContent, nil),
	})
	server.Close() // closed before use -> transport failure

	out := run(t, server, tools.Input{JobName: "sales.orders", Confirm: true})
	if out.Status != tools.StatusError {
		t.Fatalf("expected error on transport failure, got %q (%s)", out.Status, out.Message)
	}
}
