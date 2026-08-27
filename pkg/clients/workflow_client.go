package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// WorkflowDefinition mirrors the subset of dgc-core's WorkflowDefinition (the response shape of
// GET /rest/2.0/workflowDefinitions and GET /rest/2.0/workflowDefinitions/{workflowDefinitionId})
// this tool needs. Unknown fields on the real response (stopRoles, assetAssignmentRules, ...) are
// ignored on unmarshal.
type WorkflowDefinition struct {
	ID                       string          `json:"id"`
	Name                     string          `json:"name"`
	Description              string          `json:"description,omitempty"` // human-authored explanation of what the workflow does — the main signal an agent has to pick the right one among many similarly-named options
	ProcessID                string          `json:"processId"`
	Enabled                  bool            `json:"enabled"`
	FormRequired             bool            `json:"formRequired"`
	BusinessItemResourceType string          `json:"businessItemResourceType"` // ASSET | DOMAIN | COMMUNITY | GLOBAL
	RegisteredUserAccessible bool            `json:"registeredUserAccessible"` // any authenticated user may start it, regardless of startRoles
	GuestUserAccessible      bool            `json:"guestUserAccessible"`
	StartRoles               []EditAssetRole `json:"startRoles,omitempty"`
}

type workflowDefinitionsPage struct {
	Results []WorkflowDefinition `json:"results"`
}

// GetWorkflowDefinition fetches a workflow definition by its UUID — works for both OOTB and
// custom workflows. Returns the HTTP status code so callers can distinguish "no such workflow on
// this instance" (404) from other failures.
func GetWorkflowDefinition(ctx context.Context, client *http.Client, workflowDefinitionID string) (*WorkflowDefinition, int, error) {
	endpoint := "/rest/2.0/workflowDefinitions/" + url.PathEscape(workflowDefinitionID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}
	body, code, err := executeCollibraRequestWithStatus(client, req)
	if err != nil {
		return nil, code, err
	}
	var def WorkflowDefinition
	if jsonErr := json.Unmarshal(body, &def); jsonErr != nil {
		return nil, code, fmt.Errorf("failed to parse workflow definition: %w", jsonErr)
	}
	return &def, code, nil
}

// ListWorkflowDefinitionsForAsset lists the workflow definitions (OOTB or custom) Collibra
// considers applicable to the given asset (matched server-side against the asset's type and
// status via assetAssignmentRules). NOTE: businessItemResourceType is NOT a working query filter
// on this endpoint — confirmed live: passing it returns the full unfiltered list. Only
// asset-scoped discovery is supported today; domain/community-scoped discovery would need the
// equivalent domainIds/communityIds query params, not yet wired up here.
func ListWorkflowDefinitionsForAsset(ctx context.Context, client *http.Client, assetID string) ([]WorkflowDefinition, error) {
	endpoint, err := buildUrl("/rest/2.0/workflowDefinitions", struct {
		AssetIDs []string `url:"assetIds,omitempty"`
		Limit    int      `url:"limit,omitempty"`
	}{AssetIDs: []string{assetID}, Limit: 1000})
	if err != nil {
		return nil, fmt.Errorf("failed to build endpoint: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	body, err := executeCollibraRequest(client, req)
	if err != nil {
		return nil, err
	}
	var page workflowDefinitionsPage
	if jsonErr := json.Unmarshal(body, &page); jsonErr != nil {
		return nil, fmt.Errorf("failed to parse workflow definitions: %w", jsonErr)
	}
	return page.Results, nil
}

// maxListedWorkflowDefinitions caps how many raw definitions ListWorkflowDefinitions fetches
// before authorization filtering. There is no server-side "enabled" or "startable" filter, so this
// bounds the worst case (a large instance can have hundreds of definitions).
const maxListedWorkflowDefinitions = 500

// ListWorkflowDefinitions lists workflow definitions instance-wide (no asset/domain/community
// scoping), for discovery when the caller has no specific resource in mind. Safe to expose broadly
// only because the caller is expected to run each result through IsAuthorizedToStart before
// showing it — this call itself returns every definition (client-filtered to `enabled`), not just
// ones the current user can start.
func ListWorkflowDefinitions(ctx context.Context, client *http.Client) ([]WorkflowDefinition, error) {
	endpoint, err := buildUrl("/rest/2.0/workflowDefinitions", struct {
		Limit int `url:"limit,omitempty"`
	}{Limit: maxListedWorkflowDefinitions})
	if err != nil {
		return nil, fmt.Errorf("failed to build endpoint: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	body, err := executeCollibraRequest(client, req)
	if err != nil {
		return nil, err
	}
	var page workflowDefinitionsPage
	if jsonErr := json.Unmarshal(body, &page); jsonErr != nil {
		return nil, fmt.Errorf("failed to parse workflow definitions: %w", jsonErr)
	}
	enabled := make([]WorkflowDefinition, 0, len(page.Results))
	for _, def := range page.Results {
		if def.Enabled {
			enabled = append(enabled, def)
		}
	}
	return enabled, nil
}

// WorkflowFormPropertyOption is one selectable value for a dropdown-typed start-form field.
type WorkflowFormPropertyOption struct {
	Text string `json:"text"`
}

// WorkflowFormProperty is one field of a workflow's start form.
type WorkflowFormProperty struct {
	ID       string                       `json:"id"`
	Name     string                       `json:"name"`
	Type     string                       `json:"type"`
	Required bool                         `json:"required"`
	Options  []WorkflowFormPropertyOption `json:"proposedDropdownValues,omitempty"`
}

// WorkflowStartFormData is the start-form schema for a workflow definition.
type WorkflowStartFormData struct {
	FormProperties []WorkflowFormProperty `json:"formProperties"`
}

// GetWorkflowStartFormData fetches the typed start-form schema for a workflow definition — which
// fields it needs, whether each is required, and (for dropdown-typed fields) the allowed values.
// The path has a doubled "workflowDefinition(s)" segment; this is the real, confirmed-live path,
// not a typo: /rest/2.0/workflowDefinitions/workflowDefinition/{id}/startFormData.
func GetWorkflowStartFormData(ctx context.Context, client *http.Client, workflowDefinitionID string) (*WorkflowStartFormData, error) {
	endpoint := "/rest/2.0/workflowDefinitions/workflowDefinition/" + url.PathEscape(workflowDefinitionID) + "/startFormData"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	body, err := executeCollibraRequest(client, req)
	if err != nil {
		return nil, err
	}
	var form WorkflowStartFormData
	if jsonErr := json.Unmarshal(body, &form); jsonErr != nil {
		return nil, fmt.Errorf("failed to parse start form data: %w", jsonErr)
	}
	return &form, nil
}

// PermWorkflowStart is the global permission Collibra requires to start ANY workflow — confirmed
// against dgc-core's own StartPermissionIntegrationTest: it is mandatory in every case, on top of
// which either registeredUserAccessible or holding one of the definition's startRoles is needed.
const PermWorkflowStart = "WORKFLOW_START"

// IsAuthorizedToStart reports whether the current user (authenticated as client) can start def,
// optionally scoped to businessItemID (empty for a GLOBAL workflow). Implements the verified
// algorithm: PermWorkflowStart is always required; given that, registeredUserAccessible alone
// suffices, otherwise the user must hold one of def.StartRoles (globally, or scoped to
// businessItemID).
//
// uncertain=true means a lookup failed — the caller should NOT hide the workflow in that case
// (role inheritance down the community/domain/asset hierarchy is not modeled here, and a failed
// lookup must never look identical to "confirmed not allowed"); only the PermWorkflowStart check
// is treated as a reliable, hard "no".
func IsAuthorizedToStart(ctx context.Context, client *http.Client, def *WorkflowDefinition, businessItemID string) (authorized bool, uncertain bool, err error) {
	globalPerms, permErr := GetCurrentUserGlobalPermissions(ctx, client)
	if permErr != nil {
		return true, true, permErr
	}
	if !HasPermission(globalPerms, PermWorkflowStart) {
		return false, false, nil
	}
	if def.RegisteredUserAccessible {
		return true, false, nil
	}
	if len(def.StartRoles) == 0 {
		// No startRoles configured and registeredUserAccessible is false: nobody but an admin can
		// start it through the normal path. Treat as uncertain rather than a hard "no" — this
		// combination is unusual enough that guessing wrong in either direction is plausible.
		return false, true, nil
	}
	user, userErr := GetCurrentUser(ctx, client)
	if userErr != nil {
		return true, true, userErr
	}
	resps, respErr := GetUserResponsibilities(ctx, client, user.ID)
	if respErr != nil {
		return true, true, respErr
	}
	startRoleIDs := make(map[string]bool, len(def.StartRoles))
	for _, r := range def.StartRoles {
		startRoleIDs[r.ID] = true
	}
	for _, resp := range resps {
		if resp.Role == nil || !startRoleIDs[resp.Role.ID] {
			continue
		}
		if resp.BaseResource == nil {
			return true, false, nil // held globally
		}
		if businessItemID != "" && resp.BaseResource.ID == businessItemID {
			return true, false, nil // held directly on the target resource
		}
	}
	// No direct-held startRole found. This is NOT treated as a definitive "no": Collibra also
	// grants roles by inheritance down the community/domain/asset hierarchy, which this lookup
	// does not walk, so a false negative here is a real possibility, not just theoretical.
	return false, true, nil
}

// StartWorkflowInstanceRequest is the body for POST /rest/2.0/workflowInstances (see dgc-api's
// StartWorkflowInstancesRequest.java). businessItemType/businessItemIds are omitted for a GLOBAL
// workflow, which has no associated resource.
type StartWorkflowInstanceRequest struct {
	WorkflowDefinitionID string            `json:"workflowDefinitionId"`
	BusinessItemIDs      []string          `json:"businessItemIds,omitempty"`
	BusinessItemType     string            `json:"businessItemType,omitempty"` // ASSET | DOMAIN | COMMUNITY | GLOBAL
	FormProperties       map[string]string `json:"formProperties,omitempty"`
}

// WorkflowInstance mirrors the subset of dgc-core's WorkflowInstance this tool needs.
type WorkflowInstance struct {
	ID string `json:"id"`
}

// StartWorkflowInstance starts a workflow instance. POST /rest/2.0/workflowInstances returns 201
// with a JSON array of started instances (one per business item); this tool only ever starts one
// instance per call, so the first entry is returned.
func StartWorkflowInstance(ctx context.Context, client *http.Client, reqBody StartWorkflowInstanceRequest) (*WorkflowInstance, int, error) {
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/rest/2.0/workflowInstances", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	body, code, err := executeCollibraRequestWithStatus(client, req)
	if err != nil {
		return nil, code, err
	}
	var instances []WorkflowInstance
	if jsonErr := json.Unmarshal(body, &instances); jsonErr != nil {
		return nil, code, fmt.Errorf("failed to parse workflow instances: %w", jsonErr)
	}
	if len(instances) == 0 {
		return nil, code, fmt.Errorf("workflow start returned no instances")
	}
	return &instances[0], code, nil
}
