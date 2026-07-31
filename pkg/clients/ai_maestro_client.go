package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
)

// AI Maestro API v1 (/rest/aiMaestro/v1) — the AI Maestro application's own REST
// API, where Maestro Agent definitions live. Agents are NOT catalog assets, so
// they are reached here rather than through /rest/2.0. Served on the same host as
// the DGC API, so the shared RoundTripper injects host + auth transparently.
const aiMaestroAPIBasePath = "/rest/aiMaestro/v1"

// MaestroAgent is a Maestro Agent as the API returns it, runtime fields included.
type MaestroAgent struct {
	ID              string                `json:"id"`
	Name            string                `json:"name"`
	Handle          string                `json:"handle"`
	Color           string                `json:"color,omitempty"`
	Description     string                `json:"description,omitempty"`
	IsValid         bool                  `json:"isValid"`
	Instructions    string                `json:"instructions,omitempty"`
	KnowledgeBase   *MaestroKnowledgeBase `json:"knowledgeBase,omitempty"`
	WelcomeMessage  string                `json:"welcomeMessage,omitempty"`
	SampleQuestions []string              `json:"sampleQuestions,omitempty"`
	Tools           []string              `json:"tools,omitempty"`
	Sharing         *MaestroSharing       `json:"sharing,omitempty"`
	Ownership       *MaestroOwnership     `json:"ownership,omitempty"`
	Status          string                `json:"status,omitempty"`
	CreatedBy       string                `json:"createdBy,omitempty"`
	LastModifiedOn  string                `json:"lastModifiedOn,omitempty"`
	LastModifiedBy  string                `json:"lastModifiedBy,omitempty"`
}

// MaestroKnowledgeBase is the set of Collibra views the agent may draw on. Read
// only: knowledge base views are configured in AI Maestro, so neither agent
// request type carries them.
type MaestroKnowledgeBase struct {
	Views []MaestroKnowledgeBaseView `json:"views,omitempty"`
}

type MaestroKnowledgeBaseView struct {
	ViewID              string `json:"viewId"`
	Location            string `json:"location"`
	SelectedCommunityID string `json:"selectedCommunityId,omitempty"`
}

// MaestroSharing lists who, beside the creator, sees the agent in Collibra Copilot.
// The API requires all three lists once the object is present, so none of them
// carry omitempty.
type MaestroSharing struct {
	Roles  []string `json:"roles"`
	Groups []string `json:"groups"`
	Users  []string `json:"users"`
}

// MaestroOwnership lists who, beside the creator, may edit the agent.
type MaestroOwnership struct {
	Users []string `json:"users"`
}

// CreateAgentRequest is the POST /agents body. status is deliberately absent: the
// endpoint only accepts DRAFT, which is also its default. knowledgeBase is absent
// too — views are configured in AI Maestro, not through this client.
type CreateAgentRequest struct {
	Name            string            `json:"name"`
	Handle          string            `json:"handle"`
	Color           string            `json:"color,omitempty"`
	Description     string            `json:"description,omitempty"`
	Instructions    string            `json:"instructions,omitempty"`
	WelcomeMessage  string            `json:"welcomeMessage,omitempty"`
	SampleQuestions []string          `json:"sampleQuestions,omitempty"`
	Tools           []string          `json:"tools,omitempty"`
	Sharing         *MaestroSharing   `json:"sharing,omitempty"`
	Ownership       *MaestroOwnership `json:"ownership,omitempty"`
}

// PatchAgentRequest is the PATCH /agents/{agentId} body. Every field is optional
// and nil means "leave unchanged", so pointers are needed throughout: an omitted
// list preserves the stored one, an empty list clears it.
//
// status is deliberately absent. The server reads its absence as "demote to
// DRAFT", which is what an edit should do — edited content has to go back through
// review before end users see it again. knowledgeBase is absent so an edit never
// touches the agent's views.
type PatchAgentRequest struct {
	Name            *string           `json:"name,omitempty"`
	Handle          *string           `json:"handle,omitempty"`
	Color           *string           `json:"color,omitempty"`
	Description     *string           `json:"description,omitempty"`
	Instructions    *string           `json:"instructions,omitempty"`
	WelcomeMessage  *string           `json:"welcomeMessage,omitempty"`
	SampleQuestions *[]string         `json:"sampleQuestions,omitempty"`
	Tools           *[]string         `json:"tools,omitempty"`
	Sharing         *MaestroSharing   `json:"sharing,omitempty"`
	Ownership       *MaestroOwnership `json:"ownership,omitempty"`
}

// MaestroAgentError is AI Maestro's StandardErrorResponse carried as an error, so
// the machine-readable error code survives to the caller instead of being
// flattened into a message. Codes worth acting on include duplicatedHandle,
// BAD_REQUEST_UNSUPPORTED_TOOL, BAD_REQUEST_CREATOR_IN_OWNERSHIP_USERS and
// agentInvalid.
type MaestroAgentError struct {
	StatusCode   int    `json:"statusCode"`
	ErrorCode    string `json:"errorCode"`
	TitleMessage string `json:"titleMessage,omitempty"`
}

func (e *MaestroAgentError) Error() string {
	message := fmt.Sprintf("HTTP %d [%s]", e.StatusCode, e.ErrorCode)
	if e.TitleMessage != "" {
		message += ": " + e.TitleMessage
	}
	return message
}

// GetAgent reads a Maestro Agent. The endpoint answers 404 both for an unknown
// agent and for one the caller cannot see; seeing an agent is enough to read it,
// being allowed to edit it is not required.
func GetAgent(ctx context.Context, collibraHttpClient *http.Client, agentID string) (*MaestroAgent, error) {
	slog.InfoContext(ctx, fmt.Sprintf("Retrieving Maestro agent ID: %s", agentID))

	endpoint := fmt.Sprintf("%s/agents/%s", aiMaestroAPIBasePath, url.PathEscape(agentID))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("get agent: building request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	return sendMaestroAgentRequest(collibraHttpClient, req, "get agent")
}

// CreateAgent creates a Maestro Agent. The agent is always created in DRAFT with
// the connecting user as its creator and owner, and has to be submitted and
// approved in AI Maestro before end users can use it.
func CreateAgent(ctx context.Context, collibraHttpClient *http.Client, request CreateAgentRequest) (*MaestroAgent, error) {
	slog.InfoContext(ctx, fmt.Sprintf("Creating Maestro agent with handle: %s", request.Handle))

	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("create agent: marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, aiMaestroAPIBasePath+"/agents", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create agent: building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	return sendMaestroAgentRequest(collibraHttpClient, req, "create agent")
}

// UpdateAgent applies a partial update to a Maestro Agent (PATCH) and returns the
// agent as stored afterwards. Fields left out of the request keep their stored
// value; because the request carries no status, the server demotes the agent to
// DRAFT, so an edited agent has to be submitted and approved again.
func UpdateAgent(ctx context.Context, collibraHttpClient *http.Client, agentID string, request PatchAgentRequest) (*MaestroAgent, error) {
	slog.InfoContext(ctx, fmt.Sprintf("Updating Maestro agent ID: %s", agentID))

	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("update agent: marshaling request: %w", err)
	}

	endpoint := fmt.Sprintf("%s/agents/%s", aiMaestroAPIBasePath, url.PathEscape(agentID))

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("update agent: building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	return sendMaestroAgentRequest(collibraHttpClient, req, "update agent")
}

// sendMaestroAgentRequest runs a request that all three agent endpoints answer the
// same way — with the agent as stored — and decodes it. operation names the call
// in the errors it returns.
func sendMaestroAgentRequest(client *http.Client, req *http.Request, operation string) (*MaestroAgent, error) {
	responseBody, err := executeMaestroAgentRequest(client, req)
	if err != nil {
		return nil, err
	}

	var agent MaestroAgent
	if err := json.Unmarshal(responseBody, &agent); err != nil {
		return nil, fmt.Errorf("%s: decoding response: %w", operation, err)
	}
	return &agent, nil
}

// executeMaestroAgentRequest returns the success body. Unlike executeRequest it
// does not flatten a failure into a message: a parseable error body becomes a
// *MaestroAgentError so the error code reaches the caller, which is what tells it
// whether the failure is worth recovering from.
func executeMaestroAgentRequest(client *http.Client, req *http.Request) ([]byte, error) {
	response, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer func(body io.ReadCloser) {
		_ = body.Close()
	}(response.Body)

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, maestroAgentFailure(response.StatusCode, responseBody)
	}

	return responseBody, nil
}

// maestroAgentFailure reads the error envelope out of a non-2xx body. Bodies that
// are not the documented envelope — a gateway error page, say — fall back to a
// plain error carrying the raw body.
func maestroAgentFailure(statusCode int, responseBody []byte) error {
	var agentErr MaestroAgentError
	if err := json.Unmarshal(responseBody, &agentErr); err == nil && agentErr.ErrorCode != "" {
		// The body reports its own statusCode, but the transport's is authoritative.
		agentErr.StatusCode = statusCode
		return &agentErr
	}
	return fmt.Errorf("HTTP %d: %s", statusCode, string(responseBody))
}
