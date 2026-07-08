package register_database_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/collibra/chip/pkg/clients"
	tools "github.com/collibra/chip/pkg/tools/register_database"
	"github.com/collibra/chip/pkg/tools/testutil"
	"github.com/google/uuid"
)

func init() {
	tools.Sleep = func(time.Duration) {}
}

func TestRegisterDatabase_Success(t *testing.T) {
	edgeConnID, _ := uuid.NewUUID()
	communityID, _ := uuid.NewUUID()
	systemID, _ := uuid.NewUUID()
	ownerID, _ := uuid.NewUUID()
	dbConnID, _ := uuid.NewUUID()
	databaseID, _ := uuid.NewUUID()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /rest/catalogDatabase/v1/databaseConnections/refresh", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("edgeConnectionId") != edgeConnID.String() {
			t.Fatalf("unexpected edgeConnectionId: %s", r.URL.Query().Get("edgeConnectionId"))
		}
		w.WriteHeader(http.StatusAccepted)
	})
	mux.Handle("GET /rest/catalogDatabase/v1/databaseConnections", testutil.JsonHandlerOut(func(r *http.Request) (int, map[string]any) {
		return http.StatusOK, map[string]any{
			"results": []clients.DatabaseConnection{
				{ID: dbConnID.String(), Name: "source", EdgeConnectionID: edgeConnID.String()},
			},
		}
	}))
	mux.Handle("POST /rest/catalogDatabase/v1/databases", testutil.JsonHandlerInOut(func(r *http.Request, in clients.AddDatabaseRequest) (int, clients.Database) {
		if in.DatabaseConnectionID != dbConnID.String() {
			t.Fatalf("unexpected databaseConnectionId: %s", in.DatabaseConnectionID)
		}
		return http.StatusCreated, clients.Database{
			ID:                   databaseID.String(),
			Name:                 "source",
			CommunityID:          in.CommunityID,
			OwnerIDs:             in.OwnerIDs,
			ParentSystemID:       in.ParentSystemID,
			DatabaseConnectionID: in.DatabaseConnectionID,
		}
	}))

	server := httptest.NewServer(mux)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
		EdgeConnectionID: edgeConnID.String(),
		CommunityID:      communityID.String(),
		ParentSystemID:   systemID.String(),
		OwnerIDs:         []string{ownerID.String()},
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !output.Success {
		t.Fatalf("expected success, got error: %s", output.Error)
	}
	if output.Database.ID != databaseID.String() {
		t.Fatalf("expected database id %s, got %s", databaseID.String(), output.Database.ID)
	}
	if output.Database.DatabaseConnectionID != dbConnID.String() {
		t.Fatalf("expected databaseConnectionId %s, got %s", dbConnID.String(), output.Database.DatabaseConnectionID)
	}
}

func TestRegisterDatabase_MultipleDatabasesRequireDatabaseName(t *testing.T) {
	edgeConnID, _ := uuid.NewUUID()
	communityID, _ := uuid.NewUUID()
	systemID, _ := uuid.NewUUID()
	ownerID, _ := uuid.NewUUID()
	dbConnID1, _ := uuid.NewUUID()
	dbConnID2, _ := uuid.NewUUID()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /rest/catalogDatabase/v1/databaseConnections/refresh", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})
	mux.Handle("GET /rest/catalogDatabase/v1/databaseConnections", testutil.JsonHandlerOut(func(r *http.Request) (int, map[string]any) {
		return http.StatusOK, map[string]any{
			"results": []clients.DatabaseConnection{
				{ID: dbConnID1.String(), Name: "source1", EdgeConnectionID: edgeConnID.String()},
				{ID: dbConnID2.String(), Name: "source2", EdgeConnectionID: edgeConnID.String()},
			},
		}
	}))
	databaseRegistered := false
	mux.Handle("POST /rest/catalogDatabase/v1/databases", testutil.JsonHandlerInOut(func(r *http.Request, in clients.AddDatabaseRequest) (int, clients.Database) {
		databaseRegistered = true
		return http.StatusCreated, clients.Database{DatabaseConnectionID: in.DatabaseConnectionID}
	}))

	server := httptest.NewServer(mux)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
		EdgeConnectionID: edgeConnID.String(),
		CommunityID:      communityID.String(),
		ParentSystemID:   systemID.String(),
		OwnerIDs:         []string{ownerID.String()},
	})
	if err != nil {
		t.Fatalf("expected no error (failures reported via Output), got: %v", err)
	}
	if output.Success {
		t.Fatalf("expected failure when multiple databases are discovered without databaseName")
	}
	if output.Error == "" {
		t.Fatalf("expected an error message naming the discovered databases")
	}
	if databaseRegistered {
		t.Fatalf("expected no Database asset to be registered when database selection is ambiguous")
	}
}

func TestRegisterDatabase_NoDatabaseConnectionsDiscovered(t *testing.T) {
	edgeConnID, _ := uuid.NewUUID()
	communityID, _ := uuid.NewUUID()
	systemID, _ := uuid.NewUUID()
	ownerID, _ := uuid.NewUUID()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /rest/catalogDatabase/v1/databaseConnections/refresh", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})
	mux.Handle("GET /rest/catalogDatabase/v1/databaseConnections", testutil.JsonHandlerOut(func(r *http.Request) (int, map[string]any) {
		return http.StatusOK, map[string]any{"results": []clients.DatabaseConnection{}}
	}))

	server := httptest.NewServer(mux)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
		EdgeConnectionID: edgeConnID.String(),
		CommunityID:      communityID.String(),
		ParentSystemID:   systemID.String(),
		OwnerIDs:         []string{ownerID.String()},
	})
	if err != nil {
		t.Fatalf("expected no error (failures reported via Output), got: %v", err)
	}
	if output.Success {
		t.Fatalf("expected failure when no database connections are discovered")
	}
	if output.Error == "" {
		t.Fatalf("expected an error message")
	}
}

func TestRegisterDatabase_InvalidInput(t *testing.T) {
	client := testutil.NewClient(httptest.NewServer(http.NewServeMux()))
	_, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
		EdgeConnectionID: "not-a-uuid",
	})
	if err == nil {
		t.Fatalf("expected an error for invalid edgeConnectionId")
	}
}

func TestRegisterDatabase_RequiresOwnerIDs(t *testing.T) {
	edgeConnID, _ := uuid.NewUUID()
	communityID, _ := uuid.NewUUID()
	systemID, _ := uuid.NewUUID()

	client := testutil.NewClient(httptest.NewServer(http.NewServeMux()))
	_, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
		EdgeConnectionID: edgeConnID.String(),
		CommunityID:      communityID.String(),
		ParentSystemID:   systemID.String(),
	})
	if err == nil {
		t.Fatalf("expected an error when ownerIds is empty")
	}
}
