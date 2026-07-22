// Package start_ingestion implements the start_ingestion MCP tool: triggers the
// jdbc-ingestion capability run for a registered Database asset.
package start_ingestion

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
	DatabaseID          string   `json:"databaseId" jsonschema:"UUID of the Database asset to synchronize (as registered by register_database)."`
	SchemaConnectionIDs []string `json:"schemaConnectionIds,omitempty" jsonschema:"Optional. UUIDs of specific schema connections to synchronize. If omitted, all schemas that already have synchronization rules configured (via configure_database_schemas) are synchronized."`
}

type Output struct {
	Job     *clients.CatalogJob `json:"job,omitempty" jsonschema:"The started synchronization job. Poll its id with get_job_status (NOT edge_get_job_status, which is for Edge-site jobs) to see when it completes."`
	Success bool                `json:"success" jsonschema:"Whether the ingestion job was started."`
	Error   string              `json:"error,omitempty" jsonschema:"Error message if starting the job failed."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "start_ingestion",
		Title:       "Start Database Ingestion",
		Description: "Triggers the jdbc-ingestion capability run for a registered Database asset, synchronizing its configured schemas/tables into the catalog. The database must already be registered (register_database) and have its schemas configured (configure_database_schemas). Poll the returned job's id with get_job_status to confirm it actually completed — a 202/success response here only means the job was accepted, not that ingestion finished.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{DestructiveHint: chip.Ptr(true)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if err := validation.UUID("databaseId", input.DatabaseID); err != nil {
			return Output{}, err
		}
		if err := validation.UUIDs("schemaConnectionIds", input.SchemaConnectionIDs); err != nil {
			return Output{}, err
		}

		job, err := clients.SynchronizeDatabaseMetadata(ctx, collibraClient, input.DatabaseID, input.SchemaConnectionIDs)
		if err != nil {
			return Output{Success: false, Error: fmt.Sprintf("failed to start ingestion: %s", err.Error())}, nil
		}

		return Output{Job: job, Success: true}, nil
	}
}
