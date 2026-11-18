package clients

// The public DGC REST endpoints that change users and user groups. DGC's errors come back as DGC
// words them (executeCollibraRequest), so a duplicate group name or an unknown user id reads as it
// does in DGC.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// AddUserGroup calls POST /rest/2.0/userGroups.
func AddUserGroup(ctx context.Context, client *http.Client, name string) (*DirectoryUserGroup, error) {
	var group DirectoryUserGroup
	err := sendDirectoryJSON(ctx, client, http.MethodPost, "/rest/2.0/userGroups", map[string]any{"name": name}, &group)
	return &group, err
}

// ChangeUserGroup calls PATCH /rest/2.0/userGroups/{id}: it renames the group.
func ChangeUserGroup(ctx context.Context, client *http.Client, groupID, name string) (*DirectoryUserGroup, error) {
	var group DirectoryUserGroup
	err := sendDirectoryJSON(ctx, client, http.MethodPatch, "/rest/2.0/userGroups/"+url.PathEscape(groupID), map[string]any{"name": name}, &group)
	return &group, err
}

// RemoveUserGroup calls DELETE /rest/2.0/userGroups/{id}.
func RemoveUserGroup(ctx context.Context, client *http.Client, groupID string) error {
	return sendDirectoryJSON(ctx, client, http.MethodDelete, "/rest/2.0/userGroups/"+url.PathEscape(groupID), nil, nil)
}

// AddUsersToUserGroup calls POST /rest/2.0/userGroups/{id}/users and returns the users DGC answers with.
func AddUsersToUserGroup(ctx context.Context, client *http.Client, groupID string, userIDs []string) ([]DirectoryUser, error) {
	var users []DirectoryUser
	err := sendDirectoryJSON(ctx, client, http.MethodPost, "/rest/2.0/userGroups/"+url.PathEscape(groupID)+"/users", map[string]any{"userIds": userIDs}, &users)
	return users, err
}

// RemoveUsersFromUserGroup calls DELETE /rest/2.0/userGroups/{id}/users with the user ids in the body.
func RemoveUsersFromUserGroup(ctx context.Context, client *http.Client, groupID string, userIDs []string) error {
	return sendDirectoryJSON(ctx, client, http.MethodDelete, "/rest/2.0/userGroups/"+url.PathEscape(groupID)+"/users", map[string]any{"userIds": userIDs}, nil)
}

// AddUserRequest carries what a new user needs; the empty fields are not sent.
type AddUserRequest struct {
	UserName     string
	EmailAddress string
	FirstName    string
	LastName     string
	Title        string
	Department   string
	Language     string
	UserGroupIDs []string
}

// AddUser calls POST /rest/2.0/users. The user gets no password: it is set through DGC's own reset flow.
func AddUser(ctx context.Context, client *http.Client, request AddUserRequest) (*DirectoryUser, error) {
	body := map[string]any{}
	putString(body, "userName", request.UserName)
	putString(body, "emailAddress", request.EmailAddress)
	putString(body, "firstName", request.FirstName)
	putString(body, "lastName", request.LastName)
	putString(body, "title", request.Title)
	putString(body, "department", request.Department)
	putString(body, "language", request.Language)
	if len(request.UserGroupIDs) > 0 {
		body["userGroupIds"] = request.UserGroupIDs
	}
	var user DirectoryUser
	err := sendDirectoryJSON(ctx, client, http.MethodPost, "/rest/2.0/users", body, &user)
	return &user, err
}

// ChangeUserRequest carries the properties to change: a nil or empty field is left as it is.
type ChangeUserRequest struct {
	UserName     string
	EmailAddress string
	FirstName    string
	LastName     string
	Title        string
	Department   string
	Language     string
	Enabled      *bool
}

// ChangeUser calls PATCH /rest/2.0/users/{id}. Note that DGC names the email property "email" here and
// "emailAddress" when a user is added.
func ChangeUser(ctx context.Context, client *http.Client, userID string, request ChangeUserRequest) (*DirectoryUser, error) {
	body := map[string]any{}
	putString(body, "username", request.UserName)
	putString(body, "email", request.EmailAddress)
	putString(body, "firstName", request.FirstName)
	putString(body, "lastName", request.LastName)
	putString(body, "title", request.Title)
	putString(body, "department", request.Department)
	putString(body, "language", request.Language)
	if request.Enabled != nil {
		body["enabled"] = *request.Enabled
	}
	var user DirectoryUser
	err := sendDirectoryJSON(ctx, client, http.MethodPatch, "/rest/2.0/users/"+url.PathEscape(userID), body, &user)
	return &user, err
}

func putString(body map[string]any, key, value string) {
	if value != "" {
		body[key] = value
	}
}

// sendDirectoryJSON sends the request with a JSON body, when there is one, and reads the answer into
// out, when there is an answer to read.
func sendDirectoryJSON(ctx context.Context, client *http.Client, method, path string, body any, out any) error {
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to encode the body of %s %s: %w", method, path, err)
		}
		payload = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, path, payload)
	if err != nil {
		return fmt.Errorf("failed to build %s %s: %w", method, path, err)
	}
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("Accept", "application/json")
	response, err := executeCollibraRequest(client, request)
	if err != nil {
		return err
	}
	if out == nil || len(bytes.TrimSpace(response)) == 0 {
		return nil
	}
	if err := json.Unmarshal(response, out); err != nil {
		return fmt.Errorf("failed to parse the response of %s %s: %w", method, path, err)
	}
	return nil
}
