package catalog_generic_delete_schedule_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	tools "github.com/collibra/chip/pkg/tools/catalog_generic_delete_schedule"
	"github.com/collibra/chip/pkg/tools/testutil"
)

func TestCatalogGenericDeleteSchedule(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/rest/catalog/1.0/genericIntegration/00000000-0000-0000-0000-000000000001/schedule", testutil.JsonHandlerOut(func(r *http.Request) (int, map[string]any) {
		return http.StatusNoContent, nil
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
}

func TestCatalogGenericDeleteScheduleAPIError(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/rest/catalog/1.0/genericIntegration/00000000-0000-0000-0000-000000000001/schedule", testutil.JsonHandlerOut(func(r *http.Request) (int, string) {
		return http.StatusNotFound, "not found"
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

func TestCatalogGenericDeleteScheduleInvalidId(t *testing.T) {
	server := httptest.NewServer(http.NewServeMux())
	defer server.Close()

	_, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{IngestibleId: "bad"})
	if err == nil {
		t.Fatal("expected validation error")
	}
}
