package get_dq_job_log_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/get_dq_job_log"
	"github.com/collibra/chip/pkg/tools/testutil"
)

func server(t *testing.T, code int, entries []clients.DQJobLogEntry, jobUUID *string) *http.Client {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/dq/internal/v1/job/logs", func(w http.ResponseWriter, r *http.Request) {
		if jobUUID != nil {
			*jobUUID = r.URL.Query().Get("jobUUID")
		}
		if code != http.StatusOK {
			w.WriteHeader(code)
			_, _ = w.Write([]byte(`{"message":"boom"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(entries)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return testutil.NewClient(srv)
}

func TestGetDQJobLog_HappyPath(t *testing.T) {
	var jobUUID string
	c := server(t, http.StatusOK, []clients.DQJobLogEntry{
		{Activity: "RULES", Stage: "run", LogDesc: "executed rules", PrettyStageTime: "2s"},
	}, &jobUUID)
	out, err := get_dq_job_log.NewTool(c).Handler(t.Context(), get_dq_job_log.Input{JobRunID: "run-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != get_dq_job_log.StatusSuccess {
		t.Fatalf("status = %q, want success (%s)", out.Status, out.Message)
	}
	if jobUUID != "run-1" {
		t.Fatalf("jobUUID query = %q, want run-1", jobUUID)
	}
	if len(out.Entries) != 1 || out.Entries[0].Description != "executed rules" {
		t.Fatalf("unexpected entries: %+v", out.Entries)
	}
}

func TestGetDQJobLog_MissingInput(t *testing.T) {
	c := server(t, http.StatusOK, nil, nil)
	out, _ := get_dq_job_log.NewTool(c).Handler(t.Context(), get_dq_job_log.Input{})
	if out.Status != get_dq_job_log.StatusValidationError {
		t.Fatalf("status = %q, want validation_error", out.Status)
	}
}
