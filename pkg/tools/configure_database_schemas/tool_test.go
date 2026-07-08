package configure_database_schemas_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/collibra/chip/pkg/clients"
	tools "github.com/collibra/chip/pkg/tools/configure_database_schemas"
	"github.com/collibra/chip/pkg/tools/testutil"
	"github.com/google/uuid"
)

func init() {
	tools.Sleep = func(time.Duration) {}
}

func TestConfigureDatabase_Success(t *testing.T) {
	dbConnID, _ := uuid.NewUUID()
	schemaConnID, _ := uuid.NewUUID()

	mux := http.NewServeMux()
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
		DatabaseConnectionID: dbConnID.String(),
		Include:              "*",
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !output.Success {
		t.Fatalf("expected success, got error: %s", output.Error)
	}
	if len(output.SchemaConnections) != 1 {
		t.Fatalf("expected 1 schema connection, got %d", len(output.SchemaConnections))
	}
}

func TestConfigureDatabase_MissingInclude(t *testing.T) {
	dbConnID, _ := uuid.NewUUID()

	client := testutil.NewClient(httptest.NewServer(http.NewServeMux()))
	_, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
		DatabaseConnectionID: dbConnID.String(),
	})
	if err == nil {
		t.Fatalf("expected an error when include is omitted")
	}
}

func TestConfigureDatabase_MultipleSchemasRequireSchemaNames(t *testing.T) {
	dbConnID, _ := uuid.NewUUID()
	publicSchemaID, _ := uuid.NewUUID()
	privateSchemaID, _ := uuid.NewUUID()

	mux := http.NewServeMux()
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
	batchCalled := false
	mux.Handle("POST /rest/catalogDatabase/v1/schemaMetadataConfigurations/batch", testutil.JsonHandlerInOut(func(r *http.Request, in []clients.SchemaMetadataConfiguration) (int, []clients.SchemaMetadataConfiguration) {
		batchCalled = true
		return http.StatusCreated, in
	}))

	server := httptest.NewServer(mux)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
		DatabaseConnectionID: dbConnID.String(),
		Include:              "*",
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
	if batchCalled {
		t.Fatalf("expected no synchronization rules to be set when schema selection fails")
	}
}

func TestConfigureDatabase_SchemaNamesSelectsSubset(t *testing.T) {
	dbConnID, _ := uuid.NewUUID()
	publicSchemaID, _ := uuid.NewUUID()
	privateSchemaID, _ := uuid.NewUUID()

	mux := http.NewServeMux()
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
		DatabaseConnectionID: dbConnID.String(),
		SchemaNames:          []string{"public"},
		Include:              "*",
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

func TestConfigureDatabase_NoSchemaConnectionsDiscovered(t *testing.T) {
	dbConnID, _ := uuid.NewUUID()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /rest/catalogDatabase/v1/schemaConnections/refresh", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})
	mux.Handle("GET /rest/catalogDatabase/v1/schemaConnections", testutil.JsonHandlerOut(func(r *http.Request) (int, map[string]any) {
		return http.StatusOK, map[string]any{"results": []clients.SchemaConnection{}}
	}))

	server := httptest.NewServer(mux)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
		DatabaseConnectionID: dbConnID.String(),
		Include:              "*",
	})
	if err != nil {
		t.Fatalf("expected no error (failures reported via Output), got: %v", err)
	}
	if output.Success {
		t.Fatalf("expected failure when no schema connections are discovered")
	}
	if output.Error == "" {
		t.Fatalf("expected an error message")
	}
}

func TestConfigureDatabase_InvalidInput(t *testing.T) {
	client := testutil.NewClient(httptest.NewServer(http.NewServeMux()))
	_, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
		DatabaseConnectionID: "not-a-uuid",
		Include:              "*",
	})
	if err == nil {
		t.Fatalf("expected an error for invalid databaseConnectionId")
	}
}
