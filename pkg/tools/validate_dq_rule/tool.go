// Package validate_dq_rule implements the validate_dq_rule MCP tool: it checks
// that a rule's SQL/definition is valid before it is saved or run, so a bad rule
// is caught up front rather than at run time.
package validate_dq_rule

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// OutputStatus is the overall outcome of a validate_dq_rule call.
type OutputStatus string

const (
	// StatusSuccess means validation ran (see Valid for the verdict).
	StatusSuccess OutputStatus = "success"
	// StatusValidationError means the inputs failed validation before any call.
	StatusValidationError OutputStatus = "validation_error"
	// StatusError means validation could not run due to a downstream error.
	StatusError OutputStatus = "error"
)

// Input is the tool's typed input. edgeSiteId/connectionId/schemaName are the
// discovery fields from prepare_create_dq_job.
type Input struct {
	EdgeSiteID   string `json:"edgeSiteId" jsonschema:"Required. UUID of the Collibra Edge runtime/site that reaches the source database. From prepare_create_data_quality_job resolved.edgeSiteId."`
	ConnectionID string `json:"connectionId" jsonschema:"Required. UUID of the specific database connection on that Edge site. From prepare_create_data_quality_job resolved.connectionId."`
	SchemaName   string `json:"schemaName" jsonschema:"Required. Schema name the rule runs against."`
	JobName      string `json:"jobName" jsonschema:"Required. Name of the data quality job the rule belongs to (a job, also called a 'dataset', is a saved check on one database table)."`
	PreviewRule  string `json:"previewRule" jsonschema:"Required. The rule SQL to validate (the same value you would pass as monitorValue)."`
	FilterQuery  string `json:"filterQuery,omitempty" jsonschema:"Optional. Additional WHERE-clause filter applied to the rule."`
	RowLimit     int    `json:"rowLimit,omitempty" jsonschema:"Optional. Row limit for the underlying probe query. Defaults to 0 (service default)."`
}

// Output is the typed response.
type Output struct {
	Status  OutputStatus `json:"status" jsonschema:"'success' when validation ran; 'validation_error' for bad inputs; 'error' for downstream DQ failures."`
	Message string       `json:"message" jsonschema:"Human-readable summary, including the validation message from the DQ engine."`
	Valid   bool         `json:"valid" jsonschema:"True when the rule is valid; false when the DQ engine rejected it (see message)."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "validate_data_quality_rule",
		Title: "Validate Data Quality Rule",
		Description: "Validate a data quality rule's (a check on a table's data; Collibra calls it a 'monitor') SQL/definition against the source database before saving or running it, " +
			"so a malformed rule is caught up front. Returns whether the rule is valid plus the engine's validation message. " +
			"Requires edgeSiteId, connectionId and schemaName — the connection to the source database (edgeSiteId = the Collibra Edge runtime/site that reaches the source, connectionId = the specific database connection), all from prepare_create_data_quality_job.",
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

		resp, err := clients.ValidateDQRule(ctx, collibraClient, clients.PreviewRuleRequest{
			EdgeSiteID:   strings.TrimSpace(input.EdgeSiteID),
			ConnectionID: strings.TrimSpace(input.ConnectionID),
			SchemaName:   strings.TrimSpace(input.SchemaName),
			JobName:      strings.TrimSpace(input.JobName),
			PreviewRule:  input.PreviewRule,
			FilterQuery:  input.FilterQuery,
			RowLimit:     input.RowLimit,
		})
		if err != nil {
			return Output{Status: StatusError, Message: fmt.Sprintf("Could not validate rule: %v", err)}, nil
		}

		msg := "Rule is valid."
		if !resp.IsValid {
			msg = "Rule is invalid."
		}
		if strings.TrimSpace(resp.Message) != "" {
			msg = fmt.Sprintf("%s %s", msg, resp.Message)
		}
		return Output{Status: StatusSuccess, Message: msg, Valid: resp.IsValid}, nil
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
