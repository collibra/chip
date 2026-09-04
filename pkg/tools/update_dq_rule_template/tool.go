// Package update_dq_rule_template implements the
// update_data_quality_rule_template MCP tool: it changes a rule template in the
// data quality template library and reports how the change landed on the rules
// already deployed from it.
//
// The public API's update is a PUT (full replacement) whose payload requires
// name, sql, dialect, description and dimensions. The partial-update semantics
// this tool offers are therefore built here: it reads the template first and
// overlays only the fields the caller supplied, so an omitted field keeps its
// stored value instead of being wiped.
//
// The API also gives no way to update a template WITHOUT propagating to its
// deployed rules — the update and the cascade are one transaction. The tool
// exposes no cascade switch for that reason, and shows the number of rules that
// will be affected in its preview instead.
package update_dq_rule_template

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

// OutputStatus is the overall outcome of an update_data_quality_rule_template call.
type OutputStatus string

const (
	// StatusSuccess means the template was updated and every deployed rule took
	// the change.
	StatusSuccess OutputStatus = "success"
	// StatusPartial means the template was updated but the cascade skipped or
	// failed on some deployed rules.
	StatusPartial OutputStatus = "partial"
	// StatusPreview means confirm was not set: the tool echoed the merged
	// template and wrote nothing.
	StatusPreview OutputStatus = "preview"
	// StatusValidationError means the inputs failed validation before any write.
	StatusValidationError OutputStatus = "validation_error"
	// StatusError means the update failed due to a downstream DQ error.
	StatusError OutputStatus = "error"
)

// Limits enforced by the public rule template API.
const (
	maxDescriptionLength = 1000
	maxSQLLength         = 10000
	maxDialectLength     = 255
	maxDimensions        = 20
	maxBusinessRuleLinks = 100
)

// deploymentSkipped and deploymentFailed are the non-success cascade outcomes the
// service reports per deployed rule.
const (
	deploymentSkipped = "SKIPPED"
	deploymentFailed  = "FAILED"
)

// Input is the tool's typed input. Every field except name is optional: omit one
// to leave that part of the template unchanged.
type Input struct {
	Name              string   `json:"name" jsonschema:"Required. Name of the existing rule template to update. The template is addressed by name; this tool never renames it."`
	SQL               string   `json:"sql,omitempty" jsonschema:"Optional. Replacement parameterized SQL, up to 10000 characters, using a {{column}} placeholder for the column being checked. Omit to keep the stored SQL."`
	Dialect           string   `json:"dialect,omitempty" jsonschema:"Optional. Replacement SQL dialect the query is authored in, e.g. 'snowflake'. Omit to keep the stored dialect. The data quality service rejects a dialect it cannot translate."`
	Dimensions        []string `json:"dimensions,omitempty" jsonschema:"Optional. Replacement list of data quality dimensions, at least one and at most 20. This REPLACES the stored list rather than adding to it. Omit to keep the stored dimensions."`
	Description       string   `json:"description,omitempty" jsonschema:"Optional. Replacement description, up to 1000 characters. Omit to keep the stored description."`
	BusinessRuleLinks []string `json:"businessRuleLinks,omitempty" jsonschema:"Optional, up to 100. Replacement list of Business Rule assets this template implements, each given as the asset's exact name or its UUID. This REPLACES the stored links rather than adding to them. Omit to keep the stored links. Names are resolved to UUIDs before the write; an ambiguous name is reported rather than guessed."`
	Confirm           bool     `json:"confirm,omitempty" jsonschema:"Safety checkpoint. false (default) returns a PREVIEW of the merged template, plus the number of deployed rules the change will cascade onto, and writes NOTHING. Set true to apply the update after the user has approved."`
}

// TemplateDefinition is the complete template that will be, or was, written —
// the stored template with the caller's changes overlaid. Every field the tool
// sends appears here so a preview cannot hide one.
type TemplateDefinition struct {
	Name                 string   `json:"name"`
	Description          string   `json:"description"`
	SQL                  string   `json:"sql"`
	Dialect              string   `json:"dialect"`
	Dimensions           []string `json:"dimensions"`
	BusinessRuleAssetIDs []string `json:"businessRuleAssetIds,omitempty" jsonschema:"UUIDs of the linked Business Rule assets, after resolving any names supplied in businessRuleLinks."`
	Tolerance            *int     `json:"tolerance,omitempty" jsonschema:"The template's stored tolerance, carried through unchanged. This tool cannot set it."`
}

// DeploymentOutcome is how the cascade landed on one rule deployed from the
// template.
type DeploymentOutcome struct {
	JobName          string `json:"jobName" jsonschema:"The job whose deployed rule the change cascaded onto."`
	ColumnName       string `json:"columnName,omitempty" jsonschema:"The deployed rule's column, when it targets one."`
	DeployedRuleName string `json:"deployedRuleName,omitempty" jsonschema:"Name of the deployed rule."`
	Status           string `json:"status" jsonschema:"DEPLOYED when the rule took the change; SKIPPED or FAILED when it did not."`
	Reason           string `json:"reason,omitempty" jsonschema:"Why the rule was skipped or failed, e.g. a dialect the template cannot be translated into."`
}

// UpdatePreview is the merged template plus the blast radius of applying it.
type UpdatePreview struct {
	Template          TemplateDefinition `json:"template" jsonschema:"The complete template as it would be stored after the update."`
	ChangedFields     []string           `json:"changedFields" jsonschema:"Which fields this call would change; everything else is carried through unchanged."`
	DeployedRuleCount int64              `json:"deployedRuleCount" jsonschema:"How many rules are currently deployed from this template. The update ALWAYS cascades onto all of them — the API offers no way to update the definition alone."`
}

// Output is the typed response.
type Output struct {
	Status      OutputStatus        `json:"status" jsonschema:"'preview' when confirm was not set (nothing was written — review and call again with confirm=true); 'success' when the template and every deployed rule took the change; 'partial' when the template was updated but some deployed rules were skipped or failed; 'validation_error' for bad inputs; 'error' for downstream DQ failures."`
	Message     string              `json:"message" jsonschema:"Human-readable summary, including how many deployed rules took the change."`
	Preview     *UpdatePreview      `json:"preview,omitempty" jsonschema:"The merged template and the number of deployed rules that would be affected, returned when confirm=false. Nothing was written."`
	Template    *TemplateDefinition `json:"template,omitempty" jsonschema:"The template as stored after the update, on success."`
	Deployments []DeploymentOutcome `json:"deployments,omitempty" jsonschema:"One entry per rule the change cascaded onto, with the outcome for each."`
	Updated     int                 `json:"updated,omitempty" jsonschema:"Number of deployed rules that took the change."`
	Skipped     int                 `json:"skipped,omitempty" jsonschema:"Number of deployed rules that were skipped or failed."`
	Guidance    string              `json:"guidance,omitempty" jsonschema:"On preview/validation_error/error/partial, what to do next."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "update_data_quality_rule_template",
		Title: "Update Data Quality Rule Template",
		Description: "Change an existing data quality rule template — a parameterized SQL pattern, with a {{column}} placeholder, that is deployed as concrete rules " +
			"(single data-quality checks on a table's data; Collibra calls them 'monitors') across many jobs (a job, also called a 'dataset', is a saved check on ONE database table). " +
			"The template is identified by name, and the update is partial: supply only the fields you want to change and the rest keep their stored values. This tool cannot rename a template. " +
			"IMPORTANT: the change ALWAYS cascades to every rule already deployed from the template — the data quality API updates the definition and its deployments in one transaction and offers no way to do one without the other. " +
			"The preview reports how many deployed rules will be affected, and the result reports per-rule outcomes, since a rule can be SKIPPED (for example when the template cannot be translated into that job's dialect) while others take the change. " +
			"Use this to correct or evolve a template already in the library; to add a new one use create_data_quality_rule_template, and to change a single rule on one job use create_data_quality_rule instead. " +
			"Out-of-the-box (system) templates are read-only and cannot be updated. " +
			"dimensions and businessRuleLinks REPLACE the stored lists rather than adding to them; businessRuleLinks accepts Business Rule asset names or UUIDs and resolves names before the write. " +
			"Built around a confirm checkpoint: confirm=false (default) returns a PREVIEW of the merged template and the affected-rule count and writes nothing — review it with the user; confirm=true applies the update. " +
			"Requires permission to manage rule templates. " +
			"Example user requests: \"Update the null-check template's SQL\"; \"Change the dimensions on our row count template\"; \"Fix the template and push it to everything using it\".",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: chip.Ptr(false), IdempotentHint: false, OpenWorldHint: chip.Ptr(false)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		name := strings.TrimSpace(input.Name)
		if name == "" {
			return *invalid("name is required — the name of the existing rule template to update."), nil
		}
		if out := validateChanges(input); out != nil {
			return *out, nil
		}

		stored, err := clients.GetDQRuleTemplate(ctx, collibraClient, name)
		if err != nil {
			return lookupError(err, name), nil
		}
		if stored.IsSystem {
			return Output{
				Status:   StatusValidationError,
				Message:  fmt.Sprintf("Rule template %q is out-of-the-box (system-defined) and cannot be updated.", name),
				Guidance: "Out-of-the-box templates are read-only. Copy it into a new template with create_data_quality_rule_template and change that instead.",
			}, nil
		}

		merged, changed, out := merge(ctx, collibraClient, input, stored)
		if out != nil {
			return *out, nil
		}
		if len(changed) == 0 {
			return Output{
				Status:   StatusValidationError,
				Message:  fmt.Sprintf("No changes were supplied for rule template %q.", name),
				Guidance: "Supply at least one of sql, dialect, dimensions, description or businessRuleLinks with a value that differs from the stored one.",
			}, nil
		}

		// Confirm checkpoint: without confirm, echo the merged template and the
		// blast radius, and write nothing.
		if !input.Confirm {
			return Output{
				Status: StatusPreview,
				Message: fmt.Sprintf("Ready to update rule template %q (%s). The change will cascade onto %d deployed rule(s). Nothing has been changed yet.",
					name, strings.Join(changed, ", "), stored.DeployedRuleCount),
				Preview: &UpdatePreview{
					Template:          *merged,
					ChangedFields:     changed,
					DeployedRuleCount: stored.DeployedRuleCount,
				},
				Guidance: "Review the merged template and the affected-rule count with the user, then call again with confirm=true to apply it.",
			}, nil
		}

		result, err := clients.UpdateDQRuleTemplate(ctx, collibraClient, name, clients.DQRuleTemplateWriteRequest{
			Name:                 merged.Name,
			Description:          merged.Description,
			SQL:                  merged.SQL,
			Dialect:              merged.Dialect,
			Dimensions:           merged.Dimensions,
			Tolerance:            merged.Tolerance,
			BusinessRuleAssetIDs: merged.BusinessRuleAssetIDs,
		})
		if err != nil {
			return updateError(err, name), nil
		}
		return updated(result, name), nil
	}
}

// validateChanges enforces the API's field limits on whatever the caller supplied.
func validateChanges(input Input) *Output {
	if length := len(strings.TrimSpace(input.SQL)); length > maxSQLLength {
		return invalid(fmt.Sprintf("sql is %d characters; the maximum is %d.", length, maxSQLLength))
	}
	if length := len(strings.TrimSpace(input.Dialect)); length > maxDialectLength {
		return invalid(fmt.Sprintf("dialect is %d characters; the maximum is %d.", length, maxDialectLength))
	}
	if length := len(strings.TrimSpace(input.Description)); length > maxDescriptionLength {
		return invalid(fmt.Sprintf("description is %d characters; the maximum is %d.", length, maxDescriptionLength))
	}
	if len(input.Dimensions) > maxDimensions {
		return invalid(fmt.Sprintf("dimensions has %d entries; the maximum is %d.", len(input.Dimensions), maxDimensions))
	}
	if len(input.BusinessRuleLinks) > maxBusinessRuleLinks {
		return invalid(fmt.Sprintf("businessRuleLinks has %d entries; the maximum is %d.", len(input.BusinessRuleLinks), maxBusinessRuleLinks))
	}
	return nil
}

// merge overlays the supplied fields onto the stored template, returning the
// complete payload to PUT and the names of the fields that actually change.
func merge(ctx context.Context, collibraClient *http.Client, input Input, stored *clients.DQRuleTemplate) (*TemplateDefinition, []string, *Output) {
	merged := &TemplateDefinition{
		Name:                 stored.Name,
		Description:          stored.Description,
		SQL:                  stored.SQL,
		Dialect:              stored.Dialect,
		Dimensions:           stored.Dimensions,
		BusinessRuleAssetIDs: stored.BusinessRuleAssetIDs,
		Tolerance:            stored.Tolerance,
	}
	var changed []string

	if sql := strings.TrimSpace(input.SQL); sql != "" && sql != stored.SQL {
		merged.SQL = sql
		changed = append(changed, "sql")
	}
	if dialect := strings.TrimSpace(input.Dialect); dialect != "" && dialect != stored.Dialect {
		merged.Dialect = dialect
		changed = append(changed, "dialect")
	}
	if description := strings.TrimSpace(input.Description); description != "" && description != stored.Description {
		merged.Description = description
		changed = append(changed, "description")
	}
	if len(input.Dimensions) > 0 {
		dimensions, out := cleanDimensions(input.Dimensions)
		if out != nil {
			return nil, nil, out
		}
		if !sameStrings(dimensions, stored.Dimensions) {
			merged.Dimensions = dimensions
			changed = append(changed, "dimensions")
		}
	}
	if len(input.BusinessRuleLinks) > 0 {
		ids, problems, err := clients.ResolveBusinessRuleAssetRefs(ctx, collibraClient, input.BusinessRuleLinks)
		if err != nil {
			return nil, nil, &Output{
				Status:   StatusError,
				Message:  fmt.Sprintf("Could not resolve the businessRuleLinks: %v", err),
				Guidance: "The Business Rule asset lookup failed. Retry, or pass asset UUIDs instead of names to skip the lookup. Nothing was changed.",
			}
		}
		if len(problems) > 0 {
			return nil, nil, &Output{
				Status:   StatusValidationError,
				Message:  "Could not resolve every entry in businessRuleLinks: " + strings.Join(problems, "; ") + ".",
				Guidance: "Fix or remove the listed businessRuleLinks entries and call again. Nothing was changed.",
			}
		}
		if !sameStrings(ids, stored.BusinessRuleAssetIDs) {
			merged.BusinessRuleAssetIDs = ids
			changed = append(changed, "businessRuleLinks")
		}
	}

	// The PUT payload always requires these, so a template stored without one
	// cannot be updated until the caller supplies it.
	if merged.Description == "" {
		return nil, nil, invalid("this template has no stored description and the data quality service requires one — supply description to update it.")
	}
	if len(merged.Dimensions) == 0 {
		return nil, nil, invalid("this template has no stored dimensions and the data quality service requires at least one — supply dimensions to update it.")
	}
	return merged, changed, nil
}

// updated turns the API's cascade outcomes into the tool's response, reporting
// partial success when a deployed rule did not take the change.
func updated(result *clients.DQRuleTemplateUpdateResult, name string) Output {
	outcomes := make([]DeploymentOutcome, 0, len(result.Deployments))
	updatedCount, skippedCount := 0, 0
	for _, deployment := range result.Deployments {
		outcomes = append(outcomes, DeploymentOutcome{
			JobName:          deployment.JobName,
			ColumnName:       deployment.ColumnName,
			DeployedRuleName: deployment.DeployedRuleName,
			Status:           deployment.Status,
			Reason:           deployment.Reason,
		})
		if strings.EqualFold(deployment.Status, deploymentSkipped) || strings.EqualFold(deployment.Status, deploymentFailed) {
			skippedCount++
			continue
		}
		updatedCount++
	}

	out := Output{
		Status: StatusSuccess,
		Message: fmt.Sprintf("Updated rule template %q; %d deployed rule(s) took the change.",
			result.RuleTemplate.Name, updatedCount),
		Template: &TemplateDefinition{
			Name:                 result.RuleTemplate.Name,
			Description:          result.RuleTemplate.Description,
			SQL:                  result.RuleTemplate.SQL,
			Dialect:              result.RuleTemplate.Dialect,
			Dimensions:           result.RuleTemplate.Dimensions,
			BusinessRuleAssetIDs: result.RuleTemplate.BusinessRuleAssetIDs,
			Tolerance:            result.RuleTemplate.Tolerance,
		},
		Deployments: outcomes,
		Updated:     updatedCount,
		Skipped:     skippedCount,
	}
	if skippedCount > 0 {
		out.Status = StatusPartial
		out.Message = fmt.Sprintf("Updated rule template %q, but %d of %d deployed rule(s) did not take the change.",
			result.RuleTemplate.Name, skippedCount, len(result.Deployments))
		out.Guidance = "The template itself is updated. Inspect the per-deployment reasons above — a skip usually means the template could not be translated into that job's dialect."
	}
	return out
}

func cleanDimensions(dimensions []string) ([]string, *Output) {
	cleaned := make([]string, 0, len(dimensions))
	for _, dimension := range dimensions {
		if trimmed := strings.TrimSpace(dimension); trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	if len(cleaned) == 0 {
		return nil, invalid("dimensions was supplied but contains no non-empty value — omit it to keep the stored dimensions, or supply at least one, e.g. ['Completeness'].")
	}
	if len(cleaned) > maxDimensions {
		return nil, invalid(fmt.Sprintf("dimensions has %d entries; the maximum is %d.", len(cleaned), maxDimensions))
	}
	return cleaned, nil
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func invalid(message string) *Output {
	return &Output{
		Status:   StatusValidationError,
		Message:  message,
		Guidance: "Correct the input and call again. Nothing was changed.",
	}
}

func lookupError(err error, name string) Output {
	if errors.Is(err, clients.ErrDQRuleTemplateNotFound) {
		return Output{
			Status:   StatusError,
			Message:  fmt.Sprintf("No rule template named %q exists, so nothing was updated: %v", name, err),
			Guidance: "Check the name with list_data_quality_rule_templates — the name is case-sensitive and must match exactly.",
		}
	}
	return Output{
		Status:   StatusError,
		Message:  fmt.Sprintf("Could not read rule template %q before updating it: %v", name, err),
		Guidance: "The template must be readable before it can be updated. Retry, or check your permission to view rule templates. Nothing was changed.",
	}
}

func updateError(err error, name string) Output {
	return Output{
		Status:   StatusError,
		Message:  fmt.Sprintf("Could not update rule template %q: %v", name, err),
		Guidance: "Check the SQL, dialect and businessRuleLinks against the message above, then retry. The template was not changed.",
	}
}
