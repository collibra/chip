package catalog_generic_get_schedule

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
}

type Output struct {
	Schedule *clients.GenericSchedule `json:"schedule,omitempty" jsonschema:"the integration schedule"`
	Found    bool                     `json:"found" jsonschema:"whether a schedule was found"`
	Error    string                   `json:"error,omitempty" jsonschema:"error message if retrieval failed"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "catalog_generic_get_schedule",
		Description: "Retrieves the synchronization schedule for a GENERIC integration instance. Returns the cron expression, time zone, and timing of the last/next run.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if err := validation.UUID("ingestibleId", input.IngestibleId); err != nil {
			return Output{}, err
		}
		schedule, err := clients.GetGenericSchedule(ctx, collibraClient, input.IngestibleId)
		if err != nil {
			return Output{Found: false, Error: fmt.Sprintf("failed to get schedule: %s", err.Error())}, nil
		}
		return Output{Schedule: schedule, Found: true}, nil
	}
}
