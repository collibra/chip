// Package get_registered_database implements the get_registered_database MCP tool:
// resolves a registered Database asset to the Edge connection (and edge site) behind it.
package get_registered_database

import (
	"context"
	"fmt"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/validation"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct {
	AssetID string `json:"assetId" jsonschema:"UUID of the registered Database asset (as created by register_database)."`
}

type Output struct {
	Database       *clients.Database       `json:"database,omitempty" jsonschema:"The registered database."`
	EdgeConnection *clients.EdgeConnection `json:"edgeConnection,omitempty" jsonschema:"The Edge connection backing the registered database. Its id is the value for a capability's 'connection' parameter; its edgeSiteId is the edge site where dependent capabilities (e.g. Technical Lineage) must be created."`
	Success        bool                    `json:"success" jsonschema:"Whether the database was resolved to its Edge connection."`
	Error          string                  `json:"error,omitempty" jsonschema:"Error message if resolving failed."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "get_registered_database",
		Title:       "Get Registered Database",
		Description: "Resolves a registered Database asset to the Edge connection behind it (database → database connection → Edge connection), returning the connection and its edgeSiteId. Use it to reuse the ingestion's connection when setting up dependent capabilities such as Technical Lineage — the capability must be created on the returned edgeSiteId. Fails if the asset is not a database registered via register_database.",
		Handler:     handler(collibraClient),
		Permissions: []string{"dgc.edge-view-connections-and-capabilities"},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if err := validation.UUID("assetId", input.AssetID); err != nil {
			return Output{}, err
		}

		database, err := clients.GetDatabase(ctx, collibraClient, input.AssetID)
		if err != nil {
			return Output{Success: false, Error: fmt.Sprintf("failed to get registered database (is this the UUID of a Database asset registered via register_database?): %s", err.Error())}, nil
		}
		if database.DatabaseConnectionID == "" {
			return Output{Database: database, Success: false, Error: "registered database has no database connection"}, nil
		}

		databaseConnection, err := clients.GetDatabaseConnection(ctx, collibraClient, database.DatabaseConnectionID)
		if err != nil {
			return Output{Database: database, Success: false, Error: fmt.Sprintf("failed to get database connection %s: %s", database.DatabaseConnectionID, err.Error())}, nil
		}
		if databaseConnection.EdgeConnectionID == "" {
			return Output{Database: database, Success: false, Error: "database connection has no Edge connection"}, nil
		}

		edgeConnection, err := clients.GetConnection(ctx, collibraClient, databaseConnection.EdgeConnectionID)
		if err != nil {
			return Output{Database: database, Success: false, Error: fmt.Sprintf("failed to get Edge connection %s: %s", databaseConnection.EdgeConnectionID, err.Error())}, nil
		}

		return Output{Database: database, EdgeConnection: edgeConnection, Success: true}, nil
	}
}
