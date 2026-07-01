package run_dq_job_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/run_dq_job"
	"github.com/collibra/chip/pkg/tools/testutil"
)

type capture struct {
	path    string
	hadBody bool
	body    clients.RunDQJobRequest
}

// server boots an httptest server for the run-job endpoint. runCode overrides
// the success status; the captured request is recorded in rec for assertions.
func server(t *testing.T, runCode int, rec *capture) *http.Client {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /rest/dq/1.0/jobs/{jobName}/run", func(w http.ResponseWriter, r *http.Request) {
		rec.path = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		if len(strings.TrimSpace(string(raw))) > 0 {
			rec.hadBody = true
			if err := json.Unmarshal(raw, &rec.body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		code := runCode
		if code == 0 {
			code = http.StatusAccepted
		}
		if code != http.StatusAccepted {
			w.WriteHeader(code)
			_, _ = w.Write([]byte(`{"message":"boom"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(clients.RunDQJobResponse{JobRunID: "11111111-1111-1111-1111-111111111111"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return testutil.NewClient(srv)
}

func TestRunDQJob_HappyPath_NoBody(t *testing.T) {
	var rec capture
	c := server(t, http.StatusAccepted, &rec)

	out, err := run_dq_job.NewTool(c).Handler(t.Context(), run_dq_job.Input{JobName: "PUBLIC.SAMPLE_DATASET"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != run_dq_job.StatusSuccess {
		t.Fatalf("status = %q, want success (%s)", out.Status, out.Message)
	}
	if out.JobRunID == "" {
		t.Fatalf("expected a jobRunId")
	}
	if rec.hadBody {
		t.Fatalf("expected no request body when no run params supplied")
	}
	if rec.path != "/rest/dq/1.0/jobs/PUBLIC.SAMPLE_DATASET/run" {
		t.Fatalf("unexpected path: %s", rec.path)
	}
}

func TestRunDQJob_RunDateDefaultsToDateKind(t *testing.T) {
	var rec capture
	c := server(t, http.StatusAccepted, &rec)

	_, err := run_dq_job.NewTool(c).Handler(t.Context(), run_dq_job.Input{
		JobName: "DS",
		RunDate: "2026-06-28",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !rec.hadBody || rec.body.RunDate == nil {
		t.Fatalf("expected a runDate in the body")
	}
	if rec.body.RunDate.Kind != "DATE" || rec.body.RunDate.Value != "2026-06-28" {
		t.Fatalf("unexpected runDate: %+v", rec.body.RunDate)
	}
}

func TestRunDQJob_BackrunForwarded(t *testing.T) {
	var rec capture
	c := server(t, http.StatusAccepted, &rec)

	_, err := run_dq_job.NewTool(c).Handler(t.Context(), run_dq_job.Input{
		JobName: "DS",
		Backrun: &run_dq_job.BackrunInput{TimeBin: "DAY", BinValue: 10},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.body.Backrun == nil || rec.body.Backrun.TimeBin != "DAY" || rec.body.Backrun.BinValue != 10 {
		t.Fatalf("unexpected backrun: %+v", rec.body.Backrun)
	}
}

func TestRunDQJob_InvalidDateKind(t *testing.T) {
	var rec capture
	c := server(t, http.StatusAccepted, &rec)

	out, _ := run_dq_job.NewTool(c).Handler(t.Context(), run_dq_job.Input{
		JobName:  "DS",
		RunDate:  "2026-06-28",
		DateKind: "WEEK",
	})
	if out.Status != run_dq_job.StatusValidationError {
		t.Fatalf("status = %q, want validation_error", out.Status)
	}
}

func TestRunDQJob_MissingJobName(t *testing.T) {
	var rec capture
	c := server(t, http.StatusAccepted, &rec)

	out, _ := run_dq_job.NewTool(c).Handler(t.Context(), run_dq_job.Input{})
	if out.Status != run_dq_job.StatusValidationError {
		t.Fatalf("status = %q, want validation_error", out.Status)
	}
}

func TestRunDQJob_NotFoundSurfaces(t *testing.T) {
	var rec capture
	c := server(t, http.StatusNotFound, &rec)

	out, _ := run_dq_job.NewTool(c).Handler(t.Context(), run_dq_job.Input{JobName: "DS"})
	if out.Status != run_dq_job.StatusError {
		t.Fatalf("status = %q, want error", out.Status)
	}
}
