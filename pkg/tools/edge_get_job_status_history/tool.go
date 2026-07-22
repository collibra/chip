package edge_get_job_status_history

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
	Id string `json:"id" jsonschema:"the UUID of the Edge job to get status history for"`
}

type Output struct {
	History []clients.JobStatusLog `json:"history" jsonschema:"all job status updates in LIFO order (most recent first)"`
	Count   int                    `json:"count" jsonschema:"number of status entries returned"`
	Error   string                 `json:"error,omitempty" jsonschema:"error message if retrieval failed"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "edge_get_job_status_history",
		Description: "Gets all status updates for an Edge job in reverse chronological order (most recent first). Useful for diagnosing job failures.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if err := validation.UUID("id", input.Id); err != nil {
			return Output{}, err
		}
		history, err := clients.GetEdgeJobStatusHistory(ctx, collibraClient, input.Id)
		if err != nil {
			return Output{History: []clients.JobStatusLog{}, Error: fmt.Sprintf("failed to get job status history: %s", err.Error())}, nil
		}
		if history == nil {
			history = []clients.JobStatusLog{}
		}
		return Output{History: history, Count: len(history)}, nil
	}
}
