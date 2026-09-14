// Package detach_dq_rule_from_template soft-unlinks a deployed data-quality rule
// from the template it came from, leaving the rule and its run history intact.
package detach_dq_rule_from_template

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

// Status is the overall outcome of a call.
type Status string

const (
	// StatusPreview means nothing was written: the confirm checkpoint is closed.
	StatusPreview Status = "preview"
	// StatusSuccess means the rule was detached.
	StatusSuccess Status = "success"
	// StatusNeedsInput means the inputs failed validation before any call.
	StatusNeedsInput Status = "needs_input"
	// StatusError means the detach could not be performed.
	StatusError Status = "error"
)

// Input identifies the deployment to detach. A deployment has no id of its own,
// so it takes all three parts of its identity.
type Input struct {
	TemplateName     string `json:"template_name" jsonschema:"Required. Exact name of the rule template the rule is currently linked to, as shown by list_data_quality_rule_templates. Case-sensitive."`
	JobName          string `json:"job_name" jsonschema:"Required. Name of the data-quality job (also called a dataset - a saved set of checks over one database table) the rule runs on, e.g. 'PUBLIC.CUSTOMERS'."`
	DeployedRuleName string `json:"deployed_rule_name" jsonschema:"Required. Name of the deployed rule to detach, as shown by list_data_quality_rule_template_deployments. Deployed rules are named {templateName}_{columnName} at deploy time; job_name plus this name is the deployment's identity, since the API exposes no deployment id."`
	Confirm          bool   `json:"confirm,omitempty" jsonschema:"Safety checkpoint. false (default) returns a PREVIEW of exactly what would be detached and changes NOTHING; set true to perform the detach after the user has approved."`
}

// StandaloneRule is the rule's definition after detaching, read back so the
// caller can see what it now owns outright.
type StandaloneRule struct {
	JobName      string   `json:"jobName" jsonschema:"Job the rule runs on."`
	RuleName     string   `json:"ruleName" jsonschema:"The rule's name, unchanged by detaching."`
	RuleType     string   `json:"ruleType,omitempty" jsonschema:"Monitor type, e.g. FREEFORM_SQL or SIMPLE_SQL."`
	RuleSQL      string   `json:"ruleSql,omitempty" jsonschema:"The rule's SQL, preserved verbatim - detaching does not rewrite it."`
	ColumnName   string   `json:"columnName,omitempty" jsonschema:"Column the rule checks, when it is a single-column check."`
	FilterQuery  string   `json:"filterQuery,omitempty" jsonschema:"Additional WHERE-clause filter on the rule, if any."`
	Dimensions   []string `json:"dimensions,omitempty" jsonschema:"Data quality dimensions the rule contributes to."`
	Tolerance    int      `json:"tolerance" jsonschema:"Count of breaking rows allowed before the rule is judged as failing."`
	Active       bool     `json:"active" jsonschema:"Whether the rule is active."`
	Suppressed   bool     `json:"suppressed" jsonschema:"Whether the rule is suppressed: kept on the job but excluded from scoring."`
	StillLinked  bool     `json:"stillLinked" jsonschema:"Whether the rule still reports a source template. Expected false after a successful detach; true would mean the link survived and is worth reporting."`
	TemplateLink string   `json:"templateLink,omitempty" jsonschema:"The template id the rule still points at, when stillLinked is true. Absent on a clean detach."`
}

// Detachment describes the operation, previewed or performed.
type Detachment struct {
	TemplateName     string          `json:"templateName" jsonschema:"Template the rule is being unlinked from."`
	JobName          string          `json:"jobName" jsonschema:"Job the rule runs on."`
	DeployedRuleName string          `json:"deployedRuleName" jsonschema:"Rule being detached."`
	Detached         bool            `json:"detached" jsonschema:"True once the detach has actually been performed; false in a preview."`
	Rule             *StandaloneRule `json:"rule,omitempty" jsonschema:"The rule's definition read back after detaching. Absent in a preview, and absent when the read-back itself failed - which does not undo the detach."`
}

// Output is the typed response.
type Output struct {
	Status     Status      `json:"status" jsonschema:"'preview' when confirm was false and nothing was written; 'success' when the rule was detached; 'needs_input' for bad inputs; 'error' for downstream data-quality failures."`
	Message    string      `json:"message" jsonschema:"Human-readable summary of what would happen, or what happened."`
	Detachment *Detachment `json:"detachment,omitempty" jsonschema:"The previewed or performed detachment."`
	Guidance   string      `json:"guidance,omitempty" jsonschema:"What to do next: on preview, how to confirm; on needs_input/error, how to correct the call."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "detach_data_quality_rule_from_template",
		Title: "Detach Data Quality Rule From Template",
		Description: "Soft-unlinks one deployed Collibra data-quality rule from the template it came from, turning it into a " +
			"standalone rule. A rule template is a reusable parameterized SQL pattern; deploying it creates concrete rules " +
			"(Collibra calls a rule a \"monitor\" - a single data-quality check on a table's data) across many jobs. A \"job\" " +
			"(also called a \"dataset\") is a saved set of checks over ONE database table.\n\n" +
			"This is NOT a delete. The rule, its configuration and every historical run result are preserved exactly as they " +
			"are; only the link to the template is cleared. What changes is the future: once detached, the rule stops receiving " +
			"cascades, so an update_data_quality_rule_template that rewrites the template's SQL, and a " +
			"delete_data_quality_rule_template run with cascade, will no longer touch it. Detach a rule when you want it to " +
			"diverge from its template and keep its history.\n\n" +
			"The rule is addressed by three things, because a deployment has no id of its own: the template it is linked to, " +
			"the job it runs on, and its deployed rule name. All three come from " +
			"list_data_quality_rule_template_deployments, which is the tool to run first if you do not already know them.\n\n" +
			"Confirm checkpoint: confirm=false (the default) verifies the rule really is deployed from that template and " +
			"returns a preview of exactly what would be unlinked, writing nothing. Call again with confirm=true to perform it. " +
			"There is no re-link operation, so detaching is one-way - a detached rule can only be reconnected by redeploying " +
			"the template onto it, which overwrites its SQL.\n\n" +
			"After a successful detach the rule is read back and returned, so its standalone definition is visible and the " +
			"cleared template link can be confirmed. The rule also stops appearing in " +
			"list_data_quality_rule_template_deployments for that template.\n\n" +
			"Requires the Data Quality > Deploy Templates permission, the same scope that governs deploying.\n\n" +
			"Example user requests: \"Detach this rule from its template\"; \"I want to customise this rule without the " +
			"template overwriting it\"; \"Unlink Null_Check_email on PUBLIC.CUSTOMERS\"; \"Stop template changes affecting " +
			"this one rule\"; \"Let this rule go its own way but keep its history\"",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		// Not destructive: nothing is deleted and the run history survives. It is
		// still a write, hence no ReadOnlyHint and no IdempotentHint — a second
		// call fails, because the rule is no longer linked.
		Annotations: &mcp.ToolAnnotations{DestructiveHint: chip.Ptr(false), OpenWorldHint: chip.Ptr(false)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		template := strings.TrimSpace(input.TemplateName)
		job := strings.TrimSpace(input.JobName)
		rule := strings.TrimSpace(input.DeployedRuleName)

		if missing := missingFields(template, job, rule); missing != nil {
			return *missing, nil
		}

		if !input.Confirm {
			return preview(ctx, collibraClient, template, job, rule), nil
		}

		if err := clients.DetachDQRuleFromTemplate(ctx, collibraClient, template, job, rule); err != nil {
			return detachError(err, template, job, rule), nil
		}
		return detached(ctx, collibraClient, template, job, rule), nil
	}
}

func missingFields(template, job, rule string) *Output {
	var missing []string
	if template == "" {
		missing = append(missing, "template_name")
	}
	if job == "" {
		missing = append(missing, "job_name")
	}
	if rule == "" {
		missing = append(missing, "deployed_rule_name")
	}
	if len(missing) == 0 {
		return nil
	}
	return &Output{
		Status:  StatusNeedsInput,
		Message: fmt.Sprintf("Missing required input: %s.", strings.Join(missing, ", ")),
		Guidance: "A deployment is identified by all three of template_name, job_name and deployed_rule_name — it has no id " +
			"of its own. list_data_quality_rule_template_deployments returns all three for every rule deployed from a template.",
	}
}

// preview checks the rule really is deployed from this template before promising
// to detach it. Doing the check here turns the API's opaque 400 into an answer
// the caller can act on, and stops the user approving a no-op.
func preview(ctx context.Context, client *http.Client, template, job, rule string) Output {
	list, err := clients.ListDQRuleTemplateDeployments(ctx, client, template)
	if err != nil {
		return detachError(err, template, job, rule)
	}
	for _, d := range list.Results {
		if d.JobName == job && d.DeployedRuleName == rule {
			return Output{
				Status: StatusPreview,
				Message: fmt.Sprintf("Would detach rule %q on job %q from template %q. Nothing has changed yet.",
					rule, job, template),
				Detachment: &Detachment{TemplateName: template, JobName: job, DeployedRuleName: rule},
				Guidance: fmt.Sprintf("The rule and all of its run history are preserved; only the template link is cleared, "+
					"so future changes to %q stop reaching it. This cannot be undone except by redeploying the template onto "+
					"the rule, which would overwrite its SQL. Call again with confirm=true to detach.", template),
			}
		}
	}
	return Output{
		Status: StatusError,
		Message: fmt.Sprintf("Rule %q on job %q is not currently deployed from template %q, so there is nothing to detach.",
			rule, job, template),
		Guidance: "Either the rule is already standalone, or it belongs to a different template, or the job/rule name is " +
			"wrong. Run list_data_quality_rule_template_deployments for this template to see what is actually linked to it.",
	}
}

// detached reports a completed detach. The read-back is best-effort: the write
// has already committed, so a failure to re-read must not be reported as though
// the detach failed.
func detached(ctx context.Context, client *http.Client, template, job, rule string) Output {
	out := Detachment{TemplateName: template, JobName: job, DeployedRuleName: rule, Detached: true}
	message := fmt.Sprintf("Detached rule %q on job %q from template %q. The rule and its run history are unchanged.",
		rule, job, template)

	current, err := clients.GetDQRule(ctx, client, job, rule)
	if err != nil {
		return Output{
			Status:     StatusSuccess,
			Message:    message,
			Detachment: &out,
			Guidance: "The detach succeeded. Reading the rule back afterwards failed, so its standalone definition is not " +
				"included here — use get_data_quality_rule to see it. This does not affect the detach.",
		}
	}
	out.Rule = &StandaloneRule{
		JobName:      job,
		RuleName:     current.MonitorName,
		RuleType:     current.MonitorType,
		RuleSQL:      current.MonitorValue,
		ColumnName:   current.ColumnName,
		FilterQuery:  current.FilterQuery,
		Dimensions:   current.Dimensions,
		Tolerance:    current.Tolerance,
		Active:       current.IsActive == 1,
		Suppressed:   current.IsSuppressed,
		StillLinked:  current.TemplateID != "",
		TemplateLink: current.TemplateID,
	}
	guidance := fmt.Sprintf("The rule is now standalone: changes to template %q, including a cascading update or delete, no "+
		"longer reach it, and it no longer appears in list_data_quality_rule_template_deployments for that template.", template)
	if out.Rule.StillLinked {
		// Should not happen; surface it rather than let a stale link pass as clean.
		guidance = fmt.Sprintf("The detach call succeeded but the rule still reports a source template (%s). Re-read it with "+
			"get_data_quality_rule and report this if it persists.", current.TemplateID)
	}
	return Output{Status: StatusSuccess, Message: message, Detachment: &out, Guidance: guidance}
}

func detachError(err error, template, job, rule string) Output {
	switch {
	case errors.Is(err, clients.ErrDQRuleNotFromTemplate):
		return Output{
			Status: StatusError,
			Message: fmt.Sprintf("Rule %q on job %q is not linked to template %q, so it cannot be detached from it.",
				rule, job, template),
			Guidance: "The data-quality API reports the same condition for a rule that is already standalone and for one that " +
				"belongs to a different template, so it cannot say which applies here. Run " +
				"list_data_quality_rule_template_deployments for this template to see what is actually linked to it.",
		}
	case errors.Is(err, clients.ErrDQRuleNotFound), errors.Is(err, clients.ErrDQRuleTemplateNotFound):
		return Output{
			Status:  StatusError,
			Message: fmt.Sprintf("No template named %q, or no rule named %q on job %q.", template, rule, job),
			Guidance: "The API answers the same way for an unknown template and an unknown rule, so check both. Names are " +
				"case-sensitive: use list_data_quality_rule_templates for the template and " +
				"list_data_quality_rule_template_deployments for the job and rule names.",
		}
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "missing permission"):
		return Output{
			Status:   StatusError,
			Message:  fmt.Sprintf("You do not have permission to detach rules from template %q.", template),
			Guidance: "Detaching requires the Data Quality > Deploy Templates permission, the same scope that governs deploying. Ask an administrator for it, then retry.",
		}
	case strings.Contains(msg, "status 401"):
		return Output{
			Status:   StatusError,
			Message:  "Not authenticated to the data-quality API (HTTP 401).",
			Guidance: "Your Collibra session/token is missing or expired - re-authenticate and retry.",
		}
	case strings.Contains(msg, "status 422"):
		return Output{
			Status:   StatusError,
			Message:  fmt.Sprintf("The data-quality API could not process the detach for rule %q: %s", rule, safeErr(err)),
			Guidance: "The request was well-formed but rejected as invalid. Correct the inputs; retrying it unchanged will fail identically.",
		}
	default:
		return Output{
			Status:   StatusError,
			Message:  fmt.Sprintf("Failed to detach rule %q on job %q from template %q: %s", rule, job, template, safeErr(err)),
			Guidance: "This is likely a server-side or transport error. The rule may or may not have been detached — check with list_data_quality_rule_template_deployments before retrying.",
		}
	}
}

// maxErrMessage caps how much downstream text is forwarded to the model. The DQ
// client wraps the whole non-2xx body into the error and engine failures echo the
// offending value, so an unbounded pass-through is a channel for customer data.
const maxErrMessage = 200

func safeErr(err error) string {
	if err == nil {
		return "no additional detail"
	}
	msg := strings.TrimSpace(err.Error())
	if msg == "" {
		return "no additional detail"
	}
	if len(msg) > maxErrMessage {
		return msg[:maxErrMessage] + "… (truncated)"
	}
	return msg
}
