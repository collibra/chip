package start_technical_lineage_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	tools "github.com/collibra/chip/pkg/tools/start_technical_lineage"
	"github.com/collibra/chip/pkg/tools/testutil"
	"github.com/google/uuid"
)

func TestStartTechnicalLineage_Success(t *testing.T) {
	assetID, _ := uuid.NewUUID()

	handler := http.NewServeMux()
	handler.HandleFunc("POST /rest/catalog/1.0/technicalLineage/harvester/"+assetID.String(),
		func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			if len(body) != 0 {
				t.Fatalf("expected empty request body, got: %s", body)
			}
			w.WriteHeader(http.StatusAccepted)
		})

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{AssetID: assetID.String()})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !output.Success {
		t.Fatalf("expected success, got error: %s", output.Error)
	}
}

func TestStartTechnicalLineage_InvalidAssetID(t *testing.T) {
	client := testutil.NewClient(httptest.NewServer(http.NewServeMux()))
	_, err := tools.NewTool(client).Handler(t.Context(), tools.Input{AssetID: "not-a-uuid"})
	if err == nil {
		t.Fatalf("expected an error for invalid assetId")
	}
}

func TestStartTechnicalLineage_ServerError(t *testing.T) {
	assetID, _ := uuid.NewUUID()

	handler := http.NewServeMux()
	handler.HandleFunc("POST /rest/catalog/1.0/technicalLineage/harvester/"+assetID.String(),
		func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "no technical lineage capability found for asset", http.StatusNotFound)
		})

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{AssetID: assetID.String()})
	if err != nil {
		t.Fatalf("expected no error (failure reported via Output), got: %v", err)
	}
	if output.Success {
		t.Fatalf("expected failure on server error")
	}
	if output.Error == "" {
		t.Fatalf("expected error message in output")
	}
}
