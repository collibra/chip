package edge_create_capability_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	tools "github.com/collibra/chip/pkg/tools/edge_create_capability"
	"github.com/collibra/chip/pkg/tools/testutil"
	"github.com/google/uuid"
)

func TestCreateCapability_Create(t *testing.T) {
	siteID, _ := uuid.NewUUID()
	capID, _ := uuid.NewUUID()

	handler := http.NewServeMux()
	handler.Handle("POST /edge/api/rest/v2/capabilities", testutil.JsonHandlerInOut(func(r *http.Request, in clients.CapabilityRequest) (int, clients.EdgeCapability) {
		if in.TypeID != "jdbc-ingestion" {
			t.Fatalf("unexpected typeId: %s", in.TypeID)
		}
		return http.StatusCreated, clients.EdgeCapability{
			Id:         capID.String(),
			Name:       in.Name,
			EdgeSiteId: in.EdgeSiteID,
			Parameters: in.Parameters,
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
		Name:       "local-jdbc-ingestion",
		TypeID:     "jdbc-ingestion",
		EdgeSiteID: siteID.String(),
		Parameters: map[string]any{"connection": "some-connection-id", "message-mode": "KAFKA"},
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !output.Success {
		t.Fatalf("expected success, got error: %s", output.Error)
	}
	if output.Capability.Id != capID.String() {
		t.Fatalf("expected id %s, got %s", capID.String(), output.Capability.Id)
	}
}

func TestCreateCapability_UpdateWithKnownID(t *testing.T) {
	siteID, _ := uuid.NewUUID()
	capID, _ := uuid.NewUUID()

	handler := http.NewServeMux()
	handler.Handle("PUT /edge/api/rest/v2/capabilities/"+capID.String(), testutil.JsonHandlerInOut(func(r *http.Request, in clients.CapabilityRequest) (int, clients.EdgeCapability) {
		return http.StatusOK, clients.EdgeCapability{
			Id:         capID.String(),
			Name:       in.Name,
			EdgeSiteId: in.EdgeSiteID,
			Parameters: in.Parameters,
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
		CapabilityID: capID.String(),
		Name:         "local-jdbc-ingestion",
		TypeID:       "jdbc-ingestion",
		EdgeSiteID:   siteID.String(),
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !output.Success {
		t.Fatalf("expected success, got error: %s", output.Error)
	}
}

func TestCreateCapability_InvalidCapabilityID(t *testing.T) {
	client := testutil.NewClient(httptest.NewServer(http.NewServeMux()))
	_, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
		CapabilityID: "not-a-uuid",
		Name:         "bad",
		TypeID:       "jdbc-ingestion",
		EdgeSiteID:   "also-not-a-uuid",
	})
	if err == nil {
		t.Fatalf("expected an error for invalid capabilityId")
	}
}
