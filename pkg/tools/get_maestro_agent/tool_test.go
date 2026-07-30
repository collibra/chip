package get_maestro_agent_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	tools "github.com/collibra/chip/pkg/tools/get_maestro_agent"
	"github.com/collibra/chip/pkg/tools/testutil"
	"github.com/google/uuid"
)

const agentJSON = `{
  "id": "5428d1bd-3453-4c06-b48f-5c74aa12bef5",
  "name": "FinRep Assistant",
  "handle": "finrep",
  "color": "#005ce8",
  "description": "Answers questions about EBA FINREP regulatory reporting.",
  "isValid": true,
  "status": "PUBLISHED",
  "instructions": "Role:\nYou are the FinREP Maestro, an expert Data Steward.\n",
  "welcomeMessage": "Hi, I'm here to help you find trusted information.",
  "sampleQuestions": ["Why is LEI a critical data element?"],
  "tools": ["get_asset_details", "list_asset_types"],
  "knowledgeBase": {
    "views": [
      {"viewId": "6799b268-bec2-4a67-bd16-a2aa28200781", "location": "DOMAIN_DOMAIN_ASSETS", "selectedCommunityId": "0fdd1b6d-325d-458e-a0ef-9ebf646ab25a"}
    ]
  },
  "sharing": {"roles": [], "groups": ["00000000-0000-0000-0000-000001000001"], "users": []},
  "ownership": {"users": ["b2c3d4e5-f6a7-8901-bcde-f12345678901"]},
  "createdBy": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "lastModifiedOn": "2026-07-30T15:00:00Z",
  "lastModifiedBy": "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
}`

func agentPath(agentId uuid.UUID) string {
	return "/rest/aiMaestro/v1/agents/" + agentId.String()
}

func TestGetMaestroAgent(t *testing.T) {
	agentId, _ := uuid.NewUUID()

	handler := http.NewServeMux()
	handler.HandleFunc(agentPath(agentId), func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Expected GET, got: %s", r.Method)
		}
		if accept := r.Header.Get("Accept"); accept != "application/json" {
			t.Errorf("Expected Accept 'application/json', got: %q", accept)
		}
		respondJSON(w, http.StatusOK, agentJSON)
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
		AgentID: agentId.String(),
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if !output.Found {
		t.Fatal("Expected agent to be found")
	}
	if output.Error != "" || output.ErrorCode != "" {
		t.Fatalf("Expected no error, got: %q %q", output.ErrorCode, output.Error)
	}
	if output.Agent == nil {
		t.Fatal("Expected the agent to be returned")
	}

	if output.Agent.ID != "5428d1bd-3453-4c06-b48f-5c74aa12bef5" {
		t.Errorf("Expected the agent id, got: %q", output.Agent.ID)
	}
	if output.Agent.Handle != "finrep" || output.Agent.Status != "PUBLISHED" {
		t.Errorf("Expected handle 'finrep' in PUBLISHED, got: %q in %q", output.Agent.Handle, output.Agent.Status)
	}
	if !output.Agent.IsValid {
		t.Error("Expected the agent to be reported as valid")
	}
	if len(output.Agent.SampleQuestions) != 1 || len(output.Agent.Tools) != 2 {
		t.Errorf("Expected the lists intact, got: %+v %+v", output.Agent.SampleQuestions, output.Agent.Tools)
	}

	// Nested structures are passed through, not flattened away: the knowledge base
	// is only readable here, so this tool is the only place it surfaces.
	if output.Agent.KnowledgeBase == nil || len(output.Agent.KnowledgeBase.Views) != 1 {
		t.Fatalf("Expected one knowledge base view, got: %+v", output.Agent.KnowledgeBase)
	}
	view := output.Agent.KnowledgeBase.Views[0]
	if view.ViewID != "6799b268-bec2-4a67-bd16-a2aa28200781" || view.Location != "DOMAIN_DOMAIN_ASSETS" {
		t.Errorf("Expected the view intact, got: %+v", view)
	}
	if view.SelectedCommunityID != "0fdd1b6d-325d-458e-a0ef-9ebf646ab25a" {
		t.Errorf("Expected the view's community, got: %q", view.SelectedCommunityID)
	}
	if output.Agent.Sharing == nil || len(output.Agent.Sharing.Groups) != 1 {
		t.Errorf("Expected the sharing groups, got: %+v", output.Agent.Sharing)
	}
	if output.Agent.Ownership == nil || len(output.Agent.Ownership.Users) != 1 {
		t.Errorf("Expected the ownership users, got: %+v", output.Agent.Ownership)
	}
}

func TestGetMaestroAgentInvalidUUID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Expected no call to the API")
	}))
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
		AgentID: "not-a-uuid",
	})
	if err == nil {
		t.Fatal("Expected UUID validation error, got nil")
	}
}

func TestGetMaestroAgentNotFound(t *testing.T) {
	agentId, _ := uuid.NewUUID()

	handler := http.NewServeMux()
	handler.HandleFunc(agentPath(agentId), func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, `{"statusCode":404,"errorCode":"notFound","titleMessage":"Resource not found"}`)
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
		AgentID: agentId.String(),
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.Found {
		t.Fatal("Expected agent not to be found")
	}
	if output.Error == "" {
		t.Fatal("Expected error message for not found")
	}
	if output.ErrorCode != "notFound" {
		t.Errorf("Expected errorCode 'notFound', got: %q", output.ErrorCode)
	}
	if output.Agent != nil {
		t.Errorf("Expected no agent, got: %+v", output.Agent)
	}
}

// A failure that is not the documented envelope — a proxy answering 401, say —
// still has to surface as a readable message rather than a protocol error.
func TestGetMaestroAgentUnauthorized(t *testing.T) {
	agentId, _ := uuid.NewUUID()

	handler := http.NewServeMux()
	handler.HandleFunc(agentPath(agentId), func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("<html>Unauthorized</html>"))
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
		AgentID: agentId.String(),
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.Found {
		t.Fatal("Expected agent not to be found")
	}
	if output.Error == "" {
		t.Fatal("Expected an error message")
	}
	if output.ErrorCode != "" {
		t.Errorf("Expected no error code, got: %q", output.ErrorCode)
	}
}

func respondJSON(w http.ResponseWriter, statusCode int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_, _ = w.Write([]byte(body))
}
