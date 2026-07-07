package catalog_generic_update_schedule

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
	IngestibleId   string `json:"ingestibleId" jsonschema:"the UUID of the GENERIC integration instance"`
	CronExpression string `json:"cronExpression" jsonschema:"the updated cron expression (e.g. '0 3 * * *')"`
	CronTimeZone   string `json:"cronTimeZone" jsonschema:"the updated time zone for the cron schedule"`
	CronJson       string `json:"cronJson,omitempty" jsonschema:"optional JSON representation of the cron schedule"`
	Workflow       string `json:"workflow,omitempty" jsonschema:"optional workflow name"`
}

type Output struct {
	Schedule *clients.GenericSchedule `json:"schedule,omitempty" jsonschema:"the updated schedule"`
	Success  bool                     `json:"success" jsonschema:"whether the schedule was updated successfully"`
	Error    string                   `json:"error,omitempty" jsonschema:"error message if the update failed"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "catalog_generic_update_schedule",
		Description: "Updates the synchronization schedule for a GENERIC integration instance. Creates the schedule if it does not exist.",
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
		schedule, err := clients.UpdateGenericSchedule(ctx, collibraClient, input.IngestibleId, input.Workflow, clients.ChangeGenericScheduleRequest{
			CronExpression: input.CronExpression,
			CronTimeZone:   input.CronTimeZone,
			CronJson:       input.CronJson,
		})
		if err != nil {
			return Output{Success: false, Error: fmt.Sprintf("failed to update schedule: %s", err.Error())}, nil
		}
		return Output{Schedule: schedule, Success: true}, nil
	}
}
