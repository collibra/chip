// Package list_dq_rule_templates implements the list_dq_rule_templates MCP tool:
// it lists the data quality rule templates (built-in and custom) available in the
// connected DQ environment, so they can be chosen for deployment.
package list_dq_rule_templates

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// OutputStatus is the overall outcome of a list_dq_rule_templates call.
type OutputStatus string

const (
	// StatusSuccess means the templates were returned.
	StatusSuccess OutputStatus = "success"
	// StatusValidationError means the inputs failed validation before any read.
	StatusValidationError OutputStatus = "validation_error"
	// StatusError means the templates could not be read due to a downstream error.
	StatusError OutputStatus = "error"
)

const defaultLimit = 100

// Input is the tool's typed input. All fields are optional filters/pagination.
type Input struct {
	Name      string `json:"name,omitempty" jsonschema:"Optional. Partial-match filter on the template name."`
	Dimension string `json:"dimension,omitempty" jsonschema:"Optional. Filter by data quality dimension — a category such as Accuracy, Completeness or Validity (e.g. 'Completeness', 'Validity')."`
	Ootb      *bool  `json:"ootb,omitempty" jsonschema:"Optional. Filter by origin: true = built-in (OOTB, out-of-the-box) templates only, false = custom user-defined templates only, omit = all."`
	Offset    int    `json:"offset,omitempty" jsonschema:"Optional. Pagination offset (min 0). Defaults to 0."`
	Limit     int    `json:"limit,omitempty" jsonschema:"Optional. Max templates to return (1-100). Defaults to 100."`
}

// Template is one returned rule template.
type Template struct {
	ID          string   `json:"id" jsonschema:"Template UUID — pass this to deploy_dq_rule_template."`
	Name        string   `json:"name" jsonschema:"Template name."`
	Description string   `json:"description,omitempty" jsonschema:"What the template checks."`
	SQLQuery    string   `json:"sqlQuery,omitempty" jsonschema:"Parameterized SQL pattern (uses a {{column}} placeholder)."`
	Dimensions  []string `json:"dimensions,omitempty" jsonschema:"Data quality dimensions (categories such as Accuracy, Completeness, Validity) the template covers."`
	Tolerance   *int     `json:"tolerance,omitempty" jsonschema:"Default tolerance — number of failing ('breaking') records allowed before a rule fails; a count, not a percentage — when set."`
	Ootb        bool     `json:"ootb" jsonschema:"True for built-in (out-of-the-box) templates, false for custom user-defined ones."`
}

// Output is the typed response.
type Output struct {
	Status    OutputStatus `json:"status" jsonschema:"'success' when templates were returned; 'validation_error' for bad inputs; 'error' for downstream DQ failures."`
	Message   string       `json:"message" jsonschema:"Human-readable summary."`
	Templates []Template   `json:"templates,omitempty" jsonschema:"The matching templates."`
	Total     int64        `json:"total" jsonschema:"Total number of matching templates (for pagination)."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "list_dq_rule_templates",
		Title: "List Data Quality Rule Templates",
		Description: "List the data quality rule templates available in the connected DQ environment — built-in " +
			"(OOTB, i.e. out-of-the-box) templates plus any custom user-defined ones. Each template is a parameterized SQL pattern that " +
			"can be deployed as concrete rules (checks; Collibra calls them 'monitors') across columns via deploy_dq_rule_template. Optional filters: name, " +
			"dimension (a data-quality category such as Accuracy or Completeness), and ootb (built-in vs custom). Paginated (offset/limit).",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if input.Offset < 0 {
			return Output{Status: StatusValidationError, Message: "offset must be >= 0."}, nil
		}
		limit := input.Limit
		if limit == 0 {
			limit = defaultLimit
		}
		if limit < 1 || limit > 100 {
			return Output{Status: StatusValidationError, Message: "limit must be between 1 and 100."}, nil
		}

		list, err := clients.ListDQRuleTemplates(ctx, collibraClient, clients.ListDQRuleTemplatesParams{
			Name:      strings.TrimSpace(input.Name),
			Dimension: strings.TrimSpace(input.Dimension),
			Ootb:      input.Ootb,
			Offset:    input.Offset,
			Limit:     limit,
		})
		if err != nil {
			return Output{Status: StatusError, Message: fmt.Sprintf("Could not list templates: %v", err)}, nil
		}

		templates := make([]Template, 0, len(list.Results))
		for _, t := range list.Results {
			templates = append(templates, Template{
				ID:          t.ID,
				Name:        t.Name,
				Description: t.Description,
				SQLQuery:    t.SQLQuery,
				Dimensions:  t.Dimensions,
				Tolerance:   t.Tolerance,
				Ootb:        t.Ootb,
			})
		}

		return Output{
			Status:    StatusSuccess,
			Message:   fmt.Sprintf("Returned %d of %d template(s).", len(templates), list.Total),
			Templates: templates,
			Total:     list.Total,
		}, nil
	}
}
