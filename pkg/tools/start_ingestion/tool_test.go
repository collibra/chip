package start_ingestion_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	tools "github.com/collibra/chip/pkg/tools/start_ingestion"
	"github.com/collibra/chip/pkg/tools/testutil"
	"github.com/google/uuid"
)

func TestStartIngestion_Success(t *testing.T) {
	databaseID, _ := uuid.NewUUID()
	jobID, _ := uuid.NewUUID()

	handler := http.NewServeMux()
	handler.Handle("POST /rest/catalogDatabase/v1/databases/"+databaseID.String()+"/synchronizeMetadata",
		testutil.JsonHandlerInOut(func(r *http.Request, in clients.DatabaseMetadataSynchronizationRequest) (int, clients.Job) {
			if len(in.SchemaConnectionIDs) != 0 {
				t.Fatalf("expected no schemaConnectionIds, got: %v", in.SchemaConnectionIDs)
			}
			return http.StatusAccepted, clients.Job{ID: jobID.String(), State: "RUNNING"}
		}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
		DatabaseID: databaseID.String(),
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !output.Success {
		t.Fatalf("expected success, got error: %s", output.Error)
	}
	if output.Job.ID != jobID.String() {
		t.Fatalf("expected job id %s, got %s", jobID.String(), output.Job.ID)
	}
}

func TestStartIngestion_WithSchemaConnectionIDs(t *testing.T) {
	databaseID, _ := uuid.NewUUID()
	schemaConnID, _ := uuid.NewUUID()
	jobID, _ := uuid.NewUUID()

	handler := http.NewServeMux()
	handler.Handle("POST /rest/catalogDatabase/v1/databases/"+databaseID.String()+"/synchronizeMetadata",
		testutil.JsonHandlerInOut(func(r *http.Request, in clients.DatabaseMetadataSynchronizationRequest) (int, clients.Job) {
			if len(in.SchemaConnectionIDs) != 1 || in.SchemaConnectionIDs[0] != schemaConnID.String() {
				t.Fatalf("unexpected schemaConnectionIds: %v", in.SchemaConnectionIDs)
			}
			return http.StatusAccepted, clients.Job{ID: jobID.String(), State: "RUNNING"}
		}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
		DatabaseID:          databaseID.String(),
		SchemaConnectionIDs: []string{schemaConnID.String()},
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !output.Success {
		t.Fatalf("expected success, got error: %s", output.Error)
	}
}

func TestStartIngestion_InvalidDatabaseID(t *testing.T) {
	client := testutil.NewClient(httptest.NewServer(http.NewServeMux()))
	_, err := tools.NewTool(client).Handler(t.Context(), tools.Input{DatabaseID: "not-a-uuid"})
	if err == nil {
		t.Fatalf("expected an error for invalid databaseId")
	}
}

func TestStartIngestion_Conflict(t *testing.T) {
	databaseID, _ := uuid.NewUUID()

	handler := http.NewServeMux()
	handler.HandleFunc("POST /rest/catalogDatabase/v1/databases/"+databaseID.String()+"/synchronizeMetadata",
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusConflict)
		})

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{DatabaseID: databaseID.String()})
	if err != nil {
		t.Fatalf("expected no error (failure reported via Output), got: %v", err)
	}
	if output.Success {
		t.Fatalf("expected failure on conflict")
	}
}
