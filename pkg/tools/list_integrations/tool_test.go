package list_integrations_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	tools "github.com/collibra/chip/pkg/tools/list_integrations"
	"github.com/collibra/chip/pkg/tools/testutil"
)

func TestListIntegrations(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/edge/api/rest/v2/capabilities", testutil.JsonHandlerOut(func(r *http.Request) (int, []clients.EdgeCapability) {
		return http.StatusOK, []clients.EdgeCapability{
			{Id: "00000000-0000-0000-0000-000000000001", Name: "UC / Sales", Type: &clients.EdgeCapabilityType{Id: "databricks-edge-capability"}},
			{Id: "00000000-0000-0000-0000-000000000002", Name: "UC / Finance", Type: &clients.EdgeCapabilityType{Id: "databricks-edge-capability"}},
			{Id: "00000000-0000-0000-0000-000000000003", Name: "Dataplex Finance", Type: &clients.EdgeCapabilityType{Id: "dataplex-synchronization"}},
			{Id: "00000000-0000-0000-0000-000000000004", Name: "jdbc-only", Type: &clients.EdgeCapabilityType{Id: "jdbc-profiling"}},
		}
	}))
	handler.Handle("/rest/catalog/1.0/genericIntegration/00000000-0000-0000-0000-000000000001/schedule", testutil.JsonHandlerOut(func(r *http.Request) (int, clients.GenericSchedule) {
		return http.StatusOK, clients.GenericSchedule{CronExpression: "0 2 * * *", CronTimeZone: "UTC", LastRunTimeStamp: 1000, NextRunDateLongValue: 9999999999000}
	}))
	handler.Handle("/rest/catalog/1.0/genericIntegration/00000000-0000-0000-0000-000000000002/schedule", testutil.JsonHandlerOut(func(r *http.Request) (int, clients.GenericSchedule) {
		return http.StatusOK, clients.GenericSchedule{CronExpression: "0 3 * * *", CronTimeZone: "UTC"}
	}))
	handler.Handle("/rest/catalog/1.0/genericIntegration/00000000-0000-0000-0000-000000000003/schedule", testutil.JsonHandlerOut(func(r *http.Request) (int, string) {
		return http.StatusNotFound, "no schedule"
	}))
	handler.Handle("/rest/catalog/1.0/genericIntegration/00000000-0000-0000-0000-000000000004/schedule", testutil.JsonHandlerOut(func(r *http.Request) (int, string) {
		return http.StatusNotFound, "no schedule"
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	output, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Error != "" {
		t.Fatalf("unexpected output error: %s", output.Error)
	}
	// all capabilities returned when no platform filter
	if output.Total != 4 {
		t.Fatalf("expected 4 integrations (all types), got %d", output.Total)
	}
}

func TestListIntegrationsFilterByPlatform(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/edge/api/rest/v2/capabilities", testutil.JsonHandlerOut(func(r *http.Request) (int, []clients.EdgeCapability) {
		return http.StatusOK, []clients.EdgeCapability{
			{Id: "00000000-0000-0000-0000-000000000001", Name: "UC / Sales", Type: &clients.EdgeCapabilityType{Id: "databricks-edge-capability"}},
			{Id: "00000000-0000-0000-0000-000000000002", Name: "Dataplex Finance", Type: &clients.EdgeCapabilityType{Id: "dataplex-synchronization"}},
		}
	}))
	handler.Handle("/rest/catalog/1.0/genericIntegration/00000000-0000-0000-0000-000000000001/schedule", testutil.JsonHandlerOut(func(r *http.Request) (int, clients.GenericSchedule) {
		return http.StatusOK, clients.GenericSchedule{CronExpression: "0 2 * * *"}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	output, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{Platform: "databricks"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Total != 1 {
		t.Fatalf("expected 1 databricks integration, got %d", output.Total)
	}
	if output.Integrations[0].Name != "UC / Sales" {
		t.Fatalf("expected UC / Sales, got %s", output.Integrations[0].Name)
	}
}

func TestListIntegrationsScheduleEnrichment(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/edge/api/rest/v2/capabilities", testutil.JsonHandlerOut(func(r *http.Request) (int, []clients.EdgeCapability) {
		return http.StatusOK, []clients.EdgeCapability{
			{Id: "00000000-0000-0000-0000-000000000001", Name: "UC / Sales", Type: &clients.EdgeCapabilityType{Id: "databricks-edge-capability"}},
		}
	}))
	handler.Handle("/rest/catalog/1.0/genericIntegration/00000000-0000-0000-0000-000000000001/schedule", testutil.JsonHandlerOut(func(r *http.Request) (int, clients.GenericSchedule) {
		return http.StatusOK, clients.GenericSchedule{
			CronExpression:      "0 2 * * *",
			CronTimeZone:        "UTC",
			LastRunTimeStamp:    1000,
			NextRunDateLongValue: 9999999999000,
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	output, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Total != 1 {
		t.Fatalf("expected 1 integration, got %d", output.Total)
	}
	integration := output.Integrations[0]
	if !integration.HasSchedule {
		t.Fatal("expected HasSchedule to be true")
	}
	if integration.CronExpression != "0 2 * * *" {
		t.Fatalf("expected cron 0 2 * * *, got %s", integration.CronExpression)
	}
	if integration.LastRun == "" {
		t.Fatal("expected LastRun to be set")
	}
	if integration.NextRun == "" {
		t.Fatal("expected NextRun to be set")
	}
}

func TestListIntegrationsNoScheduleFallsBackToJobs(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/edge/api/rest/v2/capabilities", testutil.JsonHandlerOut(func(r *http.Request) (int, []clients.EdgeCapability) {
		return http.StatusOK, []clients.EdgeCapability{
			{Id: "00000000-0000-0000-0000-000000000001", Name: "UC / Marketing", Type: &clients.EdgeCapabilityType{Id: "databricks-edge-capability"}},
		}
	}))
	handler.Handle("/rest/catalog/1.0/genericIntegration/00000000-0000-0000-0000-000000000001/schedule", testutil.JsonHandlerOut(func(r *http.Request) (int, string) {
		return http.StatusNotFound, "not found"
	}))
	handler.Handle("/rest/jobs/v1/jobs", testutil.JsonHandlerOut(func(r *http.Request) (int, clients.JobPagedResponse) {
		return http.StatusOK, clients.JobPagedResponse{
			Results: []clients.Job{
				{ID: "job-1", Name: "UC / Marketing", State: "COMPLETED", Result: "SUCCESS", EndDate: "2026-05-01T02:00:00Z"},
			},
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	output, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Total != 1 {
		t.Fatalf("expected 1 integration, got %d", output.Total)
	}
	integration := output.Integrations[0]
	if integration.HasSchedule {
		t.Fatal("expected HasSchedule to be false")
	}
	if integration.LastRun == "" {
		t.Fatal("expected LastRun to be populated from jobs API")
	}
	if integration.LastRunState != "COMPLETED" {
		t.Fatalf("expected LastRunState COMPLETED, got %s", integration.LastRunState)
	}
	if integration.LastRunResult != "SUCCESS" {
		t.Fatalf("expected LastRunResult SUCCESS, got %s", integration.LastRunResult)
	}
}

func TestListIntegrationsScheduleNoLastRunFallsBackToJobs(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/edge/api/rest/v2/capabilities", testutil.JsonHandlerOut(func(r *http.Request) (int, []clients.EdgeCapability) {
		return http.StatusOK, []clients.EdgeCapability{
			{Id: "00000000-0000-0000-0000-000000000001", Name: "UC / Sales", Type: &clients.EdgeCapabilityType{Id: "databricks-edge-capability"}},
		}
	}))
	// schedule exists but lastRunTimeStamp is 0 (never run)
	handler.Handle("/rest/catalog/1.0/genericIntegration/00000000-0000-0000-0000-000000000001/schedule", testutil.JsonHandlerOut(func(r *http.Request) (int, clients.GenericSchedule) {
		return http.StatusOK, clients.GenericSchedule{CronExpression: "0 2 * * *", CronTimeZone: "UTC", LastRunTimeStamp: 0}
	}))
	handler.Handle("/rest/jobs/v1/jobs", testutil.JsonHandlerOut(func(r *http.Request) (int, clients.JobPagedResponse) {
		return http.StatusOK, clients.JobPagedResponse{
			Results: []clients.Job{
				{ID: "job-1", State: "FAILED", Result: "FAILURE", EndDate: "2026-04-30T03:00:00Z"},
			},
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	output, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	integration := output.Integrations[0]
	if !integration.HasSchedule {
		t.Fatal("expected HasSchedule to be true")
	}
	if integration.LastRun == "" {
		t.Fatal("expected LastRun from jobs fallback")
	}
	if integration.LastRunState != "FAILED" {
		t.Fatalf("expected LastRunState FAILED, got %s", integration.LastRunState)
	}
}

func TestListIntegrationsAPIError(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/edge/api/rest/v2/capabilities", testutil.JsonHandlerOut(func(r *http.Request) (int, string) {
		return http.StatusInternalServerError, "error"
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	output, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{})
	if err != nil {
		t.Fatalf("unexpected go error: %v", err)
	}
	if output.Error == "" {
		t.Fatal("expected output error to be set")
	}
}
