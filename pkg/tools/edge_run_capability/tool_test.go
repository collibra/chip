package edge_run_capability_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	tools "github.com/collibra/chip/pkg/tools/edge_run_capability"
	"github.com/collibra/chip/pkg/tools/testutil"
)

func TestEdgeRunCapability(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/edge/api/rest/v2/capabilities/00000000-0000-0000-0000-000000000001/run", testutil.JsonHandlerOut(func(r *http.Request) (int, string) {
		return http.StatusCreated, "00000000-0000-0000-0000-000000000099"
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
	if output.JobId != "00000000-0000-0000-0000-000000000099" {
		t.Fatalf("expected job id 00000000-0000-0000-0000-000000000099, got: %s", output.JobId)
	}
}

func TestEdgeRunCapabilityAPIError(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/edge/api/rest/v2/capabilities/00000000-0000-0000-0000-000000000001/run", testutil.JsonHandlerOut(func(r *http.Request) (int, string) {
		return http.StatusNotFound, "capability not found"
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
	if output.Error == "" {
		t.Fatal("expected error message")
	}
}

func TestEdgeRunCapabilityInvalidId(t *testing.T) {
	server := httptest.NewServer(http.NewServeMux())
	defer server.Close()

	_, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{Id: "bad"})
	if err == nil {
		t.Fatal("expected validation error")
	}
}
