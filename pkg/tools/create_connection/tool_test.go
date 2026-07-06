package create_connection_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	tools "github.com/collibra/chip/pkg/tools/create_connection"
	"github.com/collibra/chip/pkg/tools/testutil"
	"github.com/google/uuid"
)

func TestCreateConnection_Create(t *testing.T) {
	siteID, _ := uuid.NewUUID()
	connID, _ := uuid.NewUUID()

	handler := http.NewServeMux()
	handler.Handle("POST /edge/api/rest/v2/connections", testutil.JsonHandlerInOut(func(r *http.Request, in clients.ConnectionRequest) (int, clients.Connection) {
		if in.Name != "local-postgres-source" {
			t.Fatalf("unexpected name: %s", in.Name)
		}
		if in.EdgeSiteID != siteID.String() {
			t.Fatalf("unexpected edgeSiteId: %s", in.EdgeSiteID)
		}
		return http.StatusCreated, clients.Connection{
			ID:         connID.String(),
			Name:       in.Name,
			TypeID:     in.TypeID,
			EdgeSiteID: in.EdgeSiteID,
			Parameters: in.Parameters,
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
		Name:       "local-postgres-source",
		TypeID:     "Generic",
		EdgeSiteID: siteID.String(),
		Parameters: map[string]any{"driver-class": "org.postgresql.Driver"},
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !output.Success {
		t.Fatalf("expected success, got error: %s", output.Error)
	}
	if output.Connection.ID != connID.String() {
		t.Fatalf("expected id %s, got %s", connID.String(), output.Connection.ID)
	}
}

func TestCreateConnection_UpdateWithKnownID(t *testing.T) {
	siteID, _ := uuid.NewUUID()
	connID, _ := uuid.NewUUID()

	handler := http.NewServeMux()
	handler.Handle("PUT /edge/api/rest/v2/connections/"+connID.String(), testutil.JsonHandlerInOut(func(r *http.Request, in clients.ConnectionRequest) (int, clients.Connection) {
		return http.StatusOK, clients.Connection{
			ID:         connID.String(),
			Name:       in.Name,
			TypeID:     in.TypeID,
			EdgeSiteID: in.EdgeSiteID,
			Parameters: in.Parameters,
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
		ConnectionID: connID.String(),
		Name:         "local-postgres-source",
		TypeID:       "Generic",
		EdgeSiteID:   siteID.String(),
		Parameters:   map[string]any{},
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !output.Success {
		t.Fatalf("expected success, got error: %s", output.Error)
	}
	if output.Connection.ID != connID.String() {
		t.Fatalf("expected id %s, got %s", connID.String(), output.Connection.ID)
	}
}

func TestCreateConnection_InvalidEdgeSiteID(t *testing.T) {
	client := testutil.NewClient(httptest.NewServer(http.NewServeMux()))
	_, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
		Name:       "bad",
		TypeID:     "Generic",
		EdgeSiteID: "not-a-uuid",
	})
	if err == nil {
		t.Fatalf("expected an error for invalid edgeSiteId")
	}
}

func TestCreateConnection_AdditionalProperties(t *testing.T) {
	// Mirrors a real, confirmed-working Snowflake-via-Generic-JDBC connection's
	// parameters: driver-class/connection-string/driver-jar fixed, plus a
	// "connection-properties" array of {name, type, value, secret} for Role,
	// Warehouse, private_key_file, User, Database.
	siteID, _ := uuid.NewUUID()
	connID, _ := uuid.NewUUID()

	handler := http.NewServeMux()
	handler.Handle("POST /edge/api/rest/v2/connections", testutil.JsonHandlerInOut(func(r *http.Request, in clients.ConnectionRequest) (int, clients.Connection) {
		props, ok := in.Parameters["connection-properties"].([]any)
		if !ok || len(props) != 2 {
			t.Fatalf("expected 2 connection-properties entries, got: %#v", in.Parameters["connection-properties"])
		}
		role, ok := props[0].(map[string]any)
		if !ok || role["name"] != "Role" || role["type"] != "string" || role["value"] != "SOFTWARE_DEVELOPMENT" || role["secret"] != false {
			t.Fatalf("unexpected Role entry: %#v", props[0])
		}
		keyFile, ok := props[1].(map[string]any)
		if !ok || keyFile["name"] != "private_key_file" || keyFile["type"] != "file" || keyFile["value"] != "artifact://uuid/key.p8" || keyFile["secret"] != true {
			t.Fatalf("unexpected private_key_file entry: %#v", props[1])
		}
		return http.StatusCreated, clients.Connection{ID: connID.String(), Name: in.Name, EdgeSiteID: in.EdgeSiteID, Parameters: in.Parameters}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
		Name:       "snowflake-jdbc",
		TypeID:     "Generic",
		EdgeSiteID: siteID.String(),
		Parameters: map[string]any{
			"driver-class":      "net.snowflake.client.jdbc.SnowflakeDriver",
			"connection-string": "jdbc:snowflake://acme.snowflakecomputing.com",
		},
		AdditionalProperties: []tools.AdditionalProperty{
			{Name: "Role", Value: "SOFTWARE_DEVELOPMENT"},
			{Name: "private_key_file", Type: "file", Value: "artifact://uuid/key.p8", Secret: true},
		},
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !output.Success {
		t.Fatalf("expected success, got error: %s", output.Error)
	}
}

func TestCreateConnection_AdditionalProperties_CustomKey(t *testing.T) {
	siteID, _ := uuid.NewUUID()

	handler := http.NewServeMux()
	handler.Handle("POST /edge/api/rest/v2/connections", testutil.JsonHandlerInOut(func(r *http.Request, in clients.ConnectionRequest) (int, clients.Connection) {
		if _, ok := in.Parameters["additional-parameters"]; !ok {
			t.Fatalf("expected additional-parameters key, got parameters: %#v", in.Parameters)
		}
		return http.StatusCreated, clients.Connection{Parameters: in.Parameters}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
		Name:                    "aws-conn",
		TypeID:                  "AWS",
		EdgeSiteID:              siteID.String(),
		AdditionalPropertiesKey: "additional-parameters",
		AdditionalProperties:    []tools.AdditionalProperty{{Name: "region", Value: "us-east-1"}},
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !output.Success {
		t.Fatalf("expected success, got error: %s", output.Error)
	}
}

func TestCreateConnection_DriverJarUpload(t *testing.T) {
	siteID, _ := uuid.NewUUID()
	connID, _ := uuid.NewUUID()

	var uploadedJarURI string
	handler := http.NewServeMux()
	handler.HandleFunc("POST /edge/api/rest/v2/upload", func(w http.ResponseWriter, r *http.Request) {
		uploadedJarURI = "jar://test-uuid/driver.jar"
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`"` + uploadedJarURI + `"`))
	})
	handler.Handle("POST /edge/api/rest/v2/connections", testutil.JsonHandlerInOut(func(r *http.Request, in clients.ConnectionRequest) (int, clients.Connection) {
		if in.Parameters["driver-jar"] != uploadedJarURI {
			t.Fatalf("expected driver-jar %q, got %v", uploadedJarURI, in.Parameters["driver-jar"])
		}
		return http.StatusCreated, clients.Connection{
			ID:         connID.String(),
			Name:       in.Name,
			EdgeSiteID: in.EdgeSiteID,
			Parameters: in.Parameters,
		}
	}))

	edgeServer := httptest.NewServer(handler)
	defer edgeServer.Close()

	driverServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("fake jar bytes"))
	}))
	defer driverServer.Close()

	client := testutil.NewClient(edgeServer)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
		Name:              "local-postgres-source",
		TypeID:            "Generic",
		EdgeSiteID:        siteID.String(),
		DriverJarURL:      driverServer.URL,
		DriverJarFilename: "driver.jar",
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !output.Success {
		t.Fatalf("expected success, got error: %s", output.Error)
	}
}
