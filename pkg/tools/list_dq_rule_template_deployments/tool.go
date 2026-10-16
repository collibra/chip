// Package list_dq_rule_template_deployments exposes the rules currently deployed
// from a data-quality rule template, so the blast radius of a template change can
// be inspected before the change is made. Read-only.
package list_dq_rule_template_deployments

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
	// StatusSuccess means deployments were read (possibly an empty set).
	StatusSuccess Status = "success"
	// StatusNeedsInput means the inputs failed validation before any read.
	StatusNeedsInput Status = "needs_input"
	// StatusError means the deployments could not be read.
	StatusError Status = "error"
)

const (
	defaultPageSize = 25
	maxPageSize     = 200
)

// Input selects the template and the slice of its deployments to return.
type Input struct {
	TemplateName string `json:"template_name" jsonschema:"Required. Exact name of the rule template whose deployments to list, as shown by list_data_quality_rule_templates. Case-sensitive; this is the template's identifier, not a search term."`
	Page         int    `json:"page,omitempty" jsonschema:"Optional. 1-based page number. Defaults to 1. Paging is applied by this tool after reading the template's full deployment set, because the data-quality API returns all deployments in one response and accepts no paging parameters."`
	PageSize     int    `json:"page_size,omitempty" jsonschema:"Optional. Deployments per page, 1-200. Defaults to 25 so a widely-deployed template does not flood the context; totalDeployments always reports the full count regardless of this value."`
}

// Deployment is one rule deployed from the template.
type Deployment struct {
	JobName          string `json:"jobName" jsonschema:"Data-quality job (also called a dataset - a saved set of checks over one database table) that the rule was deployed onto."`
	DeployedRuleName string `json:"deployedRuleName" jsonschema:"Name of the deployed rule, generated at deploy time as {templateName}_{columnName}. Together with jobName this identifies the deployment - the API exposes no separate deployment id."`
	ColumnName       string `json:"columnName,omitempty" jsonschema:"Column the rule checks. Absent for a template deployed at table level rather than against a {{column}} placeholder."`
	LastRun          string `json:"lastRun,omitempty" jsonschema:"ISO-8601 UTC timestamp of the rule's most recent run. Absent when the rule has never run, which is normal for a freshly deployed rule."`
	LastRunStatus    string `json:"lastRunStatus,omitempty" jsonschema:"Latest evaluation outcome of the rule: passing, breaking, exception, suppressed or pending_run. This is how the rule last scored, NOT whether the deployment itself succeeded - every row returned here is by definition deployed. Absent when the rule has never run."`
	Creator          string `json:"creator,omitempty" jsonschema:"Username of whoever deployed the rule, when the API reports one."`
	ConnectionID     string `json:"connectionId,omitempty" jsonschema:"UUID of the job's Edge connection."`
	EdgeSiteID       string `json:"edgeSiteId,omitempty" jsonschema:"UUID of the Edge site reaching that connection."`
}

// Deployments is the page returned on success.
type Deployments struct {
	TemplateName     string       `json:"templateName" jsonschema:"The template these deployments belong to."`
	TotalDeployments int64        `json:"totalDeployments" jsonschema:"Total rules deployed from this template, across all pages. This is the blast radius of a change to the template."`
	Page             int          `json:"page" jsonschema:"1-based page number of this page."`
	PageSize         int          `json:"pageSize" jsonschema:"Maximum deployments in this page."`
	TotalPages       int          `json:"totalPages" jsonschema:"Number of pages at this page size."`
	HasMore          bool         `json:"hasMore" jsonschema:"True when further pages remain. Request them by incrementing page, or raise page_size."`
	Results          []Deployment `json:"results" jsonschema:"The deployments in this page. Empty when the template has no deployments at all, or when page is past the end."`
}

// Output is the typed response.
type Output struct {
	Status      Status       `json:"status" jsonschema:"'success' when deployments were read, including when the template has none; 'needs_input' for bad inputs; 'error' for downstream data-quality failures."`
	Message     string       `json:"message" jsonschema:"Human-readable summary, including the total number of deployments."`
	Deployments *Deployments `json:"deployments,omitempty" jsonschema:"The requested page, on success."`
	Guidance    string       `json:"guidance,omitempty" jsonschema:"What to do next: on success with more pages, how to see the rest; on needs_input/error, how to correct the call."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "list_data_quality_rule_template_deployments",
		Title: "List Data Quality Rule Template Deployments",
		Description: "Lists the rules currently deployed from one Collibra data-quality rule template, so the blast radius of " +
			"changing or deleting that template can be seen before the change is made. A rule template is a reusable " +
			"parameterized SQL pattern; deploying it instantiates concrete rules (Collibra calls a rule a \"monitor\" - a single " +
			"data-quality check on a table's data) across many jobs. A \"job\" (also called a \"dataset\") is a saved set of " +
			"checks over ONE database table.\n\n" +
			"Each entry gives the job the rule runs on, the deployed rule's generated name, the column it checks, and how that " +
			"rule last evaluated. A deployment has no id of its own: jobName plus deployedRuleName is its identity, and is what " +
			"the detach operation takes.\n\n" +
			"Use this before update_data_quality_rule_template or delete_data_quality_rule_template: an update ALWAYS cascades to " +
			"every deployed rule, and a delete refuses unless cascade is set. This tool answers how many rules that is, and which.\n\n" +
			"Prerequisite: template_name is the template's exact name, from list_data_quality_rule_templates. It is not a search " +
			"term - a name that does not match exactly returns a not-found error.\n\n" +
			"Not to be confused with: list_data_quality_rule_templates, which lists the templates themselves rather than what has " +
			"been deployed from one; and find_data_quality_rules, which searches rules across jobs regardless of whether a " +
			"template produced them. Reach for this tool when the question is specifically about one template's reach.\n\n" +
			"Returns 25 deployments per page by default, and always reports the full total so the answer to \"how many rules " +
			"would this affect\" does not depend on the page size. When more remain, ask the user whether to page through the " +
			"rest or narrow to a particular job or rule rather than fetching everything unprompted.\n\n" +
			"Read-only: it never deploys, detaches or changes a rule or template. Requires the Data Quality > Deploy Templates " +
			"permission - note this is the deploy permission, not plain template-read, so a user who can list templates may " +
			"still be refused here.\n\n" +
			"Example user requests: \"What rules use the Null Check template?\"; \"How many rules would change if I edit this " +
			"template?\"; \"Is this template safe to delete?\"; \"Show me everything deployed from Row Count Range\"; " +
			"\"What's the blast radius here?\"",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: chip.Ptr(false), IdempotentHint: true, OpenWorldHint: chip.Ptr(false)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		name := strings.TrimSpace(input.TemplateName)
		if name == "" {
			return Output{
				Status:   StatusNeedsInput,
				Message:  "Provide the rule template whose deployments to list.",
				Guidance: "Supply template_name - the template's exact name. Use list_data_quality_rule_templates to find it.",
			}, nil
		}
		page, pageSize, bad := resolvePaging(input.Page, input.PageSize)
		if bad != nil {
			return *bad, nil
		}

		list, err := clients.ListDQRuleTemplateDeployments(ctx, collibraClient, name)
		if err != nil {
			return lookupError(err, name), nil
		}
		return pageOf(list, name, page, pageSize), nil
	}
}

// resolvePaging validates page/page_size and applies defaults. Validation runs
// before the network call so a bad page never costs a request.
func resolvePaging(page, pageSize int) (int, int, *Output) {
	if page < 0 {
		return 0, 0, &Output{
			Status:   StatusNeedsInput,
			Message:  fmt.Sprintf("page must be 1 or greater, got %d.", page),
			Guidance: "Pages are 1-based. Omit page for the first page.",
		}
	}
	if pageSize < 0 || pageSize > maxPageSize {
		return 0, 0, &Output{
			Status:   StatusNeedsInput,
			Message:  fmt.Sprintf("page_size must be between 1 and %d, got %d.", maxPageSize, pageSize),
			Guidance: fmt.Sprintf("Omit page_size for the default of %d, or pass a value within 1-%d.", defaultPageSize, maxPageSize),
		}
	}
	if page == 0 {
		page = 1
	}
	if pageSize == 0 {
		pageSize = defaultPageSize
	}
	return page, pageSize, nil
}

// pageOf slices the full deployment set the API returned. The endpoint accepts no
// paging parameters, so this bound exists to keep a widely-deployed template from
// flooding the model's context, not to save a round trip.
func pageOf(list *clients.DQTemplateDeploymentList, name string, page, pageSize int) Output {
	total := int64(len(list.Results))
	out := Deployments{
		TemplateName:     name,
		TotalDeployments: total,
		Page:             page,
		PageSize:         pageSize,
		Results:          []Deployment{},
	}
	out.TotalPages = int((total + int64(pageSize) - 1) / int64(pageSize))

	if total == 0 {
		return Output{
			Status:      StatusSuccess,
			Message:     fmt.Sprintf("Rule template %q has no deployed rules.", name),
			Deployments: &out,
			Guidance:    "Nothing is deployed from this template, so updating or deleting it affects no existing rule.",
		}
	}

	start := (page - 1) * pageSize
	if int64(start) >= total {
		return Output{
			Status: StatusSuccess,
			Message: fmt.Sprintf("Page %d is past the end: rule template %q has %d deployed rule(s) across %d page(s) of %d.",
				page, name, total, out.TotalPages, pageSize),
			Deployments: &out,
			Guidance:    fmt.Sprintf("Request a page between 1 and %d.", out.TotalPages),
		}
	}
	end := start + pageSize
	if int64(end) > total {
		end = int(total)
	}
	for _, d := range list.Results[start:end] {
		out.Results = append(out.Results, deployment(d))
	}
	out.HasMore = int64(end) < total

	message := fmt.Sprintf("Rule template %q has %d deployed rule(s); showing %d (page %d of %d).",
		name, total, len(out.Results), page, out.TotalPages)
	guidance := ""
	if out.HasMore {
		guidance = fmt.Sprintf("%d more deployment(s) are not shown. Ask the user whether to page through the rest "+
			"(page %d next, or raise page_size up to %d) or to narrow to a particular job or rule, rather than "+
			"fetching them all unprompted.", total-int64(end), page+1, maxPageSize)
	}
	return Output{Status: StatusSuccess, Message: message, Deployments: &out, Guidance: guidance}
}

func deployment(d clients.DQTemplateDeployment) Deployment {
	out := Deployment{
		JobName:          d.JobName,
		DeployedRuleName: d.DeployedRuleName,
		ColumnName:       d.ColumnName,
		ConnectionID:     d.ConnectionID,
		EdgeSiteID:       d.EdgeSiteID,
	}
	if d.LastRun != nil {
		out.LastRun = *d.LastRun
	}
	if d.LastRunStatus != nil {
		out.LastRunStatus = *d.LastRunStatus
	}
	if d.Creator != nil {
		out.Creator = d.Creator.UserName
	}
	return out
}

func lookupError(err error, name string) Output {
	if errors.Is(err, clients.ErrDQRuleTemplateNotFound) {
		return Output{
			Status:   StatusError,
			Message:  fmt.Sprintf("No rule template named %q was found.", name),
			Guidance: "template_name must match the template's name exactly and is case-sensitive. Use list_data_quality_rule_templates to find the exact name.",
		}
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "missing permission"):
		return Output{
			Status:   StatusError,
			Message:  fmt.Sprintf("You do not have permission to view the deployments of rule template %q.", name),
			Guidance: "Reading deployments requires the Data Quality > Deploy Templates permission, which is separate from template-read access. Ask an administrator for it, then retry.",
		}
	case strings.Contains(msg, "status 401"):
		return Output{
			Status:   StatusError,
			Message:  "Not authenticated to the data-quality API (HTTP 401).",
			Guidance: "Your Collibra session/token is missing or expired - re-authenticate and retry.",
		}
	case strings.Contains(msg, "status 400"), strings.Contains(msg, "status 422"):
		return Output{
			Status:   StatusError,
			Message:  fmt.Sprintf("The data-quality API rejected the deployment lookup for %q: %s", name, safeErr(err)),
			Guidance: "The request was rejected as invalid. Correct template_name; retrying it unchanged will fail identically.",
		}
	case strings.Contains(msg, "decoding response"):
		return Output{
			Status:   StatusError,
			Message:  fmt.Sprintf("The data-quality API returned deployments for %q that could not be read: %s", name, safeErr(err)),
			Guidance: "The response was not in the expected format. This is a service-side problem - contact your Collibra administrator if it persists.",
		}
	default:
		return Output{
			Status:   StatusError,
			Message:  fmt.Sprintf("Failed to list deployments for rule template %q: %s", name, safeErr(err)),
			Guidance: "This is likely a server-side or transport error. Retry shortly; if it persists, contact your Collibra administrator.",
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
