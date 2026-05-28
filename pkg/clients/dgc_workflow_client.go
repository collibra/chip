package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
)

// WorkflowDefinitionPagedResponse represents the response from the Collibra workflow definitions API.
type WorkflowDefinitionPagedResponse struct {
	Total   int64                `json:"total"`
	Offset  int64                `json:"offset"`
	Limit   int64                `json:"limit"`
	Results []WorkflowDefinition `json:"results"`
}

// WorkflowDefinition is the trimmed subset of WorkflowDefinitionImpl needed by callers
// who want to identify a workflow and decide whether it can be started.
type WorkflowDefinition struct {
	ID                        string `json:"id"`
	Name                      string `json:"name,omitempty"`
	Description               string `json:"description,omitempty"`
	ProcessID                 string `json:"processId,omitempty"`
	Enabled                   bool   `json:"enabled,omitempty"`
	FormRequired              bool   `json:"formRequired,omitempty"`
	BusinessItemDiscriminator string `json:"businessItemDiscriminator,omitempty"`
}

type WorkflowDefinitionsQueryParams struct {
	Name               string `url:"name,omitempty"`
	DefinitionIDPhrase string `url:"definitionIdPhrase,omitempty"`
	Description        string `url:"description,omitempty"`
	Enabled            *bool  `url:"enabled,omitempty"`
	Global             *bool  `url:"global,omitempty"`
	Limit              int    `url:"limit,omitempty"`
	Offset             int    `url:"offset,omitempty"`
}

func FindWorkflowDefinitions(ctx context.Context, collibraHttpClient *http.Client, params WorkflowDefinitionsQueryParams) (*WorkflowDefinitionPagedResponse, error) {
	slog.InfoContext(ctx, fmt.Sprintf("Finding workflow definitions: name=%q limit=%d offset=%d", params.Name, params.Limit, params.Offset))

	endpoint, err := buildUrl("/rest/2.0/workflowDefinitions", params)
	if err != nil {
		return nil, fmt.Errorf("failed to build endpoint: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	body, err := executeRequest(collibraHttpClient, req)
	if err != nil {
		return nil, err
	}

	return ParseWorkflowDefinitionsResponse(body)
}

func ParseWorkflowDefinitionsResponse(jsonData []byte) (*WorkflowDefinitionPagedResponse, error) {
	var response WorkflowDefinitionPagedResponse
	if err := json.Unmarshal(jsonData, &response); err != nil {
		return nil, fmt.Errorf("failed to parse workflow definitions response: %w", err)
	}
	return &response, nil
}

// StartFormData represents the form schema returned from the start form data endpoint.
type StartFormData struct {
	FormKey        string         `json:"formKey,omitempty"`
	ProcessID      string         `json:"processId,omitempty"`
	FormProperties []FormProperty `json:"formProperties"`
}

// FormProperty represents one input field on a workflow start form.
type FormProperty struct {
	ID                     string          `json:"id"`
	Name                   string          `json:"name,omitempty"`
	Type                   string          `json:"type,omitempty"`
	Required               bool            `json:"required,omitempty"`
	Writable               bool            `json:"writable,omitempty"`
	MultiValue             bool            `json:"multiValue,omitempty"`
	Value                  string          `json:"value,omitempty"`
	HelpText               string          `json:"helpText,omitempty"`
	ProposedFixed          bool            `json:"proposedFixed,omitempty"`
	EnumValues             []DropdownValue `json:"enumValues,omitempty"`
	ProposedDropdownValues []DropdownValue `json:"proposedDropdownValues,omitempty"`
	DefaultDropdownValues  []DropdownValue `json:"defaultDropdownValues,omitempty"`
}

// DropdownValue represents an option in an enum/dropdown form field.
type DropdownValue struct {
	ID         string `json:"id,omitempty"`
	IDAsString string `json:"idAsString,omitempty"`
	Text       string `json:"text,omitempty"`
}

// StartWorkflowInstancesRequest is the body for POST /workflowInstances.
type StartWorkflowInstancesRequest struct {
	WorkflowDefinitionID string            `json:"workflowDefinitionId"`
	BusinessItemIDs      []string          `json:"businessItemIds,omitempty"`
	BusinessItemType     string            `json:"businessItemType,omitempty"`
	FormProperties       map[string]string `json:"formProperties,omitempty"`
	SendNotification     bool              `json:"sendNotification,omitempty"`
}

// WorkflowInstance is the trimmed response shape of a started workflow instance.
type WorkflowInstance struct {
	ID                       string `json:"id"`
	Ended                    bool   `json:"ended,omitempty"`
	InError                  bool   `json:"inError,omitempty"`
	ErrorMessage             string `json:"errorMessage,omitempty"`
	StartDate                int64  `json:"startDate,omitempty"`
	CreatedAssetID           string `json:"createdAssetId,omitempty"`
	ParentWorkflowInstanceID string `json:"parentWorkflowInstanceId,omitempty"`
}

func GetWorkflowStartFormData(ctx context.Context, collibraHttpClient *http.Client, workflowDefinitionID string) (*StartFormData, error) {
	return getStartFormData(ctx, collibraHttpClient, workflowDefinitionID, "startFormData")
}

// GetWorkflowConfigurationStartFormData fetches the configuration-style start
// form, used by some Marketplace and Global-Create workflows that don't expose
// their fields through the task-style /startFormData endpoint.
func GetWorkflowConfigurationStartFormData(ctx context.Context, collibraHttpClient *http.Client, workflowDefinitionID string) (*StartFormData, error) {
	return getStartFormData(ctx, collibraHttpClient, workflowDefinitionID, "configurationStartFormData")
}

func getStartFormData(ctx context.Context, collibraHttpClient *http.Client, workflowDefinitionID, kind string) (*StartFormData, error) {
	slog.InfoContext(ctx, fmt.Sprintf("Fetching %s for workflow definition %s", kind, workflowDefinitionID))

	endpoint := fmt.Sprintf("/rest/2.0/workflowDefinitions/workflowDefinition/%s/%s", workflowDefinitionID, kind)
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	body, err := executeRequest(collibraHttpClient, req)
	if err != nil {
		return nil, err
	}

	var formData StartFormData
	if err := json.Unmarshal(body, &formData); err != nil {
		return nil, fmt.Errorf("failed to parse %s response: %w", kind, err)
	}
	return &formData, nil
}

func StartWorkflowInstances(ctx context.Context, collibraHttpClient *http.Client, request StartWorkflowInstancesRequest) ([]WorkflowInstance, error) {
	slog.InfoContext(ctx, fmt.Sprintf("Starting workflow instances for definition %s", request.WorkflowDefinitionID))

	jsonData, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal start workflow request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "/rest/2.0/workflowInstances", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	body, err := executeRequest(collibraHttpClient, req)
	if err != nil {
		// Surface the request body alongside the platform error so callers can
		// see exactly what was attempted (the platform's own error responses
		// for "workflowNotStarted" don't echo it back).
		return nil, fmt.Errorf("%w; request body was: %s", err, string(jsonData))
	}

	var instances []WorkflowInstance
	if err := json.Unmarshal(body, &instances); err != nil {
		return nil, fmt.Errorf("failed to parse start workflow response: %w", err)
	}
	return instances, nil
}
