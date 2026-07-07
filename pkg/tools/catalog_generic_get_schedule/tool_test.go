package catalog_generic_get_schedule_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	tools "github.com/collibra/chip/pkg/tools/catalog_generic_get_schedule"
	"github.com/collibra/chip/pkg/tools/testutil"
)

func TestCatalogGenericGetSchedule(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/rest/catalog/1.0/genericIntegration/00000000-0000-0000-0000-000000000001/schedule", testutil.JsonHandlerOut(func(r *http.Request) (int, clients.GenericSchedule) {
		return http.StatusOK, clients.GenericSchedule{
			Id:             1,
			CronExpression: "0 2 * * *",
			CronTimeZone:   "UTC",
		}
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
	if output.Schedule == nil || output.Schedule.CronExpression != "0 2 * * *" {
		t.Fatal("expected cron expression 0 2 * * *")
	}
}

func TestCatalogGenericGetScheduleNotFound(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/rest/catalog/1.0/genericIntegration/00000000-0000-0000-0000-000000000002/schedule", testutil.JsonHandlerOut(func(r *http.Request) (int, string) {
		return http.StatusNotFound, "not found"
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	output, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{
		IngestibleId: "00000000-0000-0000-0000-000000000002",
	})
	if err != nil {
		t.Fatalf("unexpected go error: %v", err)
	}
	if output.Found {
		t.Fatal("expected not found")
	}
}

func TestCatalogGenericGetScheduleInvalidId(t *testing.T) {
	server := httptest.NewServer(http.NewServeMux())
	defer server.Close()

	_, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{IngestibleId: "bad"})
	if err == nil {
		t.Fatal("expected validation error")
	}
}
