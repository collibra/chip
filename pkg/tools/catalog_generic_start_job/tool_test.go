package catalog_generic_start_job_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	tools "github.com/collibra/chip/pkg/tools/catalog_generic_start_job"
	"github.com/collibra/chip/pkg/tools/testutil"
)

func TestCatalogGenericStartJob(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/rest/catalog/1.0/genericIntegration/00000000-0000-0000-0000-000000000001/run", testutil.JsonHandlerOut(func(r *http.Request) (int, clients.GenericJob) {
		return http.StatusAccepted, clients.GenericJob{Id: "job-1", State: "RUNNING"}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	output, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{
		IngestibleId: "00000000-0000-0000-0000-000000000001",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !output.Success {
		t.Fatalf("expected success, got: %s", output.Error)
	}
	if output.Job == nil || output.Job.Id != "job-1" {
		t.Fatal("expected job id job-1")
	}
}

func TestCatalogGenericStartJobAPIError(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/rest/catalog/1.0/genericIntegration/00000000-0000-0000-0000-000000000001/run", testutil.JsonHandlerOut(func(r *http.Request) (int, string) {
		return http.StatusConflict, "job already running"
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	output, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{
		IngestibleId: "00000000-0000-0000-0000-000000000001",
	})
	if err != nil {
		t.Fatalf("unexpected go error: %v", err)
	}
	if output.Success {
		t.Fatal("expected failure")
	}
}

func TestCatalogGenericStartJobInvalidId(t *testing.T) {
	server := httptest.NewServer(http.NewServeMux())
	defer server.Close()

	_, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{IngestibleId: "bad"})
	if err == nil {
		t.Fatal("expected validation error")
	}
}
