package catalog_generic_get_all_schedules_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	tools "github.com/collibra/chip/pkg/tools/catalog_generic_get_all_schedules"
	"github.com/collibra/chip/pkg/tools/testutil"
)

func TestCatalogGenericGetAllSchedules(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/rest/catalog/1.0/genericIntegration/00000000-0000-0000-0000-000000000001/schedules", testutil.JsonHandlerOut(func(r *http.Request) (int, clients.GenericSchedule) {
		return http.StatusOK, clients.GenericSchedule{Id: 1, CronExpression: "0 2 * * *"}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	output, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{
		IngestibleId: "00000000-0000-0000-0000-000000000001",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !output.Found {
		t.Fatalf("expected found, got error: %s", output.Error)
	}
	if output.Schedule == nil || output.Schedule.Id != 1 {
		t.Fatal("expected schedule id 1")
	}
}

func TestCatalogGenericGetAllSchedulesAPIError(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/rest/catalog/1.0/genericIntegration/00000000-0000-0000-0000-000000000001/schedules", testutil.JsonHandlerOut(func(r *http.Request) (int, string) {
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
	if output.Found {
		t.Fatal("expected not found")
	}
}

func TestCatalogGenericGetAllSchedulesInvalidId(t *testing.T) {
	server := httptest.NewServer(http.NewServeMux())
	defer server.Close()

	_, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{IngestibleId: "bad"})
	if err == nil {
		t.Fatal("expected validation error")
	}
}
