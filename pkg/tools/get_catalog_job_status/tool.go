// Package get_catalog_job_status implements the get_catalog_job_status MCP tool:
// polls the status of a DGC catalog job, e.g. the one returned by start_ingestion.
// This is distinct from get_job_status, which polls Edge-site jobs (capability runs,
// connection tests) — the two job id spaces are not interchangeable.
package get_catalog_job_status

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/validation"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct {
	JobID string `json:"jobId" jsonschema:"UUID of the DGC catalog job to check, as returned by start_ingestion. Do not pass an Edge job id here (e.g. from test_connection) — use get_job_status for those instead."`
}

type Output struct {
	Name               string  `json:"name,omitempty" jsonschema:"The job's name."`
	Type               string  `json:"type,omitempty" jsonschema:"The job's type (e.g. 'DELTA_INGESTION')."`
	State              string  `json:"state,omitempty" jsonschema:"The job's current state: WAITING, RUNNING, CANCELING, COMPLETED, CANCELED, or ERROR."`
	Result             string  `json:"result,omitempty" jsonschema:"The job's result once finished: NOT_SET (not finished yet), SUCCESS, COMPLETED_WITH_ERROR, FAILURE, or ABORTED."`
	Message            string  `json:"message,omitempty" jsonschema:"Status message from the job."`
	ProgressPercentage float64 `json:"progressPercentage,omitempty" jsonschema:"Progress percentage, 0-100."`
	StartDate          string  `json:"startDate,omitempty"`
	EndDate            string  `json:"endDate,omitempty" jsonschema:"When the job finished, if it has."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "get_catalog_job_status",
		Title:       "Get Catalog Job Status",
		Description: "Gets the current status of a DGC catalog job, e.g. the ingestion job started by start_ingestion. For Edge-site jobs (test_connection, capability runs), use get_job_status instead — the two job id spaces are separate and not interchangeable.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if err := validation.UUID("jobId", input.JobID); err != nil {
			return Output{}, err
		}

		job, err := clients.GetCatalogJob(ctx, collibraClient, input.JobID)
		if err != nil {
			return Output{}, err
		}

		return Output{
			Name:               job.Name,
			Type:               job.Type,
			State:              job.State,
			Result:             job.Result,
			Message:            job.Message,
			ProgressPercentage: job.ProgressPercentage,
			StartDate:          job.StartDate,
			EndDate:            job.EndDate,
		}, nil
	}
}
