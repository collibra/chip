package configure_database_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/collibra/chip/pkg/clients"
	tools "github.com/collibra/chip/pkg/tools/configure_database"
	"github.com/collibra/chip/pkg/tools/testutil"
	"github.com/google/uuid"
)

func init() {
	tools.Sleep = func(time.Duration) {}
}

func TestConfigureDatabase_Success(t *testing.T) {
	edgeConnID, _ := uuid.NewUUID()
	communityID, _ := uuid.NewUUID()
	systemID, _ := uuid.NewUUID()
	ownerID, _ := uuid.NewUUID()
	dbConnID, _ := uuid.NewUUID()
	databaseID, _ := uuid.NewUUID()
	schemaConnID, _ := uuid.NewUUID()

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
	mux.HandleFunc("POST /rest/catalogDatabase/v1/schemaConnections/refresh", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("databaseConnectionId") != dbConnID.String() {
			t.Fatalf("unexpected databaseConnectionId: %s", r.URL.Query().Get("databaseConnectionId"))
		}
		w.WriteHeader(http.StatusAccepted)
	})
	mux.Handle("GET /rest/catalogDatabase/v1/schemaConnections", testutil.JsonHandlerOut(func(r *http.Request) (int, map[string]any) {
		return http.StatusOK, map[string]any{
			"results": []clients.SchemaConnection{
				{ID: schemaConnID.String(), Name: "public", DatabaseConnectionID: dbConnID.String()},
			},
		}
	}))
	mux.Handle("POST /rest/catalogDatabase/v1/schemaMetadataConfigurations/batch", testutil.JsonHandlerInOut(func(r *http.Request, in []clients.SchemaMetadataConfiguration) (int, []clients.SchemaMetadataConfiguration) {
		if len(in) != 1 || in[0].SchemaConnectionID != schemaConnID.String() {
			t.Fatalf("unexpected batch payload: %+v", in)
		}
		if len(in[0].SynchronizationRules) != 1 || in[0].SynchronizationRules[0].Include != "*" {
			t.Fatalf("unexpected synchronization rules: %+v", in[0].SynchronizationRules)
		}
		return http.StatusCreated, in
	}))

	server := httptest.NewServer(mux)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
		EdgeConnectionID: edgeConnID.String(),
		CommunityID:      communityID.String(),
		ParentSystemID:   systemID.String(),
		OwnerIDs:         []string{ownerID.String()},
		Include:          "*",
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
	if len(output.SchemaConnections) != 1 {
		t.Fatalf("expected 1 schema connection, got %d", len(output.SchemaConnections))
	}
}

func TestConfigureDatabase_MissingInclude(t *testing.T) {
	edgeConnID, _ := uuid.NewUUID()
	communityID, _ := uuid.NewUUID()
	systemID, _ := uuid.NewUUID()
	ownerID, _ := uuid.NewUUID()

	client := testutil.NewClient(httptest.NewServer(http.NewServeMux()))
	_, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
		EdgeConnectionID: edgeConnID.String(),
		CommunityID:      communityID.String(),
		ParentSystemID:   systemID.String(),
		OwnerIDs:         []string{ownerID.String()},
	})
	if err == nil {
		t.Fatalf("expected an error when include is omitted")
	}
}

func TestConfigureDatabase_MultipleSchemasRequireSchemaNames(t *testing.T) {
	edgeConnID, _ := uuid.NewUUID()
	communityID, _ := uuid.NewUUID()
	systemID, _ := uuid.NewUUID()
	ownerID, _ := uuid.NewUUID()
	dbConnID, _ := uuid.NewUUID()
	databaseID, _ := uuid.NewUUID()
	publicSchemaID, _ := uuid.NewUUID()
	privateSchemaID, _ := uuid.NewUUID()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /rest/catalogDatabase/v1/databaseConnections/refresh", func(w http.ResponseWriter, r *http.Request) {
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
		return http.StatusCreated, clients.Database{ID: databaseID.String(), Name: "source", DatabaseConnectionID: in.DatabaseConnectionID}
	}))
	mux.HandleFunc("POST /rest/catalogDatabase/v1/schemaConnections/refresh", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})
	mux.Handle("GET /rest/catalogDatabase/v1/schemaConnections", testutil.JsonHandlerOut(func(r *http.Request) (int, map[string]any) {
		return http.StatusOK, map[string]any{
			"results": []clients.SchemaConnection{
				{ID: publicSchemaID.String(), Name: "public", DatabaseConnectionID: dbConnID.String()},
				{ID: privateSchemaID.String(), Name: "private", DatabaseConnectionID: dbConnID.String()},
			},
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
		Include:          "*",
	})
	if err != nil {
		t.Fatalf("expected no error (failures reported via Output), got: %v", err)
	}
	if output.Success {
		t.Fatalf("expected failure when multiple schemas are discovered without schemaNames")
	}
	if output.Error == "" {
		t.Fatalf("expected an error message naming the discovered schemas")
	}
}

func TestConfigureDatabase_SchemaNamesSelectsSubset(t *testing.T) {
	edgeConnID, _ := uuid.NewUUID()
	communityID, _ := uuid.NewUUID()
	systemID, _ := uuid.NewUUID()
	ownerID, _ := uuid.NewUUID()
	dbConnID, _ := uuid.NewUUID()
	databaseID, _ := uuid.NewUUID()
	publicSchemaID, _ := uuid.NewUUID()
	privateSchemaID, _ := uuid.NewUUID()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /rest/catalogDatabase/v1/databaseConnections/refresh", func(w http.ResponseWriter, r *http.Request) {
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
		return http.StatusCreated, clients.Database{ID: databaseID.String(), Name: "source", DatabaseConnectionID: in.DatabaseConnectionID}
	}))
	mux.HandleFunc("POST /rest/catalogDatabase/v1/schemaConnections/refresh", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})
	mux.Handle("GET /rest/catalogDatabase/v1/schemaConnections", testutil.JsonHandlerOut(func(r *http.Request) (int, map[string]any) {
		return http.StatusOK, map[string]any{
			"results": []clients.SchemaConnection{
				{ID: publicSchemaID.String(), Name: "public", DatabaseConnectionID: dbConnID.String()},
				{ID: privateSchemaID.String(), Name: "private", DatabaseConnectionID: dbConnID.String()},
			},
		}
	}))
	mux.Handle("POST /rest/catalogDatabase/v1/schemaMetadataConfigurations/batch", testutil.JsonHandlerInOut(func(r *http.Request, in []clients.SchemaMetadataConfiguration) (int, []clients.SchemaMetadataConfiguration) {
		if len(in) != 1 || in[0].SchemaConnectionID != publicSchemaID.String() {
			t.Fatalf("unexpected batch payload: %+v", in)
		}
		return http.StatusCreated, in
	}))

	server := httptest.NewServer(mux)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
		EdgeConnectionID: edgeConnID.String(),
		CommunityID:      communityID.String(),
		ParentSystemID:   systemID.String(),
		OwnerIDs:         []string{ownerID.String()},
		SchemaNames:      []string{"public"},
		Include:          "*",
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !output.Success {
		t.Fatalf("expected success, got error: %s", output.Error)
	}
	if len(output.SchemaConnections) != 1 || output.SchemaConnections[0].Name != "public" {
		t.Fatalf("expected only the 'public' schema to be configured, got: %+v", output.SchemaConnections)
	}
}

func TestConfigureDatabase_NoDatabaseConnectionsDiscovered(t *testing.T) {
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
		Include:          "*",
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

func TestConfigureDatabase_InvalidInput(t *testing.T) {
	client := testutil.NewClient(httptest.NewServer(http.NewServeMux()))
	_, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
		EdgeConnectionID: "not-a-uuid",
	})
	if err == nil {
		t.Fatalf("expected an error for invalid edgeConnectionId")
	}
}
