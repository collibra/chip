package get_job_status_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	tools "github.com/collibra/chip/pkg/tools/get_job_status"
	"github.com/collibra/chip/pkg/tools/testutil"
	"github.com/google/uuid"
)

func TestGetJobStatus(t *testing.T) {
	jobID, _ := uuid.NewUUID()

	handler := http.NewServeMux()
	handler.Handle("GET /edge/api/rest/v2/jobs/"+jobID.String()+"/statusLog", testutil.JsonHandlerOut(func(r *http.Request) (int, clients.JobStatusLog) {
		return http.StatusOK, clients.JobStatusLog{
			JobID:               jobID.String(),
			Status:              "CAPABILITY_SUCCEEDED",
			Message:             "done",
			LastUpdatedDateTime: "2026-07-04T10:00:00.000Z",
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{JobID: jobID.String()})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if output.Status != "CAPABILITY_SUCCEEDED" {
		t.Fatalf("unexpected status: %s", output.Status)
	}
}

func TestGetJobStatus_InvalidJobID(t *testing.T) {
	client := testutil.NewClient(httptest.NewServer(http.NewServeMux()))
	_, err := tools.NewTool(client).Handler(t.Context(), tools.Input{JobID: "not-a-uuid"})
	if err == nil {
		t.Fatalf("expected an error for invalid jobId")
	}
}
