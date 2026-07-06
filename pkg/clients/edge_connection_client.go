package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
)

// Connection is the response shape returned by the Edge connection management API.
type Connection struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	TypeID      string         `json:"typeId,omitempty"`
	EdgeSiteID  string         `json:"edgeSiteId"`
	VaultID     string         `json:"vaultId,omitempty"`
	Parameters  map[string]any `json:"parameters"`
}

// ConnectionRequest is the request body for creating or updating a connection via
// POST /edge/api/rest/v2/connections or PUT /edge/api/rest/v2/connections/{id}.
type ConnectionRequest struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	TypeID      string         `json:"typeId"`
	EdgeSiteID  string         `json:"edgeSiteId"`
	VaultID     string         `json:"vaultId,omitempty"`
	Parameters  map[string]any `json:"parameters"`
}

// CreateConnection creates a new Edge connection via POST /edge/api/rest/v2/connections.
// The server assigns the connection id.
func CreateConnection(ctx context.Context, client *http.Client, request ConnectionRequest) (*Connection, error) {
	return doConnectionRequest(ctx, client, http.MethodPost, "/edge/api/rest/v2/connections", request)
}

// CreateOrUpdateConnection creates or updates an Edge connection with a known id via
// PUT /edge/api/rest/v2/connections/{connectionId}.
func CreateOrUpdateConnection(ctx context.Context, client *http.Client, connectionID string, request ConnectionRequest) (*Connection, error) {
	endpoint := "/edge/api/rest/v2/connections/" + connectionID
	return doConnectionRequest(ctx, client, http.MethodPut, endpoint, request)
}

// ConnectionFindRequest is the request body for POST /edge/api/rest/v2/connections/find.
type ConnectionFindRequest struct {
	EdgeSiteID    string `json:"edgeSiteId,omitempty"`
	Name          string `json:"name,omitempty"`
	NameMatchMode string `json:"nameMatchMode,omitempty"`
}

// FindConnections searches Edge connections via POST /edge/api/rest/v2/connections/find.
// Useful for picking up a connection a user created manually (e.g. via the DGC/Edge UI,
// for a driver file too large to pass through this tool) by name, instead of needing
// its id.
func FindConnections(ctx context.Context, client *http.Client, request ConnectionFindRequest) ([]Connection, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("finding connections: marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/edge/api/rest/v2/connections/find", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("finding connections: building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("finding connections: sending request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("finding connections: reading response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("finding connections: unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	var result []Connection
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("finding connections: decoding response: %w", err)
	}
	return result, nil
}

func doConnectionRequest(ctx context.Context, client *http.Client, method, endpoint string, request ConnectionRequest) (*Connection, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("saving connection: marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("saving connection: building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("saving connection: sending request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("saving connection: reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		switch resp.StatusCode {
		case http.StatusBadRequest:
			return nil, fmt.Errorf("saving connection: bad request (invalid parameters or typeId): %s", string(respBody))
		case http.StatusNotFound:
			return nil, fmt.Errorf("saving connection: edge site not found: %s", string(respBody))
		default:
			return nil, fmt.Errorf("saving connection: unexpected status %d: %s", resp.StatusCode, string(respBody))
		}
	}

	var result Connection
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("saving connection: decoding response: %w", err)
	}

	return &result, nil
}

// UploadFile uploads an arbitrary file via POST /edge/api/rest/v2/upload and returns
// the artifact URI (e.g. "jar://<uuid>/<filename>") to reference from any FILE-type
// connection or capability parameter — confirmed (via ConnectionPropertyDeserializer)
// that the parameter value is the bare artifact URI string, not a wrapped object; this
// covers JDBC driver jars as well as certs, keytabs, private keys, etc.
func UploadFile(ctx context.Context, client *http.Client, filename string, content []byte) (string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return "", fmt.Errorf("uploading file: creating form file: %w", err)
	}
	if _, err := part.Write(content); err != nil {
		return "", fmt.Errorf("uploading file: writing content: %w", err)
	}
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("uploading file: closing multipart writer: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/edge/api/rest/v2/upload", &body)
	if err != nil {
		return "", fmt.Errorf("uploading file: building request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	// This endpoint responds with produces=TEXT_PLAIN regardless of the request's
	// Accept header's preference — asking for application/json gets a 406.
	req.Header.Set("Accept", "text/plain")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("uploading file: sending request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("uploading file: reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("uploading file: unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	// The upload endpoint returns a plain-text artifact URI, but tolerate a
	// JSON-quoted string too in case that ever changes.
	var uri string
	if err := json.Unmarshal(respBody, &uri); err != nil {
		uri = string(bytes.Trim(respBody, "\" \n\t"))
	}
	if uri == "" {
		return "", fmt.Errorf("uploading file: empty artifact URI in response: %s", string(respBody))
	}

	return uri, nil
}
