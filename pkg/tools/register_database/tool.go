// Package register_database implements the register_database MCP tool: discovers a
// database through an Edge connection and registers it as a DGC Database asset, via
// the public catalogDatabase API.
//
// This is the first of two steps in setting up a database for jdbc-ingestion — the
// second, configuring which schemas/tables get synchronized, is a separate tool
// (configure_database_schemas), which takes this tool's returned databaseConnectionId. They
// were originally one tool; split apart because registering the database is a
// one-time, id-claiming operation (POST /databases permanently claims the underlying
// database connection — a second call for the same one fails with
// databaseConnectionAlreadyUsed) while schema configuration is something an agent may
// reasonably want to redo independently, and keeping them separate means a
// schema-selection ambiguity error here never risks colliding with a database this
// tool already registered.
package register_database

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
	// pollAttempts * pollInterval bounds the wait for the async refresh. A
	// first-time connection to a data source can take 30-60s to establish, so
	// this budget needs enough headroom to cover that cold-start case.
	pollAttempts = 14
	pollInterval = 5 * time.Second
)

type Input struct {
	EdgeConnectionID string   `json:"edgeConnectionId" jsonschema:"UUID of the Edge connection (created via edge_create_connection) to discover a database on."`
	DatabaseName     string   `json:"databaseName,omitempty" jsonschema:"Optional. Exact name of the database (catalog) to register, as it appears at the data source. Required if the data source exposes more than one database/catalog through this connection; if there is exactly one, it is selected automatically."`
	CommunityID      string   `json:"communityId" jsonschema:"UUID of the community the Database asset (and its automatically created domain) will be created in."`
	ParentSystemID   string   `json:"parentSystemId" jsonschema:"UUID of the parent System asset the Database asset will be linked to."`
	OwnerIDs         []string `json:"ownerIds" jsonschema:"UUIDs of the users to assign as owners of the Database asset. Use find_users to resolve a name (e.g. 'Admin') to its UUID."`
	Description      string   `json:"description,omitempty" jsonschema:"Optional description of the Database asset."`
}

type Output struct {
	Database *clients.Database `json:"database,omitempty" jsonschema:"The registered Database asset. Pass its databaseConnectionId to configure_database_schemas next."`
	Success  bool              `json:"success" jsonschema:"Whether the database was registered successfully."`
	Error    string            `json:"error,omitempty" jsonschema:"Error message if registration failed or is still in progress (safe to retry)."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "register_database",
		Title:       "Register Database for Ingestion",
		Description: "Discovers a database through an Edge connection and registers it as a Database asset. Prerequisite for configure_database_schemas (which sets up schema/table synchronization) and, transitively, start_ingestion. Assumes the target community and parent System asset already exist (create_community/create_domain/create_asset), and that a jdbc-ingestion capability referencing this connection has already been created via edge_create_capability — discovery only finds data because that capability actually performs the crawl; without one, this fails with a discovery-timeout-shaped error even though the real cause is the missing capability. If more than one database is discovered, this returns an error naming the candidates instead of guessing — confirm which one with the user.",
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
		if len(input.OwnerIDs) == 0 {
			return Output{}, fmt.Errorf("ownerIds must contain at least one user UUID — use find_users to resolve a name to an id")
		}
		if err := validation.UUIDs("ownerIds", input.OwnerIDs); err != nil {
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

		return Output{Database: database, Success: true}, nil
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
		return nil, fmt.Errorf("no database connections were discovered for edge connection %s after %d attempts. Either the refresh is still in progress (retry this tool call), or — more commonly — no capability referencing this connection exists yet: the discovery/refresh only finds data because a jdbc-ingestion capability actually performs the crawl. Verify a capability exists for this connection (edge_create_capability) before retrying", edgeConnectionID, pollAttempts)
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
