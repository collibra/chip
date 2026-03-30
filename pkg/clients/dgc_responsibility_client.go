package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
)

const errFailedToCreateRequest = "failed to create request: %w"

// Responsibility represents a single responsibility assignment for an asset.
type Responsibility struct {
	ID           string        `json:"id"`
	Role         *ResourceRole `json:"role,omitempty"`
	Owner        *ResourceRef  `json:"owner,omitempty"`
	BaseResource *ResourceRef  `json:"baseResource,omitempty"`
	System       bool          `json:"system"`
}

// ResourceRole represents the role in a responsibility (e.g., Owner, Steward).
type ResourceRole struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ResourceRef represents a reference to a resource (user, group, community, etc.) in the API.
type ResourceRef struct {
	ID                    string `json:"id"`
	ResourceDiscriminator string `json:"resourceDiscriminator"`
}

// ResponsibilityPagedResponse represents the paginated response from the responsibilities API.
type ResponsibilityPagedResponse struct {
	Total   int64            `json:"total"`
	Offset  int64            `json:"offset"`
	Limit   int64            `json:"limit"`
	Results []Responsibility `json:"results"`
}

// ResponsibilityQueryParams defines the query parameters for the responsibilities API.
type ResponsibilityQueryParams struct {
	ResourceIDs      string `url:"resourceIds,omitempty"`
	IncludeInherited bool   `url:"includeInherited,omitempty"`
	Limit            int    `url:"limit,omitempty"`
	Offset           int    `url:"offset,omitempty"`
}

// UserResponse represents the response from the /rest/2.0/users/{userId} endpoint.
type UserResponse struct {
	ID        string `json:"id"`
	UserName  string `json:"userName"`
	FirstName string `json:"firstName,omitempty"`
	LastName  string `json:"lastName,omitempty"`
}

// UserGroupResponse represents the response from the /rest/2.0/userGroups/{groupId} endpoint.
type UserGroupResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// GetResponsibilities fetches all responsibilities for the given asset ID, including inherited ones.
func GetResponsibilities(ctx context.Context, collibraHttpClient *http.Client, assetID string) ([]Responsibility, error) {
	slog.InfoContext(ctx, fmt.Sprintf("Fetching responsibilities for asset: %s", assetID))

	params := ResponsibilityQueryParams{
		ResourceIDs:      assetID,
		IncludeInherited: true,
		Limit:            100,
		Offset:           0,
	}

	endpoint, err := buildUrl("/rest/2.0/responsibilities", params)
	if err != nil {
		return nil, fmt.Errorf("failed to build endpoint: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf(errFailedToCreateRequest, err)
	}

	body, err := executeRequest(collibraHttpClient, req)
	if err != nil {
		return nil, err
	}

	var response ResponsibilityPagedResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse responsibilities response: %w", err)
	}

	return response.Results, nil
}

// GetUserName fetches the display name for a user by ID.
func GetUserName(ctx context.Context, collibraHttpClient *http.Client, userID string) (string, error) {
	endpoint := fmt.Sprintf("/rest/2.0/users/%s", userID)

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return "", fmt.Errorf(errFailedToCreateRequest, err)
	}

	body, err := executeRequest(collibraHttpClient, req)
	if err != nil {
		return "", err
	}

	var user UserResponse
	if err := json.Unmarshal(body, &user); err != nil {
		return "", fmt.Errorf("failed to parse user response: %w", err)
	}

	if user.FirstName != "" || user.LastName != "" {
		return fmt.Sprintf("%s %s (%s)", user.FirstName, user.LastName, user.UserName), nil
	}
	return user.UserName, nil
}

// GetUserGroupName fetches the name for a user group by ID.
func GetUserGroupName(ctx context.Context, collibraHttpClient *http.Client, groupID string) (string, error) {
	endpoint := fmt.Sprintf("/rest/2.0/userGroups/%s", groupID)

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return "", fmt.Errorf(errFailedToCreateRequest, err)
	}

	body, err := executeRequest(collibraHttpClient, req)
	if err != nil {
		return "", err
	}

	var group UserGroupResponse
	if err := json.Unmarshal(body, &group); err != nil {
		return "", fmt.Errorf("failed to parse user group response: %w", err)
	}

	return group.Name, nil
}
