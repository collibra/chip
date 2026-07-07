package edge_cancel_job_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	tools "github.com/collibra/chip/pkg/tools/edge_cancel_job"
	"github.com/collibra/chip/pkg/tools/testutil"
)

func TestEdgeCancelJob(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/edge/api/rest/v2/jobs/00000000-0000-0000-0000-000000000001/cancel", testutil.JsonHandlerOut(func(r *http.Request) (int, map[string]any) {
		return http.StatusOK, map[string]any{}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	output, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{
		Id: "00000000-0000-0000-0000-000000000001",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !output.Success {
		t.Fatalf("expected success, got: %s", output.Error)
	}
}

func TestEdgeCancelJobAPIError(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/edge/api/rest/v2/jobs/00000000-0000-0000-0000-000000000001/cancel", testutil.JsonHandlerOut(func(r *http.Request) (int, string) {
		return http.StatusNotFound, "job not found"
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	output, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{
		Id: "00000000-0000-0000-0000-000000000001",
	})
	if err != nil {
		t.Fatalf("unexpected go error: %v", err)
	}
	if output.Success {
		t.Fatal("expected failure")
	}
}

func TestEdgeCancelJobInvalidId(t *testing.T) {
	server := httptest.NewServer(http.NewServeMux())
	defer server.Close()

	_, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{Id: "bad"})
	if err == nil {
		t.Fatal("expected validation error")
	}
}
