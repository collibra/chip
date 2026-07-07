package clients_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/testutil"
	"github.com/google/uuid"
)

func TestGetJob(t *testing.T) {
	jobID, _ := uuid.NewUUID()

	handler := http.NewServeMux()
	handler.HandleFunc("GET /rest/jobs/v1/jobs/"+jobID.String(), func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"` + jobID.String() + `","name":"Database Synchronisation of source","type":"DELTA_INGESTION","state":"RUNNING","result":"NOT_SET","progressPercentage":42}`))
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	job, err := clients.GetJob(t.Context(), client, jobID.String())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if job.State != "RUNNING" || job.Type != "DELTA_INGESTION" || job.ProgressPercentage != 42 {
		t.Fatalf("unexpected job: %+v", job)
	}
}
