package get_job_status_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	tools "github.com/collibra/chip/pkg/tools/get_job_status"
	"github.com/collibra/chip/pkg/tools/testutil"
	"github.com/google/uuid"
)

func TestGetCatalogJobStatus(t *testing.T) {
	jobID, _ := uuid.NewUUID()

	handler := http.NewServeMux()
	handler.HandleFunc("GET /rest/jobs/v1/jobs/"+jobID.String(), func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"` + jobID.String() + `","name":"Database Synchronisation of source","type":"DELTA_INGESTION","state":"COMPLETED","result":"SUCCESS","progressPercentage":100}`))
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{JobID: jobID.String()})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if output.State != "COMPLETED" || output.Result != "SUCCESS" {
		t.Fatalf("unexpected output: %+v", output)
	}
}

func TestGetCatalogJobStatus_InvalidJobID(t *testing.T) {
	client := testutil.NewClient(httptest.NewServer(http.NewServeMux()))
	_, err := tools.NewTool(client).Handler(t.Context(), tools.Input{JobID: "not-a-uuid"})
	if err == nil {
		t.Fatalf("expected an error for invalid jobId")
	}
}
