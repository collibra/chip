package get_maestro_agent_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	tools "github.com/collibra/chip/pkg/tools/get_maestro_agent"
	"github.com/collibra/chip/pkg/tools/testutil"
	"github.com/google/uuid"
)

const agentConfiguration = `handle: finrep
name: FinRep Assistant
description: Answers questions about EBA FINREP regulatory reporting.
instructions: |
  Role:
  You are the FinREP Maestro, an expert Data Steward.
color: "#005ce8"
welcomeMessage: Hi, I'm here to help you find trusted information.
sampleQuestions:
  - Why is LEI a critical data element?
tools:
  - get_asset_details
  - list_asset_types
knowledgeBase:
  views:
    - viewId: 6799b268-bec2-4a67-bd16-a2aa28200781
      location: DOMAIN_DOMAIN_ASSETS
sharing:
  roles: []
  groups:
    - 00000000-0000-0000-0000-000001000001
  users: []
ownership:
  users: []
`

func configurationFilePath(agentId uuid.UUID) string {
	return "/rest/aiMaestro/v1/agents/" + agentId.String() + "/configurationFile"
}

func TestGetMaestroAgent(t *testing.T) {
	agentId, _ := uuid.NewUUID()

	handler := http.NewServeMux()
	handler.Handle(configurationFilePath(agentId), testutil.StringHandlerOut(func(r *http.Request) (int, string) {
		if accept := r.Header.Get("Accept"); accept != "application/yaml" {
			t.Errorf("Expected Accept 'application/yaml', got: %q", accept)
		}
		return http.StatusOK, agentConfiguration
	}))

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

	if output.Error != "" {
		t.Fatalf("Expected no error, got: %s", output.Error)
	}

	// The configuration must be passed through byte-for-byte so the caller can edit
	// it and write it back without losing fields.
	if output.Configuration != agentConfiguration {
		t.Fatalf("Expected configuration '%s', got: '%s'", agentConfiguration, output.Configuration)
	}
}

func TestGetMaestroAgentInvalidUUID(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
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
	handler.Handle(configurationFilePath(agentId), http.NotFoundHandler())

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
}

// The caller can see the agent but is not one of its owners — the likeliest
// real-world failure, and it must surface as a readable message rather than a
// protocol error.
func TestGetMaestroAgentForbidden(t *testing.T) {
	agentId, _ := uuid.NewUUID()

	handler := http.NewServeMux()
	handler.HandleFunc(configurationFilePath(agentId), func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"statusCode":403,"errorCode":"forbiddenNotOwner","titleMessage":"Forbidden"}`))
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
		t.Fatal("Expected error message for forbidden")
	}

	if output.Configuration != "" {
		t.Fatalf("Expected no configuration, got: %s", output.Configuration)
	}
}
