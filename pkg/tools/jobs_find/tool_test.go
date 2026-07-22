package jobs_find_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	tools "github.com/collibra/chip/pkg/tools/jobs_find"
	"github.com/collibra/chip/pkg/tools/testutil"
)

func TestJobsFind(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/rest/jobs/v1/jobs", testutil.JsonHandlerOut(func(r *http.Request) (int, clients.JobPagedResponse) {
		return http.StatusOK, clients.JobPagedResponse{
			Results: []clients.Job{
				{ID: "job-1", Name: "test-job", State: "COMPLETED"},
			},
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	output, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{
		Name: "test-job",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Error != "" {
		t.Fatalf("expected no error, got: %s", output.Error)
	}
	if output.Count != 1 || output.Jobs[0].ID != "job-1" {
		t.Fatal("expected one job with id job-1")
	}
}

func TestJobsFindAPIError(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/rest/jobs/v1/jobs", testutil.JsonHandlerOut(func(r *http.Request) (int, string) {
		return http.StatusInternalServerError, "internal error"
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	output, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{})
	if err != nil {
		t.Fatalf("unexpected go error: %v", err)
	}
	if output.Error == "" {
		t.Fatal("expected error message in output")
	}
}

func TestJobsFindNoFilters(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/rest/jobs/v1/jobs", testutil.JsonHandlerOut(func(r *http.Request) (int, clients.JobPagedResponse) {
		return http.StatusOK, clients.JobPagedResponse{Results: []clients.Job{}}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	output, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Count != 0 {
		t.Fatalf("expected 0 jobs, got %d", output.Count)
	}
}
