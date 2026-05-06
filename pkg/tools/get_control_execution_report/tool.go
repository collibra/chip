package get_control_execution_report

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct {
	DateFrom       string `json:"dateFrom" jsonschema:"Required. Start of the analytics window in YYYY-MM-DD."`
	DateTo         string `json:"dateTo" jsonschema:"Required. End of the analytics window in YYYY-MM-DD (inclusive)."`
	Granularity    string `json:"granularity" jsonschema:"Required. Time bucket: 'Week' for 1-3 month windows, 'Month' for 3-18 month windows."`
	Severity       string `json:"severity,omitempty" jsonschema:"Optional. Filter to a single severity (e.g. 'Critical', 'High')."`
	ControlType    string `json:"controlType,omitempty" jsonschema:"Optional. Filter to a single control type (e.g. 'Detective')."`
	Tag            string `json:"tag,omitempty" jsonschema:"Optional. Filter to controls carrying a specific tag."`
	OrganizationID string `json:"organizationId,omitempty" jsonschema:"Optional. Filter to controls owned by a specific organization (UUID)."`
	FailureLength  *int   `json:"failureLength,omitempty" jsonschema:"Optional. Filter to controls failing for at least this many consecutive runs."`
}

type Output struct {
	Result map[string]any `json:"result" jsonschema:"Raw response from POST /rest/controlExecution/v1/report. Contains time-bucketed run counts and pass/fail breakdowns; the shape is preserved verbatim so downstream consumers don't lose fields."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "get_control_execution_report",
		Description: "Fetch a Control Tower analytics report for a date window. Wraps POST /rest/controlExecution/v1/report; returns time-series counts grouped by week or month. Use for snapshot/trend questions (month-over-month regressions, pass-rate over time, severity mix).",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if input.DateFrom == "" || input.DateTo == "" || input.Granularity == "" {
			return Output{}, fmt.Errorf("dateFrom, dateTo, and granularity are required")
		}
		body := map[string]any{
			"dateFrom":    input.DateFrom,
			"dateTo":      input.DateTo,
			"granularity": input.Granularity,
		}
		if input.Severity != "" {
			body["severity"] = input.Severity
		}
		if input.ControlType != "" {
			body["controlType"] = input.ControlType
		}
		if input.Tag != "" {
			body["tag"] = input.Tag
		}
		if input.OrganizationID != "" {
			body["organizationId"] = input.OrganizationID
		}
		if input.FailureLength != nil {
			body["failureLength"] = *input.FailureLength
		}
		raw, err := clients.ControlExecutionReport(ctx, collibraClient, body)
		if err != nil {
			return Output{}, err
		}
		var result map[string]any
		if err := json.Unmarshal(raw, &result); err != nil {
			return Output{}, fmt.Errorf("failed to decode report response: %w", err)
		}
		return Output{Result: result}, nil
	}
}
