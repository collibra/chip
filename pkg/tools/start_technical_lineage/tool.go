// Package start_technical_lineage implements the start_technical_lineage MCP tool:
// triggers the technical lineage harvest for a registered Database asset.
package start_technical_lineage

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
	AssetID string `json:"assetId" jsonschema:"UUID of the Database asset to harvest technical lineage for — the asset registered by jdbc-ingestion (configure_database), NOT the capability id. A Technical Lineage capability referencing the database's connection must already exist on the edge site."`
}

type Output struct {
	Success bool   `json:"success" jsonschema:"Whether the harvest was submitted. Submission only — the harvest runs asynchronously and no job id is returned; locate the spawned DGC job with jobs_find and poll it with get_job_status."`
	Error   string `json:"error,omitempty" jsonschema:"Error message if submitting the harvest failed."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "start_technical_lineage",
		Title:       "Start Technical Lineage Harvest",
		Description: "Triggers the technical lineage harvest for a registered Database asset. The database must already be registered (configure_database) and ingested (start_ingestion), and a Technical Lineage capability must exist on the edge site. This is the only correct trigger for technical lineage — never use edge_run_capability or catalog_etl_start_job for it. A success response only means the harvest was submitted (the endpoint returns no job id): locate the spawned DGC job with jobs_find and poll it with get_job_status (NOT edge_get_job_status — the Edge-side harvest is tracked through the DGC job).",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{DestructiveHint: chip.Ptr(true)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if err := validation.UUID("assetId", input.AssetID); err != nil {
			return Output{}, err
		}

		if err := clients.StartTechnicalLineageHarvest(ctx, collibraClient, input.AssetID); err != nil {
			return Output{Success: false, Error: fmt.Sprintf("failed to start technical lineage harvest: %s", err.Error())}, nil
		}

		return Output{Success: true}, nil
	}
}
