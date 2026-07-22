package test_connection_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	tools "github.com/collibra/chip/pkg/tools/test_connection"
	"github.com/collibra/chip/pkg/tools/testutil"
	"github.com/google/uuid"
)

func TestTestConnection_Async(t *testing.T) {
	connID, _ := uuid.NewUUID()
	jobID, _ := uuid.NewUUID()

	handler := http.NewServeMux()
	handler.Handle("GET /edge/api/rest/v2/connections/"+connID.String()+"/test", testutil.JsonHandlerOut(func(r *http.Request) (int, clients.TestConnectionResponse) {
		if r.URL.Query().Get("timeoutSec") != "" {
			t.Fatalf("expected no timeoutSec query param, got: %s", r.URL.Query().Get("timeoutSec"))
		}
		return http.StatusOK, clients.TestConnectionResponse{JobID: jobID.String(), Success: true, Message: "Job submitted successfully."}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{ConnectionID: connID.String()})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !output.Success || output.JobID != jobID.String() {
		t.Fatalf("unexpected output: %+v", output)
	}
}

func TestTestConnection_Synchronous(t *testing.T) {
	connID, _ := uuid.NewUUID()
	jobID, _ := uuid.NewUUID()

	handler := http.NewServeMux()
	handler.Handle("GET /edge/api/rest/v2/connections/"+connID.String()+"/test", testutil.JsonHandlerOut(func(r *http.Request) (int, clients.TestConnectionResponse) {
		if r.URL.Query().Get("timeoutSec") != "30" {
			t.Fatalf("expected timeoutSec=30, got: %s", r.URL.Query().Get("timeoutSec"))
		}
		return http.StatusOK, clients.TestConnectionResponse{JobID: jobID.String(), Success: false, Message: "connection refused"}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{ConnectionID: connID.String(), TimeoutSec: 30})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if output.Success {
		t.Fatalf("expected the connection test itself to report failure")
	}
	if output.Message != "connection refused" {
		t.Fatalf("unexpected message: %s", output.Message)
	}
}

func TestTestConnection_InvalidConnectionID(t *testing.T) {
	client := testutil.NewClient(httptest.NewServer(http.NewServeMux()))
	_, err := tools.NewTool(client).Handler(t.Context(), tools.Input{ConnectionID: "not-a-uuid"})
	if err == nil {
		t.Fatalf("expected an error for invalid connectionId")
	}
}
