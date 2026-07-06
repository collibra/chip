// Package configure_database implements the configure_database MCP tool: registers a
// database discovered through an Edge connection as a Database asset and configures
// which schemas/tables get synchronized, via the public catalogDatabase API.
//
// This wraps a 4-step flow (refresh database connections, register the database,
// refresh schema connections, set synchronization rules) as a single tool call. The
// refresh steps are asynchronous on the server, so this tool polls for their result
// with a bounded number of attempts; if a stage hasn't completed within that budget,
// it returns an error asking the agent to retry the tool call (the refresh calls are
// idempotent to re-issue).
package configure_database

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/validation"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Sleep is used for polling backoff. Overridable in tests to avoid slow test runs.
var Sleep = time.Sleep

const (
	// pollAttempts * pollInterval bounds the wait for each async refresh. A
	// first-time connection to a data source can take 30-60s to establish, so
	// this budget needs enough headroom to cover that cold-start case.
	pollAttempts = 14
	pollInterval = 5 * time.Second
)

type Input struct {
	EdgeConnectionID string   `json:"edgeConnectionId" jsonschema:"UUID of the Edge connection (created via create_connection) to discover a database on."`
	DatabaseName     string   `json:"databaseName,omitempty" jsonschema:"Optional. Exact name of the database (catalog) to register, as it appears at the data source. Required if the data source exposes more than one database/catalog through this connection; if there is exactly one, it is selected automatically."`
	CommunityID      string   `json:"communityId" jsonschema:"UUID of the community the Database asset (and its automatically created domain) will be created in."`
	ParentSystemID   string   `json:"parentSystemId" jsonschema:"UUID of the parent System asset the Database asset will be linked to."`
	OwnerIDs         []string `json:"ownerIds" jsonschema:"UUIDs of the users to assign as owners of the Database asset."`
	Description      string   `json:"description,omitempty" jsonschema:"Optional description of the Database asset."`

	Include        string `json:"include,omitempty" jsonschema:"Optional. Comma-separated table name pattern to synchronize, '*' wildcard supported. Defaults to '*' (all tables)."`
	Exclude        string `json:"exclude,omitempty" jsonschema:"Optional. Comma-separated table name pattern to exclude from synchronization."`
	TargetDomainID string `json:"targetDomainId,omitempty" jsonschema:"Optional. UUID of a domain to create synchronized assets in. If omitted, an automatically created domain per schema is used."`
	SkipViews      bool   `json:"skipViews,omitempty" jsonschema:"Optional. If true, database views are excluded from synchronization."`
}

type Output struct {
	Database          *clients.Database          `json:"database,omitempty" jsonschema:"The registered Database asset."`
	SchemaConnections []clients.SchemaConnection `json:"schemaConnections,omitempty" jsonschema:"The schema connections discovered under the database, now configured for synchronization."`
	Success           bool                       `json:"success" jsonschema:"Whether the database was fully configured."`
	Error             string                     `json:"error,omitempty" jsonschema:"Error message if configuration failed or is still in progress (safe to retry)."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "configure_database",
		Title:       "Configure Database for Ingestion",
		Description: "Discovers a database through an Edge connection, registers it as a Database asset, and configures which tables get synchronized. Prerequisite for start_ingestion. Assumes the target community, and parent System asset already exist.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{DestructiveHint: chip.Ptr(true)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if err := validation.UUID("edgeConnectionId", input.EdgeConnectionID); err != nil {
			return Output{}, err
		}
		if err := validation.UUID("communityId", input.CommunityID); err != nil {
			return Output{}, err
		}
		if err := validation.UUID("parentSystemId", input.ParentSystemID); err != nil {
			return Output{}, err
		}
		if err := validation.UUIDs("ownerIds", input.OwnerIDs); err != nil {
			return Output{}, err
		}
		if err := validation.UUIDOptional("targetDomainId", input.TargetDomainID); err != nil {
			return Output{}, err
		}

		databaseConnection, err := discoverDatabaseConnection(ctx, collibraClient, input.EdgeConnectionID, input.DatabaseName)
		if err != nil {
			return Output{Success: false, Error: err.Error()}, nil
		}

		database, err := clients.RegisterDatabase(ctx, collibraClient, clients.AddDatabaseRequest{
			DatabaseConnectionID: databaseConnection.ID,
			CommunityID:          input.CommunityID,
			ParentSystemID:       input.ParentSystemID,
			OwnerIDs:             input.OwnerIDs,
			Description:          input.Description,
		})
		if err != nil {
			return Output{Success: false, Error: fmt.Sprintf("failed to register database: %s", err.Error())}, nil
		}

		schemaConnections, err := discoverSchemaConnections(ctx, collibraClient, databaseConnection.ID)
		if err != nil {
			return Output{Database: database, Success: false, Error: err.Error()}, nil
		}

		include := input.Include
		if include == "" {
			include = "*"
		}

		configurations := make([]clients.SchemaMetadataConfiguration, len(schemaConnections))
		for i, schemaConnection := range schemaConnections {
			configurations[i] = clients.SchemaMetadataConfiguration{
				SchemaConnectionID: schemaConnection.ID,
				SynchronizationRules: []clients.MetadataSynchronizationRule{
					{
						Include:        include,
						Exclude:        input.Exclude,
						TargetDomainID: input.TargetDomainID,
						SkipViews:      input.SkipViews,
					},
				},
			}
		}

		if _, err := clients.SetSchemaMetadataConfigurationsBatch(ctx, collibraClient, configurations); err != nil {
			return Output{Database: database, SchemaConnections: schemaConnections, Success: false,
				Error: fmt.Sprintf("failed to set schema synchronization rules: %s", err.Error())}, nil
		}

		return Output{Database: database, SchemaConnections: schemaConnections, Success: true}, nil
	}
}

// discoverDatabaseConnection triggers a refresh of database connections for the given
// Edge connection and polls until at least one appears, then selects one by name (or
// the only one, if there's no ambiguity).
func discoverDatabaseConnection(ctx context.Context, client *http.Client, edgeConnectionID, databaseName string) (*clients.DatabaseConnection, error) {
	if err := clients.RefreshDatabaseConnections(ctx, client, edgeConnectionID); err != nil {
		return nil, fmt.Errorf("failed to refresh database connections: %w", err)
	}

	var connections []clients.DatabaseConnection
	for attempt := 0; attempt < pollAttempts; attempt++ {
		if attempt > 0 {
			Sleep(pollInterval)
		}
		found, err := clients.ListDatabaseConnections(ctx, client, edgeConnectionID)
		if err != nil {
			return nil, fmt.Errorf("failed to list database connections: %w", err)
		}
		if len(found) > 0 {
			connections = found
			break
		}
	}

	if len(connections) == 0 {
		return nil, fmt.Errorf("no database connections were discovered for edge connection %s after %d attempts; the refresh may still be in progress — retry this tool call", edgeConnectionID, pollAttempts)
	}

	if databaseName != "" {
		for _, connection := range connections {
			if connection.Name == databaseName {
				return &connection, nil
			}
		}
		return nil, fmt.Errorf("no database named %q found among discovered database connections for edge connection %s", databaseName, edgeConnectionID)
	}

	if len(connections) > 1 {
		return nil, fmt.Errorf("multiple databases discovered for edge connection %s; specify databaseName to select one", edgeConnectionID)
	}

	return &connections[0], nil
}

// discoverSchemaConnections triggers a refresh of schema connections for the given
// database connection and polls until at least one appears.
func discoverSchemaConnections(ctx context.Context, client *http.Client, databaseConnectionID string) ([]clients.SchemaConnection, error) {
	if err := clients.RefreshSchemaConnections(ctx, client, databaseConnectionID); err != nil {
		return nil, fmt.Errorf("failed to refresh schema connections: %w", err)
	}

	for attempt := 0; attempt < pollAttempts; attempt++ {
		if attempt > 0 {
			Sleep(pollInterval)
		}
		found, err := clients.ListSchemaConnections(ctx, client, databaseConnectionID)
		if err != nil {
			return nil, fmt.Errorf("failed to list schema connections: %w", err)
		}
		if len(found) > 0 {
			return found, nil
		}
	}

	return nil, fmt.Errorf("no schemas were discovered for database connection %s after %d attempts; the refresh may still be in progress — retry this tool call", databaseConnectionID, pollAttempts)
}
