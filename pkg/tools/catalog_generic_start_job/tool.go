package catalog_generic_start_job

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
	IngestibleId        string `json:"ingestibleId" jsonschema:"the UUID of the GENERIC integration instance"`
	CloudIngestionJobId string `json:"cloudIngestionJobId,omitempty" jsonschema:"optional cloud ingestion job ID to resume"`
	Workflow            string `json:"workflow,omitempty" jsonschema:"optional workflow name"`
	RuntimeArguments    string `json:"runtimeArguments,omitempty" jsonschema:"optional runtime arguments as a string"`
}

type Output struct {
	Job     *clients.GenericJob `json:"job,omitempty" jsonschema:"the started job details"`
	Success bool                `json:"success" jsonschema:"whether the job was started"`
	Error   string              `json:"error,omitempty" jsonschema:"error message if starting failed"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "catalog_generic_start_job",
		Description: "Starts a synchronization job for a GENERIC integration instance.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{DestructiveHint: chip.Ptr(true)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if err := validation.UUID("ingestibleId", input.IngestibleId); err != nil {
			return Output{}, err
		}
		job, err := clients.StartGenericJob(ctx, collibraClient, input.IngestibleId, input.CloudIngestionJobId, input.Workflow, input.RuntimeArguments)
		if err != nil {
			return Output{Success: false, Error: fmt.Sprintf("failed to start job: %s", err.Error())}, nil
		}
		return Output{Job: job, Success: true}, nil
	}
}
