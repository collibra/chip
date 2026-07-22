package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const catalogDatabaseBasePath = "/rest/catalogDatabase/v1"

// DatabaseConnection is a discovered connection to a specific database (catalog) on a
// data source, returned by GET /databaseConnections.
type DatabaseConnection struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	EdgeConnectionID string `json:"edgeConnectionId"`
	DatabaseID       string `json:"databaseId,omitempty"`
}

// SchemaConnection is a discovered connection to a specific schema within a database,
// returned by GET /schemaConnections.
type SchemaConnection struct {
	ID                   string `json:"id"`
	Name                 string `json:"name"`
	DatabaseConnectionID string `json:"databaseConnectionId"`
	SchemaID             string `json:"schemaId,omitempty"`
}

// Database is a registered Database asset, returned by POST/GET /databases.
type Database struct {
	ID                   string   `json:"id"`
	Name                 string   `json:"name"`
	Description          string   `json:"description,omitempty"`
	CommunityID          string   `json:"communityId"`
	OwnerIDs             []string `json:"ownerIds"`
	ParentSystemID       string   `json:"parentSystemId"`
	DatabaseConnectionID string   `json:"databaseConnectionId"`
	EdgeConnectionStatus string   `json:"edgeConnectionStatus,omitempty"`
}

// AddDatabaseRequest is the request body for POST /databases.
type AddDatabaseRequest struct {
	DatabaseConnectionID string   `json:"databaseConnectionId"`
	CommunityID          string   `json:"communityId"`
	ParentSystemID       string   `json:"parentSystemId"`
	OwnerIDs             []string `json:"ownerIds"`
	Description          string   `json:"description,omitempty"`
}

// MetadataSynchronizationRule defines which tables of a schema get synchronized and
// what additional metadata is captured for them.
type MetadataSynchronizationRule struct {
	Include                     string `json:"include,omitempty"`
	Exclude                     string `json:"exclude,omitempty"`
	TargetDomainID              string `json:"targetDomainId,omitempty"`
	SkipViews                   bool   `json:"skipViews,omitempty"`
	RegisterSourceTags          bool   `json:"registerSourceTags,omitempty"`
	IngestSemanticViews         bool   `json:"ingestSemanticViews,omitempty"`
	RegisterDataUsageStatistics bool   `json:"registerDataUsageStatistics,omitempty"`
}

// SchemaMetadataConfiguration links a schema connection to the synchronization rules
// that decide what gets ingested for it.
type SchemaMetadataConfiguration struct {
	SchemaConnectionID   string                        `json:"schemaConnectionId"`
	SynchronizationRules []MetadataSynchronizationRule `json:"synchronizationRules,omitempty"`
}

// CatalogJob represents an asynchronous job triggered by the database (catalog)
// registration API, e.g. POST /databases/{id}/synchronizeMetadata.
type CatalogJob struct {
	ID      string `json:"id"`
	State   string `json:"state,omitempty"`
	Result  string `json:"result,omitempty"`
	Message string `json:"message,omitempty"`
}

// DatabaseMetadataSynchronizationRequest is the request body for
// POST /databases/{databaseId}/synchronizeMetadata.
type DatabaseMetadataSynchronizationRequest struct {
	SchemaConnectionIDs []string `json:"schemaConnectionIds,omitempty"`
}

type pagedResponse[T any] struct {
	Results []T `json:"results"`
}

// RefreshDatabaseConnections asynchronously refreshes the database connections known
// for an Edge connection via POST /databaseConnections/refresh?edgeConnectionId=...
// The refresh runs in the background; call ListDatabaseConnections afterwards (with
// retries) to see newly discovered connections.
func RefreshDatabaseConnections(ctx context.Context, client *http.Client, edgeConnectionID string) error {
	endpoint := catalogDatabaseBasePath + "/databaseConnections/refresh?edgeConnectionId=" + edgeConnectionID
	return doCatalogDatabasePost(ctx, client, endpoint, nil, http.StatusAccepted)
}

// ListDatabaseConnections lists database connections discovered for an Edge connection
// via GET /databaseConnections?edgeConnectionId=...
func ListDatabaseConnections(ctx context.Context, client *http.Client, edgeConnectionID string) ([]DatabaseConnection, error) {
	endpoint := catalogDatabaseBasePath + "/databaseConnections?edgeConnectionId=" + edgeConnectionID
	var response pagedResponse[DatabaseConnection]
	if err := doCatalogDatabaseGet(ctx, client, endpoint, &response); err != nil {
		return nil, err
	}
	return response.Results, nil
}

// RegisterDatabase creates a Database asset from a discovered database connection via
// POST /databases.
func RegisterDatabase(ctx context.Context, client *http.Client, request AddDatabaseRequest) (*Database, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("registering database: marshaling request: %w", err)
	}
	var database Database
	if err := doCatalogDatabasePostJSON(ctx, client, catalogDatabaseBasePath+"/databases", body, http.StatusCreated, &database); err != nil {
		return nil, err
	}
	return &database, nil
}

// RefreshSchemaConnections asynchronously refreshes the schema connections known for a
// database connection via POST /schemaConnections/refresh?databaseConnectionId=...
func RefreshSchemaConnections(ctx context.Context, client *http.Client, databaseConnectionID string) error {
	endpoint := catalogDatabaseBasePath + "/schemaConnections/refresh?databaseConnectionId=" + databaseConnectionID
	return doCatalogDatabasePost(ctx, client, endpoint, nil, http.StatusAccepted)
}

// ListSchemaConnections lists schema connections discovered for a database connection
// via GET /schemaConnections?databaseConnectionId=...
func ListSchemaConnections(ctx context.Context, client *http.Client, databaseConnectionID string) ([]SchemaConnection, error) {
	endpoint := catalogDatabaseBasePath + "/schemaConnections?databaseConnectionId=" + databaseConnectionID
	var response pagedResponse[SchemaConnection]
	if err := doCatalogDatabaseGet(ctx, client, endpoint, &response); err != nil {
		return nil, err
	}
	return response.Results, nil
}

// SetSchemaMetadataConfigurationsBatch defines synchronization rules for one or more
// schema connections via POST /schemaMetadataConfigurations/batch.
func SetSchemaMetadataConfigurationsBatch(ctx context.Context, client *http.Client, configurations []SchemaMetadataConfiguration) ([]SchemaMetadataConfiguration, error) {
	body, err := json.Marshal(configurations)
	if err != nil {
		return nil, fmt.Errorf("setting schema metadata configurations: marshaling request: %w", err)
	}
	var result []SchemaMetadataConfiguration
	if err := doCatalogDatabasePostJSON(ctx, client, catalogDatabaseBasePath+"/schemaMetadataConfigurations/batch", body, http.StatusCreated, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// SynchronizeDatabaseMetadata starts the jdbc-ingestion capability run for a registered
// Database asset via POST /databases/{databaseId}/synchronizeMetadata. An empty
// schemaConnectionIds list synchronizes all schemas that already have a
// SchemaMetadataConfiguration.
func SynchronizeDatabaseMetadata(ctx context.Context, client *http.Client, databaseID string, schemaConnectionIDs []string) (*CatalogJob, error) {
	body, err := json.Marshal(DatabaseMetadataSynchronizationRequest{SchemaConnectionIDs: schemaConnectionIDs})
	if err != nil {
		return nil, fmt.Errorf("starting ingestion: marshaling request: %w", err)
	}
	endpoint := catalogDatabaseBasePath + "/databases/" + databaseID + "/synchronizeMetadata"
	var job CatalogJob
	if err := doCatalogDatabasePostJSON(ctx, client, endpoint, body, http.StatusAccepted, &job); err != nil {
		return nil, err
	}
	return &job, nil
}

func doCatalogDatabaseGet(ctx context.Context, client *http.Client, endpoint string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("calling %s: building request: %w", endpoint, err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("calling %s: sending request: %w", endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("calling %s: reading response: %w", endpoint, err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("calling %s: unexpected status %d: %s", endpoint, resp.StatusCode, string(body))
	}

	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("calling %s: decoding response: %w", endpoint, err)
	}

	return nil
}

func doCatalogDatabasePost(ctx context.Context, client *http.Client, endpoint string, body []byte, expectedStatus int) error {
	return doCatalogDatabasePostJSON(ctx, client, endpoint, body, expectedStatus, nil)
}

func doCatalogDatabasePostJSON(ctx context.Context, client *http.Client, endpoint string, body []byte, expectedStatus int, out any) error {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, reader)
	if err != nil {
		return fmt.Errorf("calling %s: building request: %w", endpoint, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("calling %s: sending request: %w", endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("calling %s: reading response: %w", endpoint, err)
	}

	if resp.StatusCode != expectedStatus {
		switch resp.StatusCode {
		case http.StatusConflict:
			return fmt.Errorf("calling %s: conflict, an operation may already be in progress: %s", endpoint, string(respBody))
		default:
			return fmt.Errorf("calling %s: unexpected status %d: %s", endpoint, resp.StatusCode, string(respBody))
		}
	}

	if out == nil || len(respBody) == 0 {
		return nil
	}

	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("calling %s: decoding response: %w", endpoint, err)
	}

	return nil
}
