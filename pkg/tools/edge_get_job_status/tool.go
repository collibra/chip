// Package edge_get_job_status implements the edge_get_job_status MCP tool: polls the status of
// an Edge job, e.g. one started by test_connection or start_ingestion.
package edge_get_job_status

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/validation"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct {
	JobID string `json:"jobId" jsonschema:"UUID of the job to check, as returned by test_connection or start_ingestion."`
}

type Output struct {
	Status              string `json:"status,omitempty" jsonschema:"The job's current status: UNKNOWN, SUBMITTED, DOWNLOADING, SCHEDULED, RUNNING, SUCCEEDED, FAILED, CANCELLED, CANCELLING, CAPABILITY_PROGRESS, CAPABILITY_SUCCEEDED, or CAPABILITY_FAILED. SUCCEEDED/FAILED/CANCELLED/CAPABILITY_SUCCEEDED/CAPABILITY_FAILED are terminal."`
	Message             string `json:"message,omitempty" jsonschema:"Status message from the job."`
	LastUpdatedDateTime string `json:"lastUpdatedDateTime,omitempty" jsonschema:"When the status was last updated."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "edge_get_job_status",
		Title:       "Get Edge Job Status",
		Description: "Gets the current status of an Edge job, e.g. one started by test_connection or start_ingestion.",
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

		status, err := clients.GetJobStatusLog(ctx, collibraClient, input.JobID)
		if err != nil {
			return Output{}, err
		}

		return Output{Status: status.Status, Message: status.Message, LastUpdatedDateTime: status.LastUpdatedDateTime}, nil
	}
}
