package edit_maestro_agent_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	tools "github.com/collibra/chip/pkg/tools/edit_maestro_agent"
	"github.com/collibra/chip/pkg/tools/testutil"
	"github.com/google/uuid"
)

const updatedAgent = `{
  "id": "5428d1bd-3453-4c06-b48f-5c74aa12bef5",
  "name": "FinRep Assistant",
  "handle": "finrep",
  "color": "#005ce8",
  "description": "Answers FINREP questions.",
  "isValid": true,
  "status": "DRAFT",
  "instructions": "Role:\nYou are the FinREP Maestro, an expert Data Steward.\n",
  "welcomeMessage": "Hi, I'm here to help you find trusted information.",
  "sampleQuestions": ["Why is LEI a critical data element?"],
  "tools": ["get_asset_details"],
  "knowledgeBase": {
    "views": [
      {"viewId": "6799b268-bec2-4a67-bd16-a2aa28200781", "location": "DOMAIN_DOMAIN_ASSETS"}
    ]
  },
  "sharing": {"roles": [], "groups": ["00000000-0000-0000-0000-000001000001"], "users": []},
  "ownership": {"users": []},
  "createdBy": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "lastModifiedOn": "2026-07-30T15:00:00Z",
  "lastModifiedBy": "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
}`

func agentPath(agentId uuid.UUID) string {
	return "/rest/aiMaestro/v1/agents/" + agentId.String()
}

// requestBody reads the JSON body back as a map, so a test can assert which keys
// reached the API — the point of a partial update is what it leaves out.
func requestBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()

	if contentType := r.Header.Get("Content-Type"); contentType != "application/json" {
		t.Errorf("Expected Content-Type 'application/json', got: %q", contentType)
	}

	raw, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("Failed to read request body: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("Failed to parse request body %q: %v", raw, err)
	}
	return body
}

func respondJSON(w http.ResponseWriter, statusCode int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_, _ = w.Write([]byte(body))
}

func newServer(t *testing.T, agentId uuid.UUID, handler http.HandlerFunc) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc(agentPath(agentId), handler)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

// The whole point of the tool: a one-field edit sends that field and nothing else,
// so every other field keeps its stored value.
func TestEditMaestroAgentSendsOnlyTheFieldsGiven(t *testing.T) {
	agentId, _ := uuid.NewUUID()

	server := newServer(t, agentId, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("Expected PATCH, got: %s", r.Method)
		}
		body := requestBody(t, r)

		if len(body) != 1 {
			t.Errorf("Expected only description to be sent, got: %+v", body)
		}
		if body["description"] != "Answers FINREP questions." {
			t.Errorf("Expected the new description, got: %v", body["description"])
		}
		// Neither field is settable through this tool. Sending a status would stop
		// the server demoting the agent to DRAFT; sending a knowledge base would
		// overwrite views this tool has no business touching.
		for _, field := range []string{"status", "knowledgeBase"} {
			if _, present := body[field]; present {
				t.Errorf("Expected no %s in the request, got: %+v", field, body)
			}
		}

		respondJSON(w, http.StatusOK, updatedAgent)
	})

	output, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{
		AgentID:     agentId.String(),
		Description: chip.Ptr("Answers FINREP questions."),
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if !output.Updated {
		t.Fatalf("Expected the agent to be updated, got error: %s", output.Error)
	}
	if output.Agent == nil {
		t.Fatal("Expected the updated agent to be returned")
	}
	if output.Agent.Description != "Answers FINREP questions." {
		t.Errorf("Expected the new description, got: %q", output.Agent.Description)
	}
	// An edit demotes the agent, and the caller has to be able to see that it did.
	if output.Agent.Status != "DRAFT" {
		t.Errorf("Expected the agent back in DRAFT, got: %q", output.Agent.Status)
	}
	// The knowledge base is untouched by the edit and still comes back.
	if output.Agent.KnowledgeBase == nil || len(output.Agent.KnowledgeBase.Views) != 1 {
		t.Errorf("Expected the knowledge base views intact, got: %+v", output.Agent.KnowledgeBase)
	}
	if output.ErrorCode != "" || output.Error != "" {
		t.Errorf("Expected no error, got: %q %q", output.ErrorCode, output.Error)
	}
}

// An empty list means "clear this field", so it has to reach the API as an empty
// array rather than being dropped like an omitted one.
func TestEditMaestroAgentClearsLists(t *testing.T) {
	agentId, _ := uuid.NewUUID()

	// Clearing sampleQuestions makes the agent's definition incomplete, so the
	// server answers with isValid false. That has to reach the caller: it is the
	// only sign the edit left the agent unsubmittable.
	const clearedAgent = `{
	  "id": "5428d1bd-3453-4c06-b48f-5c74aa12bef5",
	  "name": "FinRep Assistant",
	  "handle": "finrep",
	  "isValid": false,
	  "status": "DRAFT",
	  "instructions": "Role:\nYou are the FinREP Maestro, an expert Data Steward.\n",
	  "welcomeMessage": "Hi, I'm here to help you find trusted information.",
	  "sampleQuestions": [],
	  "sharing": {"roles": [], "groups": [], "users": []},
	  "createdBy": "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
	}`

	server := newServer(t, agentId, func(w http.ResponseWriter, r *http.Request) {
		body := requestBody(t, r)

		for _, field := range []string{"tools", "sampleQuestions"} {
			list, present := body[field]
			if !present {
				t.Errorf("Expected %s to be sent, got: %+v", field, body)
				continue
			}
			if items, ok := list.([]any); !ok || len(items) != 0 {
				t.Errorf("Expected %s to be an empty array, got: %v", field, list)
			}
		}
		if _, present := body["name"]; present {
			t.Errorf("Expected no name in the request, got: %+v", body)
		}

		respondJSON(w, http.StatusOK, clearedAgent)
	})

	output, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{
		AgentID:         agentId.String(),
		Tools:           &[]string{},
		SampleQuestions: &[]string{},
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if !output.Updated {
		t.Fatalf("Expected the agent to be updated, got error: %s", output.Error)
	}
	if output.Agent == nil || output.Agent.IsValid {
		t.Errorf("Expected the agent to be reported as no longer valid, got: %+v", output.Agent)
	}
}

func TestEditMaestroAgentSendsNestedBlocks(t *testing.T) {
	agentId, _ := uuid.NewUUID()

	server := newServer(t, agentId, func(w http.ResponseWriter, r *http.Request) {
		body := requestBody(t, r)

		sharing, ok := body["sharing"].(map[string]any)
		if !ok {
			t.Fatalf("Expected a sharing object, got: %v", body["sharing"])
		}
		// The API requires all three lists once sharing is set, so none may be dropped.
		for _, list := range []string{"roles", "groups", "users"} {
			if _, present := sharing[list]; !present {
				t.Errorf("Expected sharing.%s to be sent, got: %+v", list, sharing)
			}
		}
		groups, ok := sharing["groups"].([]any)
		if !ok || len(groups) != 1 || groups[0] != "00000000-0000-0000-0000-000001000001" {
			t.Errorf("Expected the Everyone group, got: %v", sharing["groups"])
		}
		ownership, ok := body["ownership"].(map[string]any)
		if !ok {
			t.Fatalf("Expected an ownership object, got: %v", body["ownership"])
		}
		if users, ok := ownership["users"].([]any); !ok || len(users) != 0 {
			t.Errorf("Expected ownership.users to be an empty array, got: %v", ownership["users"])
		}

		respondJSON(w, http.StatusOK, updatedAgent)
	})

	output, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{
		AgentID:   agentId.String(),
		Sharing:   &clients.MaestroSharing{Roles: []string{}, Groups: []string{"00000000-0000-0000-0000-000001000001"}, Users: []string{}},
		Ownership: &clients.MaestroOwnership{Users: []string{}},
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if !output.Updated {
		t.Fatalf("Expected the agent to be updated, got error: %s", output.Error)
	}
}

// An edit with no fields in it would still demote the agent to DRAFT, so it is
// refused rather than sent.
func TestEditMaestroAgentRejectsEmptyEdit(t *testing.T) {
	agentId, _ := uuid.NewUUID()

	server := newServer(t, agentId, func(w http.ResponseWriter, r *http.Request) {
		t.Error("Expected no call to the API")
	})

	_, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{
		AgentID: agentId.String(),
	})
	if err == nil {
		t.Fatal("Expected an error for an edit with no fields, got nil")
	}
	// The message names what may be edited so the caller can correct itself.
	if !strings.Contains(err.Error(), "sampleQuestions") {
		t.Errorf("Expected the editable fields in the error, got: %v", err)
	}
}

func TestEditMaestroAgentInvalidUUID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Expected no call to the API")
	}))
	defer server.Close()

	_, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{
		AgentID: "not-a-uuid",
		Name:    chip.Ptr("FinRep Assistant"),
	})
	if err == nil {
		t.Fatal("Expected UUID validation error, got nil")
	}
}

// A malformed UUID would otherwise reach the API as an opaque 400, so it is caught
// before the call and the failing field is named.
func TestEditMaestroAgentRejectsMalformedReferences(t *testing.T) {
	agentId, _ := uuid.NewUUID()

	server := newServer(t, agentId, func(w http.ResponseWriter, r *http.Request) {
		t.Error("Expected no call to the API")
	})

	_, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{
		AgentID: agentId.String(),
		Sharing: &clients.MaestroSharing{Roles: []string{}, Groups: []string{"everyone"}, Users: []string{}},
	})
	if err == nil {
		t.Fatal("Expected an error for a malformed sharing group, got nil")
	}
	if !strings.Contains(err.Error(), "sharing.groups") {
		t.Errorf("Expected the failing field in the error, got: %v", err)
	}
}

// A rejected update changes nothing, and its error code is what tells the caller
// whether the failure is worth recovering from.
func TestEditMaestroAgentDuplicatedHandle(t *testing.T) {
	agentId, _ := uuid.NewUUID()

	server := newServer(t, agentId, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusBadRequest, `{"statusCode":400,"errorCode":"duplicatedHandle","titleMessage":"The agent handle is already in use"}`)
	})

	output, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{
		AgentID: agentId.String(),
		Handle:  chip.Ptr("finrep"),
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.Updated {
		t.Fatal("Expected the update to fail")
	}
	if output.ErrorCode != "duplicatedHandle" {
		t.Errorf("Expected errorCode 'duplicatedHandle', got: %q", output.ErrorCode)
	}
	if !strings.Contains(output.Error, "already in use") {
		t.Errorf("Expected the API message in the error, got: %q", output.Error)
	}
	if output.Agent != nil {
		t.Errorf("Expected no agent, got: %+v", output.Agent)
	}
}

// The caller can see the agent but is not one of its owners — the likeliest
// real-world failure, and it must surface as a readable message rather than a
// protocol error.
func TestEditMaestroAgentForbidden(t *testing.T) {
	agentId, _ := uuid.NewUUID()

	server := newServer(t, agentId, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusForbidden, `{"statusCode":403,"errorCode":"FORBIDDEN_NOT_OWNER","titleMessage":"Only agent owners can perform this operation"}`)
	})

	output, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{
		AgentID:     agentId.String(),
		Description: chip.Ptr("Answers FINREP questions."),
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.Updated {
		t.Fatal("Expected the update to fail")
	}
	if output.ErrorCode != "FORBIDDEN_NOT_OWNER" {
		t.Errorf("Expected errorCode 'FORBIDDEN_NOT_OWNER', got: %q", output.ErrorCode)
	}
	if output.Error == "" {
		t.Error("Expected an error message")
	}
}

// The output has to survive a JSON round trip, since that is how it reaches the
// calling agent.
func TestEditMaestroAgentOutputMarshalsFully(t *testing.T) {
	agentId, _ := uuid.NewUUID()

	server := newServer(t, agentId, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, updatedAgent)
	})

	output, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{
		AgentID:     agentId.String(),
		Description: chip.Ptr("Answers FINREP questions."),
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	encoded, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("Failed to marshal output: %v", err)
	}
	for _, field := range []string{`"handle":"finrep"`, `"status":"DRAFT"`, `"description":"Answers FINREP questions."`, `"updated":true`} {
		if !strings.Contains(string(encoded), field) {
			t.Errorf("Expected %s in the output, got: %s", field, encoded)
		}
	}
}
