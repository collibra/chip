// Package get_dq_rule_template implements the get_dq_rule_template MCP tool: it
// reads a single data quality rule template by id.
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
	TemplateID string `json:"templateId" jsonschema:"Required. UUID of the rule template (from list_dq_rule_templates)."`
}

// Template is the returned template definition.
type Template struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Description     string   `json:"description,omitempty"`
	SQLQuery        string   `json:"sqlQuery,omitempty"`
	SourceDialect   string   `json:"sourceDialect,omitempty"`
	Dimensions      []string `json:"dimensions,omitempty"`
	Tolerance       *int     `json:"tolerance,omitempty"`
	Ootb            bool     `json:"ootb"`
	DeploymentCount int64    `json:"deploymentCount"`
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
		Name:  "get_dq_rule_template",
		Title: "Get Data Quality Rule Template",
		Description: "Read a single data quality rule template by id — its parameterized SQL, dimensions, " +
			"default tolerance, whether it is built-in (OOTB), and how many times it has been deployed.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if strings.TrimSpace(input.TemplateID) == "" {
			return Output{Status: StatusValidationError, Message: "templateId is required."}, nil
		}

		t, err := clients.GetDQRuleTemplate(ctx, collibraClient, strings.TrimSpace(input.TemplateID))
		if err != nil {
			return Output{Status: StatusError, Message: fmt.Sprintf("Could not read template: %v", err)}, nil
		}

		return Output{
			Status:  StatusSuccess,
			Message: fmt.Sprintf("Found template %q.", t.Name),
			Template: &Template{
				ID:              t.ID,
				Name:            t.Name,
				Description:     t.Description,
				SQLQuery:        t.SQLQuery,
				SourceDialect:   t.SourceDialect,
				Dimensions:      t.Dimensions,
				Tolerance:       t.Tolerance,
				Ootb:            t.Ootb,
				DeploymentCount: t.DeploymentCount,
			},
		}, nil
	}
}
