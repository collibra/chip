package get_dq_job_status_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/get_dq_job_status"
	"github.com/collibra/chip/pkg/tools/testutil"
)

func server(t *testing.T, code int, run clients.DQJobRun, path *string) *http.Client {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/dq/1.0/jobRuns/{jobRunId}", func(w http.ResponseWriter, r *http.Request) {
		if path != nil {
			*path = r.URL.Path
		}
		if code != http.StatusOK {
			w.WriteHeader(code)
			_, _ = w.Write([]byte(`{"message":"boom"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(run)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return testutil.NewClient(srv)
}

func TestGetDQJobStatus_HappyPath(t *testing.T) {
	var path string
	c := server(t, http.StatusOK, clients.DQJobRun{
		JobRunID: "run-1", JobName: "DS", Status: "FINISHED", Activity: "RESULTS",
	}, &path)
	out, err := get_dq_job_status.NewTool(c).Handler(t.Context(), get_dq_job_status.Input{JobRunID: "run-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != get_dq_job_status.StatusSuccess {
		t.Fatalf("status = %q, want success (%s)", out.Status, out.Message)
	}
	if out.RunStatus != "FINISHED" {
		t.Fatalf("runStatus = %q, want FINISHED", out.RunStatus)
	}
	if path != "/rest/dq/1.0/jobRuns/run-1" {
		t.Fatalf("unexpected path: %s", path)
	}
}

func TestGetDQJobStatus_MissingInput(t *testing.T) {
	c := server(t, http.StatusOK, clients.DQJobRun{}, nil)
	out, _ := get_dq_job_status.NewTool(c).Handler(t.Context(), get_dq_job_status.Input{})
	if out.Status != get_dq_job_status.StatusValidationError {
		t.Fatalf("status = %q, want validation_error", out.Status)
	}
}

func TestGetDQJobStatus_NotFound(t *testing.T) {
	c := server(t, http.StatusNotFound, clients.DQJobRun{}, nil)
	out, _ := get_dq_job_status.NewTool(c).Handler(t.Context(), get_dq_job_status.Input{JobRunID: "nope"})
	if out.Status != get_dq_job_status.StatusError {
		t.Fatalf("status = %q, want error", out.Status)
	}
}
