package search_dq_job_runs_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/tools/search_dq_job_runs"
	"github.com/collibra/chip/pkg/tools/testutil"
)

func server(t *testing.T, code int, body map[string]any, captured *map[string][]string) *http.Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/dq/1.0/jobRuns", func(w http.ResponseWriter, r *http.Request) {
		if captured != nil {
			*captured = map[string][]string(r.URL.Query())
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		if body != nil {
			_ = json.NewEncoder(w).Encode(body)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return testutil.NewClient(srv)
}

func TestSearchDqJobRuns_HappyPath(t *testing.T) {
	var query map[string][]string
	c := server(t, http.StatusOK, map[string]any{
		"results": []map[string]any{
			{
				"jobRunId":  "run-1",
				"jobName":   "public.nyse",
				"status":    "FAILED",
				"startTime": "2025-10-22T04:00:00Z",
			},
		},
		"total":  1,
		"offset": 0,
		"limit":  25,
	}, &query)

	out, err := search_dq_job_runs.NewTool(c).Handler(t.Context(), search_dq_job_runs.Input{JobName: "nyse", Status: "FAILED"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != search_dq_job_runs.StatusSuccess {
		t.Fatalf("status = %q, want success (%s)", out.Status, out.Message)
	}
	if len(out.Runs) != 1 || out.Runs[0].JobRunID != "run-1" || out.Runs[0].Status != "FAILED" {
		t.Fatalf("unexpected runs: %+v", out.Runs)
	}
	if out.Total != 1 || out.Page != 1 || out.PageSize != 25 {
		t.Fatalf("unexpected pagination: %+v", out)
	}
	if query["jobName"][0] != "nyse" || query["nameMatchMode"][0] != "CONTAINS" {
		t.Fatalf("unexpected query params: %+v", query)
	}
	if query["status"][0] != "FAILED" {
		t.Fatalf("unexpected status filter: %+v", query)
	}
}

func TestSearchDqJobRuns_PageMapsToOffset(t *testing.T) {
	var query map[string][]string
	c := server(t, http.StatusOK, map[string]any{"results": []map[string]any{}, "total": 0, "offset": 20, "limit": 10}, &query)

	_, err := search_dq_job_runs.NewTool(c).Handler(t.Context(), search_dq_job_runs.Input{Page: 3, PageSize: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if query["offset"][0] != "20" || query["limit"][0] != "10" {
		t.Fatalf("unexpected pagination params: %+v", query)
	}
}

func TestSearchDqJobRuns_InvalidPageSize(t *testing.T) {
	c := server(t, http.StatusOK, nil, nil)
	out, err := search_dq_job_runs.NewTool(c).Handler(t.Context(), search_dq_job_runs.Input{PageSize: 1000})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != search_dq_job_runs.StatusValidationError {
		t.Fatalf("status = %q, want validation_error", out.Status)
	}
}

func TestSearchDqJobRuns_DownstreamErrorSurfaces(t *testing.T) {
	c := server(t, http.StatusForbidden, map[string]any{"message": "nope"}, nil)
	out, err := search_dq_job_runs.NewTool(c).Handler(t.Context(), search_dq_job_runs.Input{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != search_dq_job_runs.StatusError {
		t.Fatalf("status = %q, want error", out.Status)
	}
}
