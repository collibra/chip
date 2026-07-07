package catalog_generic_cancel_job

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
	IngestibleId string `json:"ingestibleId" jsonschema:"the UUID of the GENERIC integration instance"`
	Workflow     string `json:"workflow,omitempty" jsonschema:"optional workflow name"`
}

type Output struct {
	Success bool   `json:"success" jsonschema:"whether the job was cancelled"`
	Error   string `json:"error,omitempty" jsonschema:"error message if cancellation failed"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "catalog_generic_cancel_job",
		Description: "Cancels the currently running synchronization job for a GENERIC integration instance.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{DestructiveHint: chip.Ptr(true), IdempotentHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if err := validation.UUID("ingestibleId", input.IngestibleId); err != nil {
			return Output{}, err
		}
		if err := clients.CancelGenericJob(ctx, collibraClient, input.IngestibleId, input.Workflow); err != nil {
			return Output{Success: false, Error: fmt.Sprintf("failed to cancel job: %s", err.Error())}, nil
		}
		return Output{Success: true}, nil
	}
}
