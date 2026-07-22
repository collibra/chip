package edge_get_job_status_history_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	tools "github.com/collibra/chip/pkg/tools/edge_get_job_status_history"
	"github.com/collibra/chip/pkg/tools/testutil"
)

func TestEdgeGetJobStatusHistory(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/edge/api/rest/v2/jobs/00000000-0000-0000-0000-000000000001/statusLogHistory", testutil.JsonHandlerOut(func(r *http.Request) (int, []clients.JobStatusLog) {
		return http.StatusOK, []clients.JobStatusLog{
			{Status: "SUCCEEDED", Message: "Done"},
			{Status: "RUNNING", Message: "In progress"},
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	output, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{
		Id: "00000000-0000-0000-0000-000000000001",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Error != "" {
		t.Fatalf("unexpected output error: %s", output.Error)
	}
	if len(output.History) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(output.History))
	}
	if output.History[0].Status != "SUCCEEDED" {
		t.Fatalf("expected SUCCEEDED first, got %s", output.History[0].Status)
	}
}

func TestEdgeGetJobStatusHistoryAPIError(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/edge/api/rest/v2/jobs/00000000-0000-0000-0000-000000000001/statusLogHistory", testutil.JsonHandlerOut(func(r *http.Request) (int, string) {
		return http.StatusNotFound, "not found"
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	output, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{
		Id: "00000000-0000-0000-0000-000000000001",
	})
	if err != nil {
		t.Fatalf("unexpected go error: %v", err)
	}
	if output.Error == "" {
		t.Fatal("expected output error")
	}
}

func TestEdgeGetJobStatusHistoryInvalidId(t *testing.T) {
	server := httptest.NewServer(http.NewServeMux())
	defer server.Close()

	_, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{Id: "bad"})
	if err == nil {
		t.Fatal("expected validation error")
	}
}
