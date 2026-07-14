// Package deploy_dq_rule_template implements the deploy_dq_rule_template MCP tool:
// it instantiates a rule template as concrete rules across one or more job/column
// targets, using dialect-specific SQL resolved by the DQ service.
package deploy_dq_rule_template

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// OutputStatus is the overall outcome of a deploy_dq_rule_template call.
type OutputStatus string

const (
	// StatusSuccess means the template was deployed to all targets.
	StatusSuccess OutputStatus = "success"
	// StatusValidationError means the inputs failed validation before any write.
	StatusValidationError OutputStatus = "validation_error"
	// StatusError means the deployment failed due to a downstream DQ error.
	StatusError OutputStatus = "error"
)

// Target is one deployment target.
type Target struct {
	JobName    string `json:"jobName" jsonschema:"Required. Name of the existing DQ job (dataset) to deploy the rule on, e.g. 'PUBLIC.CUSTOMERS'."`
	ColumnName string `json:"columnName,omitempty" jsonschema:"Column substituted for the template's {{column}} placeholder. Required for column-level templates; omit for table-level templates."`
}

// Input is the tool's typed input.
type Input struct {
	TemplateID string   `json:"templateId" jsonschema:"Required. UUID of the rule template to deploy (from list_dq_rule_templates)."`
	Targets    []Target `json:"targets" jsonschema:"Required. One or more job/column targets. Each deployed rule is named {templateName}_{columnName} by the server."`
}

// Output is the typed response.
type Output struct {
	Status  OutputStatus `json:"status" jsonschema:"'success' when the template was deployed; 'validation_error' for bad inputs; 'error' for downstream DQ failures."`
	Message string       `json:"message" jsonschema:"Human-readable summary."`
	Count   int          `json:"count,omitempty" jsonschema:"Number of targets the template was deployed to, on success."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "deploy_dq_rule_template",
		Title: "Deploy Data Quality Rule Template",
		Description: "Instantiate a rule template as concrete rules across one or more job/column targets. " +
			"The DQ service resolves dialect-specific SQL and creates one rule per target, each named " +
			"{templateName}_{columnName}. Provide a columnName per target for column-level templates. " +
			"Requires permission to deploy templates and to create rules on the target jobs. " +
			"The deploy is all-or-nothing on the server side.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{DestructiveHint: chip.Ptr(false)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if strings.TrimSpace(input.TemplateID) == "" {
			return Output{Status: StatusValidationError, Message: "templateId is required."}, nil
		}
		if len(input.Targets) == 0 {
			return Output{Status: StatusValidationError, Message: "at least one target is required."}, nil
		}

		targets := make([]clients.DQTemplateDeployTarget, 0, len(input.Targets))
		for i, t := range input.Targets {
			if strings.TrimSpace(t.JobName) == "" {
				return Output{Status: StatusValidationError, Message: fmt.Sprintf("targets[%d].jobName is required.", i)}, nil
			}
			targets = append(targets, clients.DQTemplateDeployTarget{
				JobName:    strings.TrimSpace(t.JobName),
				ColumnName: strings.TrimSpace(t.ColumnName),
			})
		}

		if err := clients.DeployDQRuleTemplate(ctx, collibraClient, strings.TrimSpace(input.TemplateID), targets); err != nil {
			return Output{Status: StatusError, Message: fmt.Sprintf("Could not deploy template: %v", err)}, nil
		}

		return Output{
			Status:  StatusSuccess,
			Message: fmt.Sprintf("Deployed template to %d target(s).", len(targets)),
			Count:   len(targets),
		}, nil
	}
}
