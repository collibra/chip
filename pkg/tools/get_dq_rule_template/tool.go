// Package get_dq_rule_template implements the get_dq_rule_template MCP tool: it
// reads a single data quality rule template by name.
package get_dq_rule_template

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// OutputStatus is the overall outcome of a get_dq_rule_template call.
type OutputStatus string

const (
	// StatusSuccess means the template was found and returned.
	StatusSuccess OutputStatus = "success"
	// StatusValidationError means the inputs failed validation before any read.
	StatusValidationError OutputStatus = "validation_error"
	// StatusError means the template could not be read due to a downstream error.
	StatusError OutputStatus = "error"
)

// Input is the tool's typed input.
type Input struct {
	RuleTemplateName string `json:"ruleTemplateName" jsonschema:"Required. Name of the rule template (from list_data_quality_rule_templates)."`
}

// Template is the returned template definition.
type Template struct {
	ID                string   `json:"id"`
	Name              string   `json:"ruleTemplateName"`
	Description       string   `json:"description,omitempty"`
	SQL               string   `json:"sql,omitempty"`
	Dialect           string   `json:"dialect,omitempty"`
	Dimensions        []string `json:"dimensions,omitempty"`
	Tolerance         *int     `json:"tolerance,omitempty"`
	IsSystem          bool     `json:"isSystem"`
	DeployedRuleCount int64    `json:"deployedRuleCount"`
}

// Output is the typed response.
type Output struct {
	Status   OutputStatus `json:"status" jsonschema:"'success' when the template was found; 'validation_error' for bad inputs; 'error' for downstream DQ failures."`
	Message  string       `json:"message" jsonschema:"Human-readable summary."`
	Template *Template    `json:"template,omitempty" jsonschema:"The template definition, on success."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "get_data_quality_rule_template",
		Title: "Get Data Quality Rule Template",
		Description: "Read a single data quality rule template by name — its parameterized SQL, dimensions (data-quality categories such as Accuracy or Completeness), " +
			"default tolerance (number of failing records allowed before a rule fails; a count, not a percentage), whether it is built-in (system) vs custom, and how many rules have been deployed from it.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: chip.Ptr(false), IdempotentHint: true, OpenWorldHint: chip.Ptr(false)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if strings.TrimSpace(input.RuleTemplateName) == "" {
			return Output{Status: StatusValidationError, Message: "ruleTemplateName is required."}, nil
		}

		t, err := clients.GetDQRuleTemplate(ctx, collibraClient, strings.TrimSpace(input.RuleTemplateName))
		if err != nil {
			return Output{Status: StatusError, Message: fmt.Sprintf("Could not read template: %v", err)}, nil
		}

		return Output{
			Status:  StatusSuccess,
			Message: fmt.Sprintf("Found template %q.", t.Name),
			Template: &Template{
				ID:                t.ID,
				Name:              t.Name,
				Description:       t.Description,
				SQL:               t.SQL,
				Dialect:           t.Dialect,
				Dimensions:        t.Dimensions,
				Tolerance:         t.Tolerance,
				IsSystem:          t.IsSystem,
				DeployedRuleCount: t.DeployedRuleCount,
			},
		}, nil
	}
}
