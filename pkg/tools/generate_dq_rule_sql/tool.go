// Package generate_dq_rule_sql implements the generate_dq_rule_sql MCP tool: it
// turns a plain-language description of a data quality check into rule SQL, so a
// rule can be authored without writing SQL by hand.
package generate_dq_rule_sql

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// OutputStatus is the overall outcome of a generate_dq_rule_sql call.
type OutputStatus string

const (
	// StatusSuccess means SQL was generated.
	StatusSuccess OutputStatus = "success"
	// StatusValidationError means the inputs failed validation before any call.
	StatusValidationError OutputStatus = "validation_error"
	// StatusError means generation failed due to a downstream DQ error.
	StatusError OutputStatus = "error"
)

// Input is the tool's typed input. edgeSiteId/connectionId come from
// prepare_create_dq_job; the table is identified by jobName.
type Input struct {
	EdgeSiteID   string   `json:"edgeSiteId" jsonschema:"Required. UUID of the Collibra Edge runtime/site that reaches the source database. From prepare_create_dq_job resolved.edgeSiteId."`
	ConnectionID string   `json:"connectionId" jsonschema:"Required. UUID of the specific database connection on that Edge site. From prepare_create_dq_job resolved.connectionId."`
	JobName      string   `json:"jobName" jsonschema:"Required. Name of the data quality job whose table the rule runs against (a job, also called a 'dataset', is a saved check on one database table)."`
	Columns      []string `json:"columns" jsonschema:"Required. One or more column names giving the rule its context (e.g. the column(s) the check concerns)."`
	Query        string   `json:"query" jsonschema:"Required. Plain-language description of the rule intent, e.g. 'email must not be null and must contain an @'."`
}

// Output is the typed response.
type Output struct {
	Status   OutputStatus `json:"status" jsonschema:"'success' when SQL was generated; 'validation_error' for bad inputs; 'error' for downstream DQ failures."`
	Message  string       `json:"message" jsonschema:"Human-readable summary."`
	SQLQuery string       `json:"sqlQuery,omitempty" jsonschema:"The generated rule SQL. Review/validate it (validate_dq_rule) before creating the rule; use it as monitorValue in create_dq_rule."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "generate_dq_rule_sql",
		Title: "Generate Data Quality Rule SQL (Text2SQL)",
		Description: "Turn a plain-language description of a data quality check into rule SQL, so a rule (the check; Collibra calls it a 'monitor') can be " +
			"authored without writing SQL by hand. Returns a single SQL string (no separate filter clause). " +
			"Always review and validate the generated SQL (validate_dq_rule) before creating the rule. " +
			"Requires edgeSiteId and connectionId — the connection to the source database (edgeSiteId = the Collibra Edge runtime/site that reaches the source, connectionId = the specific database connection), from prepare_create_dq_job — and uses Collibra DQ AI.",
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

		cols := make([]string, 0, len(input.Columns))
		for _, c := range input.Columns {
			if s := strings.TrimSpace(c); s != "" {
				cols = append(cols, s)
			}
		}
		if len(cols) == 0 {
			return Output{Status: StatusValidationError, Message: "columns must contain at least one non-empty column name."}, nil
		}

		resp, err := clients.GenerateDQRuleSQL(ctx, collibraClient, clients.Text2SQLRequest{
			EdgeSiteID:   strings.TrimSpace(input.EdgeSiteID),
			ConnectionID: strings.TrimSpace(input.ConnectionID),
			JobName:      strings.TrimSpace(input.JobName),
			Columns:      cols,
			Query:        input.Query,
		})
		if err != nil {
			return Output{Status: StatusError, Message: fmt.Sprintf("Could not generate SQL: %v", err)}, nil
		}

		return Output{
			Status:   StatusSuccess,
			Message:  "Generated rule SQL. Review and validate it before creating the rule.",
			SQLQuery: resp.SQLQuery,
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
		{"jobName", input.JobName},
		{"query", input.Query},
	}
	for _, f := range required {
		if strings.TrimSpace(f.val) == "" {
			return &Output{Status: StatusValidationError, Message: f.name + " is required."}
		}
	}
	if len(input.Columns) == 0 {
		return &Output{Status: StatusValidationError, Message: "columns is required (at least one column)."}
	}
	return nil
}
