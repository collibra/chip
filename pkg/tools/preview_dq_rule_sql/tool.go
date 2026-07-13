// Package preview_dq_rule_sql implements the preview_dq_rule_sql MCP tool: it
// runs a rule's SQL against the source and returns the composed query's sample
// result set, so the rule's behavior can be inspected before it is saved or run.
package preview_dq_rule_sql

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// OutputStatus is the overall outcome of a preview_dq_rule_sql call.
type OutputStatus string

const (
	// StatusSuccess means the preview result set was returned.
	StatusSuccess OutputStatus = "success"
	// StatusValidationError means the inputs failed validation before any call.
	StatusValidationError OutputStatus = "validation_error"
	// StatusError means the preview could not run due to a downstream error.
	StatusError OutputStatus = "error"
)

// Input is the tool's typed input. edgeSiteId/connectionId/schemaName are the
// discovery fields from prepare_create_dq_job.
type Input struct {
	EdgeSiteID   string `json:"edgeSiteId" jsonschema:"Required. Edge site UUID. From prepare_create_dq_job resolved.edgeSiteId."`
	ConnectionID string `json:"connectionId" jsonschema:"Required. Edge connection UUID. From prepare_create_dq_job resolved.connectionId."`
	SchemaName   string `json:"schemaName" jsonschema:"Required. Schema name the rule runs against."`
	JobName      string `json:"jobName" jsonschema:"Required. Name of the data quality job (dataset) the rule belongs to."`
	PreviewRule  string `json:"previewRule" jsonschema:"Required. The rule SQL to preview (the same value you would pass as monitorValue)."`
	FilterQuery  string `json:"filterQuery,omitempty" jsonschema:"Optional. Additional WHERE-clause filter applied to the rule."`
	RowLimit     int    `json:"rowLimit,omitempty" jsonschema:"Optional. Max number of sample rows to return. Defaults to 0 (service default)."`
}

// Column describes one column in the preview result set.
type Column struct {
	Name     string `json:"name" jsonschema:"Column name."`
	Type     string `json:"type,omitempty" jsonschema:"Source column type."`
	Position int    `json:"position" jsonschema:"Column position in the result set."`
}

// Output is the typed response.
type Output struct {
	Status  OutputStatus `json:"status" jsonschema:"'success' when the preview ran; 'validation_error' for bad inputs; 'error' for downstream DQ failures."`
	Message string       `json:"message" jsonschema:"Human-readable summary."`
	Columns []Column     `json:"columns,omitempty" jsonschema:"Columns returned by the rule's SQL."`
	Rows    [][]string   `json:"rows,omitempty" jsonschema:"Sample rows (each row is a list of column values, column order matches columns)."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "preview_dq_rule_sql",
		Title: "Preview Data Quality Rule SQL",
		Description: "Run a data quality rule's SQL against the source and return the composed query's sample result set " +
			"(columns and sample rows), so the rule's behavior can be inspected before saving or running it. " +
			"Requires edgeSiteId/connectionId/schemaName (from prepare_create_dq_job).",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if out := validate(input); out != nil {
			return *out, nil
		}

		data, err := clients.PreviewDQRuleSQL(ctx, collibraClient, clients.PreviewRuleRequest{
			EdgeSiteID:   strings.TrimSpace(input.EdgeSiteID),
			ConnectionID: strings.TrimSpace(input.ConnectionID),
			SchemaName:   strings.TrimSpace(input.SchemaName),
			JobName:      strings.TrimSpace(input.JobName),
			PreviewRule:  input.PreviewRule,
			FilterQuery:  input.FilterQuery,
			RowLimit:     input.RowLimit,
		})
		if err != nil {
			return Output{Status: StatusError, Message: fmt.Sprintf("Could not preview rule SQL: %v", err)}, nil
		}

		cols := make([]Column, 0, len(data.Columns))
		for _, c := range data.Columns {
			cols = append(cols, Column{Name: c.ColumnName, Type: c.ColumnType, Position: c.Position})
		}
		rows := make([][]string, 0, len(data.Data))
		for _, r := range data.Data {
			rows = append(rows, r.Values)
		}

		return Output{
			Status:  StatusSuccess,
			Message: fmt.Sprintf("Preview returned %d column(s) and %d row(s).", len(cols), len(rows)),
			Columns: cols,
			Rows:    rows,
		}, nil
	}
}

func validate(input Input) *Output {
	required := []struct {
		name string
		val  string
	}{
		{"edgeSiteId", input.EdgeSiteID},
		{"connectionId", input.ConnectionID},
		{"schemaName", input.SchemaName},
		{"jobName", input.JobName},
		{"previewRule", input.PreviewRule},
	}
	for _, f := range required {
		if strings.TrimSpace(f.val) == "" {
			return &Output{Status: StatusValidationError, Message: f.name + " is required."}
		}
	}
	return nil
}
