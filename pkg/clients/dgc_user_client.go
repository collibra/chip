package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// User is a DGC user, as returned by GET /rest/2.0/users and
// GET /rest/2.0/users/{id}.
type User struct {
	ID           string `json:"id"`
	UserName     string `json:"userName,omitempty"`
	FirstName    string `json:"firstName,omitempty"`
	LastName     string `json:"lastName,omitempty"`
	EmailAddress string `json:"emailAddress,omitempty"`
	Title        string `json:"title,omitempty"`
	Department   string `json:"department,omitempty"`
	Enabled      bool   `json:"enabled"`
}

// UserPagedResponse is the response shape for GET /rest/2.0/users.
type UserPagedResponse struct {
	Total   int64  `json:"total"`
	Offset  int64  `json:"offset"`
	Limit   int64  `json:"limit"`
	Results []User `json:"results"`
}

// FindUsersQueryParams are the supported query parameters for GET /rest/2.0/users.
// Name matches case-insensitively as a substring, by default across username,
// first name, last name, and first+last/last+first combinations.
type FindUsersQueryParams struct {
	Name            string `url:"name,omitempty"`
	Limit           int    `url:"limit,omitempty"`
	Offset          int    `url:"offset,omitempty"`
	IncludeDisabled bool   `url:"includeDisabled,omitempty"`
}

// FindUsers searches DGC users via GET /rest/2.0/users.
func FindUsers(ctx context.Context, client *http.Client, params FindUsersQueryParams) (*UserPagedResponse, error) {
	endpoint, err := buildUrl("/rest/2.0/users", params)
	if err != nil {
		return nil, fmt.Errorf("finding users: building endpoint: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("finding users: building request: %w", err)
	}

	body, err := executeRequest(client, req)
	if err != nil {
		return nil, fmt.Errorf("finding users: %w", err)
	}

	var result UserPagedResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("finding users: decoding response: %w", err)
	}
	return &result, nil
}

// GetUser fetches a single DGC user by id via GET /rest/2.0/users/{id}.
func GetUser(ctx context.Context, client *http.Client, userID string) (*User, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/rest/2.0/users/"+userID, nil)
	if err != nil {
		return nil, fmt.Errorf("getting user %q: building request: %w", userID, err)
	}

	body, err := executeRequest(client, req)
	if err != nil {
		return nil, fmt.Errorf("getting user %q: %w", userID, err)
	}

	var result User
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("getting user %q: decoding response: %w", userID, err)
	}
	return &result, nil
}
