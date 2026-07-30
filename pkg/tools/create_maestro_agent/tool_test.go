package create_maestro_agent_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	tools "github.com/collibra/chip/pkg/tools/create_maestro_agent"
	"github.com/collibra/chip/pkg/tools/testutil"
)

const agentsPath = "/rest/aiMaestro/v1/agents"

const createdAgent = `{
  "id": "5428d1bd-3453-4c06-b48f-5c74aa12bef5",
  "name": "FinRep Assistant",
  "handle": "finrep",
  "color": "#005ce8",
  "description": "Answers questions about EBA FINREP regulatory reporting.",
  "isValid": true,
  "status": "DRAFT",
  "instructions": "Role:\nYou are the FinREP Maestro, an expert Data Steward.\n",
  "welcomeMessage": "Hi, I'm here to help you find trusted information.",
  "sampleQuestions": ["Why is LEI a critical data element?"],
  "tools": ["get_asset_details"],
  "sharing": {"roles": [], "groups": ["00000000-0000-0000-0000-000001000001"], "users": []},
  "ownership": {"users": []},
  "createdBy": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "lastModifiedOn": "2026-07-30T15:00:00Z",
  "lastModifiedBy": "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
}`

// validInput is a request the API would accept, for tests that care about one
// specific thing going wrong elsewhere.
func validInput() tools.Input {
	return tools.Input{
		Name:            "FinRep Assistant",
		Handle:          "finrep",
		Description:     "Answers questions about EBA FINREP regulatory reporting.",
		Instructions:    "Role:\nYou are the FinREP Maestro, an expert Data Steward.\n",
		Color:           "#005ce8",
		WelcomeMessage:  "Hi, I'm here to help you find trusted information.",
		SampleQuestions: []string{"Why is LEI a critical data element?"},
		Tools:           []string{"get_asset_details"},
		Sharing:         &clients.MaestroSharing{Roles: []string{}, Groups: []string{"00000000-0000-0000-0000-000001000001"}, Users: []string{}},
		Ownership:       &clients.MaestroOwnership{Users: []string{}},
	}
}

// requestBody reads the JSON body back as a map, so a test can assert on the keys
// that reached the API as well as their values.
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

func newServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc(agentsPath, handler)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func TestCreateMaestroAgent(t *testing.T) {
	server := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST, got: %s", r.Method)
		}
		body := requestBody(t, r)

		if body["name"] != "FinRep Assistant" || body["handle"] != "finrep" {
			t.Errorf("Expected the name and handle, got: %+v", body)
		}
		if body["color"] != "#005ce8" {
			t.Errorf("Expected the color, got: %v", body["color"])
		}
		// Nested blocks reach the API as objects, not flattened away.
		sharing, ok := body["sharing"].(map[string]any)
		if !ok {
			t.Fatalf("Expected a sharing object, got: %v", body["sharing"])
		}
		for _, list := range []string{"roles", "groups", "users"} {
			if _, present := sharing[list]; !present {
				// The API requires all three once sharing is set, so none may be dropped.
				t.Errorf("Expected sharing.%s to be sent, got: %+v", list, sharing)
			}
		}
		if _, ok := body["ownership"].(map[string]any); !ok {
			t.Errorf("Expected an ownership object, got: %v", body["ownership"])
		}
		// Neither field is settable through this tool, so neither may leak into the
		// request: a status would be rejected outright, and a knowledge base is
		// configured in AI Maestro.
		for _, field := range []string{"status", "knowledgeBase"} {
			if _, present := body[field]; present {
				t.Errorf("Expected no %s in the request, got: %+v", field, body)
			}
		}

		respondJSON(w, http.StatusCreated, createdAgent)
	})

	output, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), validInput())
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if !output.Created {
		t.Fatalf("Expected the agent to be created, got error: %s", output.Error)
	}
	if output.Agent == nil {
		t.Fatal("Expected the created agent to be returned")
	}
	if output.Agent.ID != "5428d1bd-3453-4c06-b48f-5c74aa12bef5" {
		t.Errorf("Expected the agent id, got: %q", output.Agent.ID)
	}
	if output.Agent.Handle != "finrep" || output.Agent.Status != "DRAFT" {
		t.Errorf("Expected handle 'finrep' in DRAFT, got: %q in %q", output.Agent.Handle, output.Agent.Status)
	}
	// This request fills in all five fields the server's completeness check looks at
	// — name, handle, instructions, welcomeMessage and a sample question — so the
	// agent is submittable straight away, with no knowledge base configured.
	if !output.Agent.IsValid {
		t.Error("Expected the new agent to be reported as valid")
	}
	if output.Agent.Sharing == nil || len(output.Agent.Sharing.Groups) != 1 {
		t.Errorf("Expected the sharing groups, got: %+v", output.Agent.Sharing)
	}
	if output.ErrorCode != "" || output.Error != "" {
		t.Errorf("Expected no error, got: %q %q", output.ErrorCode, output.Error)
	}
}

// Only name and handle are required, and the optional fields must not be invented
// when the caller leaves them out. Such an agent is created, but its definition is
// incomplete, and the caller has to be able to see that in isValid.
func TestCreateMaestroAgentSendsOnlyWhatItWasGiven(t *testing.T) {
	const incompleteAgent = `{
	  "id": "5428d1bd-3453-4c06-b48f-5c74aa12bef5",
	  "name": "FinRep Assistant",
	  "handle": "finrep",
	  "color": "#005ce8",
	  "isValid": false,
	  "status": "DRAFT",
	  "sampleQuestions": [],
	  "sharing": {"roles": [], "groups": [], "users": []},
	  "createdBy": "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
	}`

	server := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		body := requestBody(t, r)

		if len(body) != 2 {
			t.Errorf("Expected only name and handle to be sent, got: %+v", body)
		}
		if body["name"] != "FinRep Assistant" || body["handle"] != "finrep" {
			t.Errorf("Expected the name and handle, got: %+v", body)
		}

		respondJSON(w, http.StatusCreated, incompleteAgent)
	})

	output, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{
		Name:   "FinRep Assistant",
		Handle: "finrep",
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if !output.Created {
		t.Fatalf("Expected the agent to be created, got error: %s", output.Error)
	}
	// instructions, welcomeMessage and sampleQuestions are still missing, so the
	// agent cannot be submitted for review yet.
	if output.Agent == nil || output.Agent.IsValid {
		t.Errorf("Expected the incomplete agent to be reported as not valid, got: %+v", output.Agent)
	}
}

// A duplicate handle is the failure the caller is expected to recover from, so its
// error code has to survive.
func TestCreateMaestroAgentDuplicatedHandle(t *testing.T) {
	server := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusBadRequest, `{"statusCode":400,"errorCode":"duplicatedHandle","titleMessage":"The agent handle is already in use"}`)
	})

	output, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), validInput())
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.Created {
		t.Fatal("Expected the creation to fail")
	}
	if output.ErrorCode != "duplicatedHandle" {
		t.Errorf("Expected errorCode 'duplicatedHandle', got: %q", output.ErrorCode)
	}
	// The API's own message names the problem, so it must reach the caller too.
	if !strings.Contains(output.Error, "already in use") {
		t.Errorf("Expected the API message in the error, got: %q", output.Error)
	}
	if output.Agent != nil {
		t.Errorf("Expected no agent, got: %+v", output.Agent)
	}
}

// A failure that is not the documented envelope — a proxy answering 401, say —
// still has to surface as a readable message rather than a protocol error.
func TestCreateMaestroAgentUnauthorized(t *testing.T) {
	server := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("<html>Unauthorized</html>"))
	})

	output, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), validInput())
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.Created {
		t.Fatal("Expected the creation to fail")
	}
	if output.Error == "" {
		t.Error("Expected an error message")
	}
	if output.ErrorCode != "" {
		t.Errorf("Expected no error code, got: %q", output.ErrorCode)
	}
}

func TestCreateMaestroAgentRejectsMissingRequiredFields(t *testing.T) {
	server := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("Expected no call to the API")
	})

	cases := map[string]tools.Input{
		"no name":      {Handle: "finrep"},
		"blank name":   {Name: "   \n", Handle: "finrep"},
		"no handle":    {Name: "FinRep Assistant"},
		"blank handle": {Name: "FinRep Assistant", Handle: "  "},
	}
	for name, input := range cases {
		if _, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), input); err == nil {
			t.Errorf("Expected an error for %s, got nil", name)
		}
	}
}

// A malformed UUID would otherwise reach the API as an opaque 400, so it is caught
// before the call and the failing field is named.
func TestCreateMaestroAgentRejectsMalformedReferences(t *testing.T) {
	server := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("Expected no call to the API")
	})

	input := validInput()
	input.Ownership = &clients.MaestroOwnership{Users: []string{"not-a-uuid"}}

	_, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), input)
	if err == nil {
		t.Fatal("Expected an error for a malformed ownership user, got nil")
	}
	if !strings.Contains(err.Error(), "ownership.users") {
		t.Errorf("Expected the failing field in the error, got: %v", err)
	}
}

// The output has to survive a JSON round trip, since that is how it reaches the
// calling agent.
func TestCreateMaestroAgentOutputMarshalsFully(t *testing.T) {
	server := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusCreated, createdAgent)
	})

	output, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), validInput())
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	encoded, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("Failed to marshal output: %v", err)
	}
	for _, field := range []string{`"handle":"finrep"`, `"status":"DRAFT"`, `"isValid":true`, `"created":true`} {
		if !strings.Contains(string(encoded), field) {
			t.Errorf("Expected %s in the output, got: %s", field, encoded)
		}
	}
}
