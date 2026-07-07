package edge_run_capability

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
	Id              string         `json:"id" jsonschema:"the UUID of the capability to run"`
	JobId           string         `json:"jobId,omitempty" jsonschema:"optional UUID for tracking the job execution"`
	InFastNamespace bool           `json:"inFastNamespace,omitempty" jsonschema:"if true, runs in the fast (priority) namespace"`
	Parameters      map[string]any `json:"parameters,omitempty" jsonschema:"optional run-time parameters for this execution only"`
	WorkflowName    string         `json:"workflowName,omitempty" jsonschema:"optional workflow name; defaults to workflow.yaml inside the capability package"`
}

type Output struct {
	JobId   string `json:"jobId,omitempty" jsonschema:"UUID of the job submitted to Edge"`
	Success bool   `json:"success" jsonschema:"whether the run was submitted successfully"`
	Error   string `json:"error,omitempty" jsonschema:"error message if submission failed"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "edge_run_capability",
		Description: "Runs an Edge capability immediately. Returns the job UUID that can be used to track execution via edge_get_job_status.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{DestructiveHint: chip.Ptr(true)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if err := validation.UUID("id", input.Id); err != nil {
			return Output{}, err
		}
		if err := validation.UUIDOptional("jobId", input.JobId); err != nil {
			return Output{}, err
		}
		jobId, err := clients.RunCapability(ctx, collibraClient, input.Id, clients.CapabilityRunRequest{
			JobId:           input.JobId,
			InFastNamespace: input.InFastNamespace,
			Parameters:      input.Parameters,
			WorkflowName:    input.WorkflowName,
		})
		if err != nil {
			return Output{Success: false, Error: fmt.Sprintf("failed to run capability: %s", err.Error())}, nil
		}
		return Output{JobId: jobId, Success: true}, nil
	}
}
