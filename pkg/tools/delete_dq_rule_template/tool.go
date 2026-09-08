// Package delete_dq_rule_template implements the
// delete_data_quality_rule_template MCP tool: it removes a rule template from
// the data quality template library, optionally deleting the rules deployed from
// it.
//
// The public API's delete is idempotent — it answers 204 even when no template
// matched — and it does not refuse a template that still has deployments. Both
// behaviours the ticket asks for are therefore enforced here, by reading the
// template first: a missing template is reported as an error, and a template
// with live deployments is refused unless cascade is set.
package delete_dq_rule_template

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// OutputStatus is the overall outcome of a delete_data_quality_rule_template call.
type OutputStatus string

const (
	// StatusSuccess means the template (and, with cascade, its deployed rules)
	// was deleted.
	StatusSuccess OutputStatus = "success"
	// StatusPreview means confirm was not set: the tool reported what would be
	// deleted and deleted nothing.
	StatusPreview OutputStatus = "preview"
	// StatusValidationError means the inputs or the template's state ruled out the
	// delete before any write.
	StatusValidationError OutputStatus = "validation_error"
	// StatusError means the delete failed due to a downstream DQ error.
	StatusError OutputStatus = "error"
)

// Input is the tool's typed input.
type Input struct {
	Name    string `json:"name" jsonschema:"Required. Name of the rule template to delete. The name is case-sensitive and must match exactly."`
	Cascade bool   `json:"cascade,omitempty" jsonschema:"Whether to also delete every rule currently deployed from this template. false (default) refuses the delete while any deployment is live, reporting how many there are; true deletes the template and all of its deployed rules. Sent to the data quality service as deleteDeployments."`
	Confirm bool   `json:"confirm,omitempty" jsonschema:"Safety checkpoint. false (default) reports exactly what would be deleted — the template and, with cascade, the number of deployed rules — and deletes NOTHING. Set true to actually delete after the user has approved."`
}

// DeletionPlan describes what a delete would remove, or did remove.
type DeletionPlan struct {
	Name              string `json:"name" jsonschema:"The template to be deleted."`
	Description       string `json:"description,omitempty" jsonschema:"The template's description, so the user can confirm they mean this one."`
	Dialect           string `json:"dialect,omitempty" jsonschema:"The template's SQL dialect."`
	DeployedRuleCount int64  `json:"deployedRuleCount" jsonschema:"How many rules are currently deployed from this template."`
	Cascade           bool   `json:"cascade" jsonschema:"Whether the deployed rules are deleted along with the template."`
}

// Output is the typed response.
type Output struct {
	Status   OutputStatus  `json:"status" jsonschema:"'preview' when confirm was not set (nothing was deleted — review and call again with confirm=true); 'success' when the template was deleted; 'validation_error' when the inputs or the template's state rule out the delete; 'error' for downstream DQ failures."`
	Message  string        `json:"message" jsonschema:"Human-readable summary of what was, or would be, deleted."`
	Preview  *DeletionPlan `json:"preview,omitempty" jsonschema:"What would be deleted, returned when confirm=false. Nothing was deleted."`
	Deleted  *DeletionPlan `json:"deleted,omitempty" jsonschema:"What was deleted, on success."`
	Guidance string        `json:"guidance,omitempty" jsonschema:"On preview/validation_error/error, what to do next."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "delete_data_quality_rule_template",
		Title: "Delete Data Quality Rule Template",
		Description: "Delete a data quality rule template — a parameterized SQL pattern that can be deployed as concrete rules " +
			"(single data-quality checks on a table's data; Collibra calls them 'monitors') across many jobs (a job, also called a 'dataset', is a saved check on ONE database table). " +
			"The template is identified by name. Deleting a template removes it from the library so it can no longer be deployed. " +
			"cascade controls what happens to rules already deployed from it: with cascade=false (the default) the delete is REFUSED while any deployment is live, and the tool reports how many there are; " +
			"with cascade=true the template and every rule deployed from it are deleted together. Deleting deployed rules removes those checks from their jobs, so future runs no longer evaluate them. " +
			"Out-of-the-box (system) templates are read-only and cannot be deleted. " +
			"Built around a confirm checkpoint: confirm=false (default) reports exactly what would be deleted, including the deployed-rule count, and deletes nothing — review it with the user; confirm=true performs the delete. " +
			"To change a template instead of removing it use update_data_quality_rule_template; to inspect one first use get_data_quality_rule_template. Requires permission to manage rule templates. " +
			"Example user requests: \"Delete the old row count template\"; \"Remove that template and all the rules using it\"; \"Get rid of the duplicate email template we created by mistake\".",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: chip.Ptr(true), IdempotentHint: false, OpenWorldHint: chip.Ptr(false)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		name := strings.TrimSpace(input.Name)
		if name == "" {
			return Output{
				Status:   StatusValidationError,
				Message:  "name is required — the name of the rule template to delete.",
				Guidance: "Supply the template's exact name. Find it with list_data_quality_rule_templates. Nothing was deleted.",
			}, nil
		}

		// The delete endpoint is idempotent and reports nothing about
		// deployments, so read the template first: that is the only way to tell
		// the user it does not exist, or to refuse a cascade-less delete.
		stored, err := clients.GetDQRuleTemplate(ctx, collibraClient, name)
		if err != nil {
			return lookupError(err, name), nil
		}
		if stored.IsSystem {
			return Output{
				Status:   StatusValidationError,
				Message:  fmt.Sprintf("Rule template %q is out-of-the-box (system-defined) and cannot be deleted.", name),
				Guidance: "Out-of-the-box templates ship with the product and are read-only. Nothing was deleted.",
			}, nil
		}

		plan := &DeletionPlan{
			Name:              stored.Name,
			Description:       stored.Description,
			Dialect:           stored.Dialect,
			DeployedRuleCount: stored.DeployedRuleCount,
			Cascade:           input.Cascade,
		}

		if !input.Cascade && stored.DeployedRuleCount > 0 {
			return Output{
				Status: StatusValidationError,
				Message: fmt.Sprintf("Rule template %q cannot be deleted while %d rule(s) deployed from it are still active.",
					name, stored.DeployedRuleCount),
				Guidance: "Set cascade=true to delete the template together with its deployed rules, or remove those deployments first. Nothing was deleted.",
			}, nil
		}

		// Confirm checkpoint: without confirm, report the blast radius and
		// delete nothing.
		if !input.Confirm {
			return Output{
				Status:   StatusPreview,
				Message:  previewMessage(plan),
				Preview:  plan,
				Guidance: "Review what would be deleted with the user, then call again with confirm=true to delete it. This cannot be undone.",
			}, nil
		}

		if err := clients.DeleteDQRuleTemplate(ctx, collibraClient, name, input.Cascade); err != nil {
			return deleteError(err, name), nil
		}
		return Output{
			Status:  StatusSuccess,
			Message: deletedMessage(plan),
			Deleted: plan,
		}, nil
	}
}

func previewMessage(plan *DeletionPlan) string {
	if plan.Cascade && plan.DeployedRuleCount > 0 {
		return fmt.Sprintf("Ready to delete rule template %q AND the %d rule(s) deployed from it. Nothing has been deleted yet.",
			plan.Name, plan.DeployedRuleCount)
	}
	return fmt.Sprintf("Ready to delete rule template %q. It has no deployed rules. Nothing has been deleted yet.", plan.Name)
}

func deletedMessage(plan *DeletionPlan) string {
	if plan.Cascade && plan.DeployedRuleCount > 0 {
		return fmt.Sprintf("Deleted rule template %q and the %d rule(s) deployed from it.", plan.Name, plan.DeployedRuleCount)
	}
	return fmt.Sprintf("Deleted rule template %q.", plan.Name)
}

func lookupError(err error, name string) Output {
	if errors.Is(err, clients.ErrDQRuleTemplateNotFound) {
		return Output{
			Status:   StatusError,
			Message:  fmt.Sprintf("No rule template named %q exists, so nothing was deleted: %v", name, err),
			Guidance: "Check the name with list_data_quality_rule_templates — the name is case-sensitive and must match exactly.",
		}
	}
	return Output{
		Status:   StatusError,
		Message:  fmt.Sprintf("Could not read rule template %q before deleting it: %v", name, err),
		Guidance: "The template must be readable before it can be deleted. Retry, or check your permission to view rule templates. Nothing was deleted.",
	}
}

func deleteError(err error, name string) Output {
	if errors.Is(err, clients.ErrDQRuleTemplateReadOnly) {
		return Output{
			Status:   StatusValidationError,
			Message:  fmt.Sprintf("The data quality service refused to delete rule template %q because it is out-of-the-box: %v", name, err),
			Guidance: "Out-of-the-box templates are read-only. Nothing was deleted.",
		}
	}
	return Output{
		Status:   StatusError,
		Message:  fmt.Sprintf("Could not delete rule template %q: %v", name, err),
		Guidance: "Retry, or check your permission to manage rule templates. The template may still exist — confirm with get_data_quality_rule_template.",
	}
}
