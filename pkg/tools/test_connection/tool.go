// Package test_connection implements the test_connection MCP tool: verifies an Edge
// connection can actually reach its data source.
package test_connection

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
	ConnectionID string `json:"connectionId" jsonschema:"UUID of the connection to test (created via edge_create_connection)."`
	TimeoutSec   int    `json:"timeoutSec,omitempty" jsonschema:"Optional. If provided and greater than 0, waits synchronously for the test to finish (up to this many seconds) and returns the final result. If omitted, returns immediately with a jobId while the test runs in the background — poll it with edge_get_job_status."`
}

type Output struct {
	JobID   string `json:"jobId,omitempty" jsonschema:"The id of the connection-test job. Use with edge_get_job_status to poll for the result if timeoutSec was not provided."`
	Success bool   `json:"success" jsonschema:"Whether the test connection request itself succeeded. If timeoutSec was omitted, this reflects that the test job was submitted, not the connection test's outcome — check edge_get_job_status for that."`
	Message string `json:"message,omitempty" jsonschema:"Status message from the test."`
	Error   string `json:"error,omitempty" jsonschema:"Error message if the test request itself failed."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "test_connection",
		Title:       "Test Edge Connection",
		Description: "Tests whether an Edge connection can reach its data source. Without timeoutSec, returns immediately with a jobId to poll via edge_get_job_status; with timeoutSec, waits up to that many seconds and returns the final result.",
		Handler:     handler(collibraClient),
		Permissions: []string{"dgc.edge-integration-capability-manage"},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if err := validation.UUID("connectionId", input.ConnectionID); err != nil {
			return Output{}, err
		}

		var timeoutSec *int
		if input.TimeoutSec > 0 {
			timeoutSec = &input.TimeoutSec
		}

		result, err := clients.TestConnection(ctx, collibraClient, input.ConnectionID, timeoutSec)
		if err != nil {
			return Output{Success: false, Error: fmt.Sprintf("failed to test connection: %s", err.Error())}, nil
		}

		return Output{JobID: result.JobID, Success: result.Success, Message: result.Message}, nil
	}
}
