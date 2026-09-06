package search_dq_jobs_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/tools/search_dq_jobs"
	"github.com/collibra/chip/pkg/tools/testutil"
)

func server(t *testing.T, code int, body map[string]any, captured *map[string][]string) *http.Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/dq/1.0/jobs", func(w http.ResponseWriter, r *http.Request) {
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

func TestSearchDqJobs_HappyPath(t *testing.T) {
	var query map[string][]string
	c := server(t, http.StatusOK, map[string]any{
		"results": []map[string]any{
			{
				"jobName": "public.nyse",
				"jobType": "PUSHDOWN",
				"dataLocation": map[string]any{
					"edgeSiteName": "edge-1",
					"schemaName":   "public",
					"tableName":    "nyse",
				},
			},
		},
		"total":  1,
		"offset": 0,
		"limit":  25,
	}, &query)

	out, err := search_dq_jobs.NewTool(c).Handler(t.Context(), search_dq_jobs.Input{Schema: "public", JobType: "PUSHDOWN"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != search_dq_jobs.StatusSuccess {
		t.Fatalf("status = %q, want success (%s)", out.Status, out.Message)
	}
	if len(out.Jobs) != 1 || out.Jobs[0].JobName != "public.nyse" {
		t.Fatalf("unexpected jobs: %+v", out.Jobs)
	}
	if out.Jobs[0].SchemaName != "public" || out.Jobs[0].EdgeSiteName != "edge-1" {
		t.Fatalf("unexpected job detail: %+v", out.Jobs[0])
	}
	if out.Total != 1 || out.Page != 1 || out.PageSize != 25 {
		t.Fatalf("unexpected pagination: %+v", out)
	}
	if query["schemaName"][0] != "public" || query["jobType"][0] != "PUSHDOWN" {
		t.Fatalf("unexpected query params: %+v", query)
	}
	if query["offset"][0] != "0" || query["limit"][0] != "25" {
		t.Fatalf("unexpected pagination params: %+v", query)
	}
}

func TestSearchDqJobs_PageMapsToOffset(t *testing.T) {
	var query map[string][]string
	c := server(t, http.StatusOK, map[string]any{"results": []map[string]any{}, "total": 0, "offset": 20, "limit": 10}, &query)

	_, err := search_dq_jobs.NewTool(c).Handler(t.Context(), search_dq_jobs.Input{Page: 3, PageSize: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if query["offset"][0] != "20" || query["limit"][0] != "10" {
		t.Fatalf("unexpected pagination params: %+v", query)
	}
}

func TestSearchDqJobs_InvalidPageSize(t *testing.T) {
	c := server(t, http.StatusOK, nil, nil)
	out, err := search_dq_jobs.NewTool(c).Handler(t.Context(), search_dq_jobs.Input{PageSize: 1000})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != search_dq_jobs.StatusValidationError {
		t.Fatalf("status = %q, want validation_error", out.Status)
	}
}

func TestSearchDqJobs_DownstreamErrorSurfaces(t *testing.T) {
	c := server(t, http.StatusForbidden, map[string]any{"message": "nope"}, nil)
	out, err := search_dq_jobs.NewTool(c).Handler(t.Context(), search_dq_jobs.Input{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != search_dq_jobs.StatusError {
		t.Fatalf("status = %q, want error", out.Status)
	}
}
