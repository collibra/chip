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
	// StatusSuccess means every target was deployed.
	StatusSuccess OutputStatus = "success"
	// StatusPartial means the deploy ran but some targets were skipped/failed
	// while others were deployed.
	StatusPartial OutputStatus = "partial"
	// StatusValidationError means the inputs failed validation before any write.
	StatusValidationError OutputStatus = "validation_error"
	// StatusError means the deployment failed due to a downstream DQ error.
	StatusError OutputStatus = "error"
	// StatusPreview means confirm was not set: the tool returned the template +
	// target list for review and deployed nothing.
	StatusPreview OutputStatus = "preview"
)

// Target is one deployment target.
type Target struct {
	JobName    string `json:"jobName" jsonschema:"Required. Name of the existing data quality job to deploy the rule on (a job, also called a 'dataset', is a saved check on one database table), e.g. 'PUBLIC.CUSTOMERS'."`
	ColumnName string `json:"columnName,omitempty" jsonschema:"Column substituted for the template's {{column}} placeholder. Required for column-level templates; omit for table-level templates."`
}

// Input is the tool's typed input.
type Input struct {
	RuleTemplateName string   `json:"ruleTemplateName" jsonschema:"Required. Name of the rule template to deploy (from list_data_quality_rule_templates)."`
	Targets          []Target `json:"targets" jsonschema:"Required. One or more job/column targets. Each deployed rule is named {templateName}_{columnName} by the server."`
	Confirm          bool     `json:"confirm,omitempty" jsonschema:"Safety checkpoint. false (default) returns a PREVIEW of the template and the target list WITHOUT deploying, so it can be reviewed with the user (inspect the template's SQL with get_data_quality_rule_template). Set true to actually deploy after the user has approved."`
}

// Outcome is the per-target result of a deploy.
type Outcome struct {
	JobName          string `json:"jobName" jsonschema:"The target job."`
	ColumnName       string `json:"columnName,omitempty" jsonschema:"The target column, when set."`
	DeployedRuleName string `json:"deployedRuleName,omitempty" jsonschema:"Name of the rule the server created for this target, when deployed."`
	Status           string `json:"status" jsonschema:"Per-target status reported by the DQ service (e.g. deployed or SKIPPED)."`
	Reason           string `json:"reason,omitempty" jsonschema:"Why a target was skipped or failed, when applicable."`
}

// Output is the typed response.
type Output struct {
	Status   OutputStatus `json:"status" jsonschema:"'preview' when confirm was not set (nothing deployed — review and call again with confirm=true); 'success' when every target was deployed; 'partial' when some targets were skipped/failed; 'validation_error' for bad inputs; 'error' for downstream DQ failures."`
	Message  string       `json:"message" jsonschema:"Human-readable summary, including deployed vs skipped counts."`
	Preview  *Preview     `json:"preview,omitempty" jsonschema:"The template name and resolved targets returned when confirm=false; nothing was deployed."`
	Outcomes []Outcome    `json:"outcomes,omitempty" jsonschema:"Per-target deploy outcomes (partial-success): each target's status and, on skip/failure, the reason."`
	Deployed int          `json:"deployed,omitempty" jsonschema:"Number of targets successfully deployed."`
	Skipped  int          `json:"skipped,omitempty" jsonschema:"Number of targets skipped or failed."`
}

// Preview is the deployment plan echoed back for review when confirm is false.
type Preview struct {
	RuleTemplateName string   `json:"ruleTemplateName"`
	Targets          []Target `json:"targets"`
	Count            int      `json:"count"`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "deploy_data_quality_rule_template",
		Title: "Deploy Data Quality Rule Template",
		Description: "Instantiate a rule template as concrete rules (checks on a table's data; Collibra calls them 'monitors') across one or more job/column targets " +
			"(a job, also called a 'dataset', is a saved check on ONE database table). " +
			"The DQ service resolves dialect-specific SQL and creates one rule per target, each named " +
			"{templateName}_{columnName}. Provide a columnName per target for column-level templates. " +
			"Built around a confirm checkpoint: confirm=false (default) returns a PREVIEW of the template + targets without deploying — review it with the user; confirm=true deploys. " +
			"Requires permission to deploy templates and to create rules on the target jobs. " +
			"The deploy is partial-success: each target is deployed or skipped independently, and the per-target outcomes (with skip reasons) are returned.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{DestructiveHint: chip.Ptr(false)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if strings.TrimSpace(input.RuleTemplateName) == "" {
			return Output{Status: StatusValidationError, Message: "ruleTemplateName is required."}, nil
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

		ruleTemplateName := strings.TrimSpace(input.RuleTemplateName)

		// Confirm checkpoint: without confirm, return the deployment plan for
		// review and deploy nothing.
		if !input.Confirm {
			review := make([]Target, len(targets))
			for i, t := range targets {
				review[i] = Target{JobName: t.JobName, ColumnName: t.ColumnName}
			}
			return Output{
				Status: StatusPreview,
				Message: fmt.Sprintf("Preview only — nothing deployed. Will deploy template %q to %d target(s) (each rule named {template}_{column}). "+
					"Inspect the template's SQL with get_data_quality_rule_template, review the targets with the user, then call again with confirm=true.", ruleTemplateName, len(targets)),
				Preview: &Preview{RuleTemplateName: ruleTemplateName, Targets: review, Count: len(targets)},
			}, nil
		}

		result, err := clients.DeployDQRuleTemplate(ctx, collibraClient, ruleTemplateName, targets)
		if err != nil {
			return Output{Status: StatusError, Message: fmt.Sprintf("Could not deploy template: %v", err)}, nil
		}

		// Partial-success: surface each target's outcome and tally deployed vs
		// skipped. A target counts as deployed when the server assigned it a rule
		// name; otherwise it was skipped/failed (with a reason).
		outcomes := make([]Outcome, 0, len(result.Results))
		deployed, skipped := 0, 0
		for _, o := range result.Results {
			outcomes = append(outcomes, Outcome{
				JobName:          o.JobName,
				ColumnName:       o.ColumnName,
				DeployedRuleName: o.DeployedRuleName,
				Status:           o.Status,
				Reason:           o.Reason,
			})
			if o.DeployedRuleName != "" {
				deployed++
			} else {
				skipped++
			}
		}

		status := StatusSuccess
		if skipped > 0 {
			if deployed == 0 {
				status = StatusError
			} else {
				status = StatusPartial
			}
		}

		return Output{
			Status:   status,
			Message:  fmt.Sprintf("Deployed %d of %d target(s); %d skipped.", deployed, len(outcomes), skipped),
			Outcomes: outcomes,
			Deployed: deployed,
			Skipped:  skipped,
		}, nil
	}
}
