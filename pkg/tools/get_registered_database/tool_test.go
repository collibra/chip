package get_registered_database_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	tools "github.com/collibra/chip/pkg/tools/get_registered_database"
	"github.com/collibra/chip/pkg/tools/testutil"
	"github.com/google/uuid"
)

func TestGetRegisteredDatabase_Success(t *testing.T) {
	assetID, _ := uuid.NewUUID()
	databaseConnID, _ := uuid.NewUUID()
	edgeConnID, _ := uuid.NewUUID()
	edgeSiteID, _ := uuid.NewUUID()

	handler := http.NewServeMux()
	handler.Handle("GET /rest/catalogDatabase/v1/databases/"+assetID.String(),
		testutil.JsonHandlerOut(func(r *http.Request) (int, clients.Database) {
			return http.StatusOK, clients.Database{ID: assetID.String(), Name: "snowflake-prod", DatabaseConnectionID: databaseConnID.String()}
		}))
	handler.Handle("GET /rest/catalogDatabase/v1/databaseConnections/"+databaseConnID.String(),
		testutil.JsonHandlerOut(func(r *http.Request) (int, clients.DatabaseConnection) {
			return http.StatusOK, clients.DatabaseConnection{ID: databaseConnID.String(), EdgeConnectionID: edgeConnID.String()}
		}))
	handler.Handle("GET /edge/api/rest/v2/connections/"+edgeConnID.String(),
		testutil.JsonHandlerOut(func(r *http.Request) (int, clients.EdgeConnection) {
			return http.StatusOK, clients.EdgeConnection{Id: edgeConnID.String(), Name: "Snowflake JDBC", TypeId: "Generic", EdgeSiteId: edgeSiteID.String()}
		}))

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
	if output.EdgeConnection.Id != edgeConnID.String() {
		t.Fatalf("expected edge connection id %s, got %s", edgeConnID.String(), output.EdgeConnection.Id)
	}
	if output.EdgeConnection.EdgeSiteId != edgeSiteID.String() {
		t.Fatalf("expected edge site id %s, got %s", edgeSiteID.String(), output.EdgeConnection.EdgeSiteId)
	}
	if output.Database.Name != "snowflake-prod" {
		t.Fatalf("expected database name snowflake-prod, got %s", output.Database.Name)
	}
}

func TestGetRegisteredDatabase_InvalidAssetID(t *testing.T) {
	client := testutil.NewClient(httptest.NewServer(http.NewServeMux()))
	_, err := tools.NewTool(client).Handler(t.Context(), tools.Input{AssetID: "not-a-uuid"})
	if err == nil {
		t.Fatalf("expected an error for invalid assetId")
	}
}

func TestGetRegisteredDatabase_NotRegistered(t *testing.T) {
	assetID, _ := uuid.NewUUID()

	handler := http.NewServeMux()
	handler.HandleFunc("GET /rest/catalogDatabase/v1/databases/"+assetID.String(),
		func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "database not found", http.StatusNotFound)
		})

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{AssetID: assetID.String()})
	if err != nil {
		t.Fatalf("expected no error (failure reported via Output), got: %v", err)
	}
	if output.Success {
		t.Fatalf("expected failure for unregistered database")
	}
	if output.Error == "" {
		t.Fatalf("expected error message in output")
	}
}

func TestGetRegisteredDatabase_EdgeConnectionLookupFails(t *testing.T) {
	assetID, _ := uuid.NewUUID()
	databaseConnID, _ := uuid.NewUUID()
	edgeConnID, _ := uuid.NewUUID()

	handler := http.NewServeMux()
	handler.Handle("GET /rest/catalogDatabase/v1/databases/"+assetID.String(),
		testutil.JsonHandlerOut(func(r *http.Request) (int, clients.Database) {
			return http.StatusOK, clients.Database{ID: assetID.String(), DatabaseConnectionID: databaseConnID.String()}
		}))
	handler.Handle("GET /rest/catalogDatabase/v1/databaseConnections/"+databaseConnID.String(),
		testutil.JsonHandlerOut(func(r *http.Request) (int, clients.DatabaseConnection) {
			return http.StatusOK, clients.DatabaseConnection{ID: databaseConnID.String(), EdgeConnectionID: edgeConnID.String()}
		}))
	handler.HandleFunc("GET /edge/api/rest/v2/connections/"+edgeConnID.String(),
		func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "connection not found", http.StatusNotFound)
		})

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{AssetID: assetID.String()})
	if err != nil {
		t.Fatalf("expected no error (failure reported via Output), got: %v", err)
	}
	if output.Success {
		t.Fatalf("expected failure when Edge connection lookup fails")
	}
	if output.Database == nil {
		t.Fatalf("expected the resolved database to be returned alongside the error")
	}
}
