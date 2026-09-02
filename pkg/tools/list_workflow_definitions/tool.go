// Package list_workflow_definitions implements the list_workflow_definitions MCP tool: discovers
// workflow definitions (the templates start_workflow starts instances from) on the Collibra
// instance, scoped to a specific asset/domain/community or to the global scope — see validate for
// why an unscoped "list everything" mode isn't offered.
//
// Always enabled-only — candidate, not settled: the ticket's AC says "returns startable workflow
// definitions" and a disabled workflow is never startable by anyone, so for now this tool never
// exposes an enabled=false option at all, in either scope. The API itself supports it fine (see
// FindWorkflowDefinitionsFilter.Enabled in pkg/clients/workflow_client.go, kept general there); the
// restriction is a tool-level policy choice, raised with the workflow team as something to
// revisit (e.g. if auditing disabled workflows is ever needed), not a technical constraint.
package list_workflow_definitions

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/validation"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// OutputStatus is the overall outcome of a list_workflow_definitions call. Typed rather than a
// bare Go error so a caller can tell "you asked for something impossible" from "Collibra refused"
// and react differently — and so this tool matches start_workflow, which it is almost always used
// with (TOOL_CONTRIBUTION_STANDARDS.md §6.1, §6.5, §6.6).
type OutputStatus string

const (
	// StatusSuccess means the lookup ran; results may still be empty.
	StatusSuccess OutputStatus = "success"
	// StatusValidationError means the input itself was refused, before any network call.
	StatusValidationError OutputStatus = "validation_error"
	// StatusError means Collibra refused or could not answer the request.
	StatusError OutputStatus = "error"
)

// defaultLimit matches Collibra's own workflow-definitions list page size, so results read like
// what a user already sees in the product rather than an arbitrarily large dump.
const defaultLimit = 50

// maxServerLimit is the search endpoint's own documented hard cap. Used to pull the whole
// (already authorization-filtered) set in one call when nameContains forces client-side matching
// — see the resource-scoped branch in handler.
const maxServerLimit = 1000

// Input mirrors the server's own find-workflow-definitions filters. Exactly one of
// AssetID/DomainID/CommunityID/Global(true) is required — see the tool description for why an
// instance-wide "everything" mode isn't offered.
type Input struct {
	AssetID     string `json:"assetId,omitempty" jsonschema:"UUID of an asset — resolve it first via search_asset_keyword / get_asset_details. Returns workflows Collibra considers applicable to THAT asset (matched by its type and status) AND already filtered to ones the current user is authorized to start. Required unless domainId, communityId, or global=true is supplied instead."`
	DomainID    string `json:"domainId,omitempty" jsonschema:"UUID of a domain. Same authorization/applicability filtering as assetId, scoped to the domain. Required unless assetId, communityId, or global=true is supplied instead."`
	CommunityID string `json:"communityId,omitempty" jsonschema:"UUID of a community. Same authorization/applicability filtering as assetId, scoped to the community. Required unless assetId, domainId, or global=true is supplied instead."`

	// Global, not Enabled: this tool always requires enabled — see the package comment. There is
	// deliberately no way to ask for disabled workflows; the ticket's AC is "returns startable
	// workflow definitions" and a disabled one is never startable, by anyone.
	Global *bool `json:"global,omitempty" jsonschema:"Set true for workflows that concern no specific resource (e.g. 'propose a new business term'). Required (true) unless assetId, domainId, or communityId is supplied instead. Returns every global workflow the current user is permitted to start. Some of them are normally triggered automatically rather than by a person — e.g. on a timer, or by another workflow — and are startable by hand but rarely meant to be; if one looks like that (an escalation, a scheduled job, a sub-process), say so instead of offering to start it."`

	NameContains string `json:"nameContains,omitempty" jsonschema:"Optional. Case-INsensitive partial match, tried against both the workflow's name and the label the product shows on its start button — those two often differ, so quoting either works. E.g. 'business term' matches the workflow named 'Propose New Business Term' whose button reads 'Propose Business Term'."`

	// No DescriptionContains: server-side description filtering only takes effect on the raw,
	// unauthorized query (the one path this tool never uses — see validate); the authorized resource-
	// scoped path has no description predicate at all, and the GraphQL global path takes no
	// filter arguments whatsoever. A fetch-everything-and-filter-client-side workaround was
	// built and then reverted — not worth the cost for a free-text field a caller would rarely
	// guess correctly; nameContains plus reading the returned description covers the same need.

	Offset int `json:"offset,omitempty" jsonschema:"Optional. Index of the first result to return, for paging past a truncated response (see hasMore). Default: 0."`
	Limit  int `json:"limit,omitempty" jsonschema:"Optional. Maximum number of results to return. Default: 50."`
}

// WorkflowDefinitionSummary is one workflow definition — a reusable template start_workflow
// starts instances (actual running processes) from, identified by workflowDefinitionId.
type WorkflowDefinitionSummary struct {
	WorkflowDefinitionID string `json:"workflowDefinitionId" jsonschema:"Pass this to start_workflow to inspect or start this workflow."`
	Name                 string `json:"name" jsonschema:"The workflow definition's name, as an administrator sees it."`
	StartLabel           string `json:"startLabel,omitempty" jsonschema:"The caption the product puts on this workflow's start button or Create-menu entry — what an end user actually sees and is likely to call it. Often differs from name; prefer it when talking to the user. Omitted when it adds nothing beyond name."`
	Description          string `json:"description,omitempty" jsonschema:"Human-authored explanation of what this workflow actually does — the main signal for picking the right one among several similarly-named options. May be empty if the workflow was deployed without one."`
	Scope                string `json:"scope" jsonschema:"ASSET, DOMAIN or COMMUNITY (concerns a specific resource — pass its id as businessItemId to start_workflow), or GLOBAL (concerns no specific resource)."`
	FormRequired         bool   `json:"formRequired" jsonschema:"Whether starting this workflow requires filling in start-form fields — start_workflow surfaces them if so."`
}

// Output is the typed response.
type Output struct {
	Status  OutputStatus `json:"status" jsonschema:"'success' when the lookup ran (results may still be empty); 'validation_error' when the request was refused before any call to Collibra; 'error' when Collibra refused or could not answer."`
	Message string       `json:"message" jsonschema:"Human-readable outcome. On validation_error and error this says what went wrong and what would work instead — read it rather than retrying blindly."`

	Total   int                         `json:"total" jsonschema:"Total number of workflow definitions matching the filters — may be larger than len(results) if the response was truncated by limit; see hasMore."`
	HasMore bool                        `json:"hasMore" jsonschema:"True when more results exist beyond this page — re-call with a larger offset (offset + len(results)) to see them."`
	Results []WorkflowDefinitionSummary `json:"results" jsonschema:"The matching workflow definitions."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "list_workflow_definitions",
		Title: "List Workflow Definitions",
		Description: "Answers \"what workflows can I start\" — lists workflow definitions, the reusable templates " +
			"start_workflow starts instances from. A 'workflow definition' is a governed business process (built-in, " +
			"like requesting dataset access, OR customer-built in Collibra's Workflow Designer) that Collibra routes to " +
			"approvers when started. NOT a data pipeline/ETL definition, and NOT a running instance.\n\n" +
			"Requires exactly one of assetId, domainId, communityId, or global=true — every lane returns " +
			"only what Collibra confirms this user can start, so treat the results as real options rather " +
			"than candidates still to be checked. For a fully open \"what can " +
			"I start\" with no resource in mind, call once with global=true; to also cover a specific resource, " +
			"call again with its id and combine both results — the server has no single call that returns both.\n\n" +
			"Example user requests: \"What workflows can I start?\"; \"What workflows can I start for this asset?\"; " +
			"\"List workflows with 'approval' in the name for this domain\".",
		Handler: handler(collibraClient),
		// No extra dgc.* scope: reading workflow definitions needs nothing beyond what an
		// authenticated Collibra user already has. (The kg.view-all in the published API spec's
		// @SecurityRequirement belongs to the OAuth2 client-credentials flow, a different
		// namespace — see pkg/clients/workflow_client.go's package comment.)
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: chip.Ptr(false), IdempotentHint: true, OpenWorldHint: chip.Ptr(false)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if invalid := validate(input); invalid != nil {
			return *invalid, nil
		}

		limit := input.Limit
		if limit <= 0 {
			limit = defaultLimit
		}
		global := input.Global != nil && *input.Global

		needle := strings.TrimSpace(input.NameContains)

		var defs []clients.WorkflowDefinition
		var total int
		// truncated records that the server held more rows than we were allowed to scan, so the
		// answer is known-incomplete and must not be presented as a complete one.
		truncated := false
		if global {
			// The non-deprecated, authorization-checked source for global workflows — see
			// ListGlobalWorkflowDefinitions's doc comment. It takes no name/pagination arguments
			// of its own, so filtering/paging happens here.
			all, code, err := clients.ListGlobalWorkflowDefinitions(ctx, collibraClient)
			if err != nil {
				return lookupError(code, err, input), nil
			}
			filtered := filterByName(all, needle)
			total = len(filtered)
			defs = paginate(filtered, input.Offset, limit)
		} else {
			// Resource-scoped (assetId/domainId/communityId) — authorization-checked server-side,
			// Enabled is always true — see the package comment.
			// `name` is deliberately NOT forwarded: the server would match it case-sensitively and
			// against the name only, which is not what this tool promises (see filterByName). So
			// when nameContains is set, pull the whole already-authorization-filtered set and
			// match here instead. That set is small — it is the workflows applicable to one
			// resource — and maxServerLimit is the endpoint's own cap.
			fetchOffset, fetchLimit := input.Offset, limit
			if needle != "" {
				fetchOffset, fetchLimit = 0, maxServerLimit
			}
			page, code, err := clients.FindWorkflowDefinitions(ctx, collibraClient, clients.FindWorkflowDefinitionsFilter{
				AssetID:     strings.TrimSpace(input.AssetID),
				DomainID:    strings.TrimSpace(input.DomainID),
				CommunityID: strings.TrimSpace(input.CommunityID),
				Enabled:     chip.Ptr(true),
				Offset:      fetchOffset,
				Limit:       fetchLimit,
			})
			if err != nil {
				return lookupError(code, err, input), nil
			}
			if needle != "" {
				// The server held more rows than it handed us, so the local match ran over only a
				// prefix of the authorized set. Reporting "found N" from a partial scan would be a
				// confident answer to a question we did not fully ask.
				truncated = page.Total > len(page.Results)
				filtered := filterByName(page.Results, needle)
				total = len(filtered)
				defs = paginate(filtered, input.Offset, limit)
			} else {
				defs = page.Results
				total = page.Total
			}
		}

		results := make([]WorkflowDefinitionSummary, len(defs))
		for i, def := range defs {
			results[i] = WorkflowDefinitionSummary{
				WorkflowDefinitionID: def.ID,
				Name:                 def.Name,
				StartLabel:           startLabelIfDistinct(def),
				Description:          def.Description,
				Scope:                def.BusinessItemResourceType,
				FormRequired:         def.FormRequired,
			}
		}

		// Saturating on purpose: offset is caller-controlled and a plain sum overflows to negative
		// at extreme values, flipping the comparison and advertising pages past the last one.
		seen := total
		if input.Offset <= total-len(results) {
			seen = input.Offset + len(results)
		}

		return Output{
			Status:  StatusSuccess,
			Message: summarize(len(results), total, seen, needle, truncated, input),
			Total:   total,
			HasMore: seen < total,
			Results: results,
		}, nil
	}
}

// filterByName matches nameContains against a definition's name AND its start label,
// case-insensitively — on BOTH lanes, so the two behave identically.
//
// Neither property comes from the server. The scoped lane's server-side name filter is a plain
// case-sensitive Java String.contains over the name only, and the global GraphQL field takes no
// name argument at all; the product's own workflow search, meanwhile, is case-insensitive over
// name OR start label. Matching the server would mean a user who types what they read on screen
// gets nothing back: live, 8 of 26 global definitions have a start label that differs from the
// name ("Propose New Business Term" vs "Propose Business Term", "Issue Creation" vs "Log Issue"),
// and lowercase input misses everything. So the filter is applied here instead, and the scoped
// lane deliberately does not forward `name` to the server — see handler.
func filterByName(defs []clients.WorkflowDefinition, nameContains string) []clients.WorkflowDefinition {
	if nameContains == "" {
		return defs
	}
	needle := strings.ToLower(nameContains)
	filtered := make([]clients.WorkflowDefinition, 0, len(defs))
	for _, d := range defs {
		if strings.Contains(strings.ToLower(d.Name), needle) ||
			strings.Contains(strings.ToLower(d.StartLabel), needle) {
			filtered = append(filtered, d)
		}
	}
	return filtered
}

// startLabelIfDistinct returns the start label only when it actually says something the name does
// not — most definitions set it to the same string, and echoing a duplicate for every row is noise
// the model has to read past.
func startLabelIfDistinct(def clients.WorkflowDefinition) string {
	if def.StartLabel == def.Name {
		return ""
	}
	return def.StartLabel
}

// paginate applies offset/limit client-side, for the same reason as filterByName.
//
// The negative-offset guard is deliberately redundant with validate: a slice expression with a
// negative bound panics, and nothing in chip or the MCP SDK recovers from a panic in a tool
// handler, so the blast radius of one bad offset reaching here is the whole server process, not
// one failed call. Cheap insurance for a crash-class bug.
func paginate(defs []clients.WorkflowDefinition, offset, limit int) []clients.WorkflowDefinition {
	if offset < 0 {
		offset = 0
	}
	if offset >= len(defs) {
		return nil
	}
	defs = defs[offset:]
	if limit > 0 && limit < len(defs) {
		defs = defs[:limit]
	}
	return defs
}

// validate enforces UUID formats and that exactly one of assetId/domainId/communityId/global=true
// is supplied, before any network call (TOOL_CONTRIBUTION_STANDARDS.md §6.1). Calling with none of
// these would otherwise silently return every workflow definition on the instance unfiltered by
// the current user's authorization (with no resource the server falls back to a raw, unchecked
// query) — exactly the "startable" guarantee this tool exists to provide, so it is refused rather
// than silently degraded. Calling with a resource AND global=true would otherwise return an empty
// page, which reads as "no workflows" rather than "bad input".
//
// It also rejects a negative offset/limit. That is not cosmetic: on the global lane offset feeds
// paginate's slice expression, and a negative one panics — which, with no recover anywhere in
// chip or the MCP SDK, takes the whole server process down rather than failing one call.
func validate(input Input) *Output {
	for _, f := range []struct{ name, value string }{
		{"assetId", input.AssetID}, {"domainId", input.DomainID}, {"communityId", input.CommunityID},
	} {
		if err := validation.UUIDOptional(f.name, f.value); err != nil {
			return &Output{
				Status:  StatusValidationError,
				Message: err.Error() + " Resolve the resource first with search_asset_keyword or get_asset_details and pass the UUID it returns.",
			}
		}
	}

	scopeCount := 0
	for _, id := range []string{input.AssetID, input.DomainID, input.CommunityID} {
		if strings.TrimSpace(id) != "" {
			scopeCount++
		}
	}
	if scopeCount > 1 {
		return &Output{
			Status:  StatusValidationError,
			Message: fmt.Sprintf("Supply only ONE of assetId, domainId or communityId (got %d). Each scopes the list to a single resource and Collibra evaluates applicability per resource, so combining them does not mean \"either\" — call this tool once per resource and combine the results.", scopeCount),
		}
	}
	scoped := scopeCount > 0
	global := input.Global != nil && *input.Global
	if scoped && global {
		return &Output{
			Status:  StatusValidationError,
			Message: "assetId/domainId/communityId cannot be combined with global=true in one call — Collibra returns an empty result for that exact combination. To see both the workflows applicable to this resource AND global ones, call this tool twice: once with the resource, once with global=true.",
		}
	}
	if !scoped && !global {
		return &Output{
			Status:  StatusValidationError,
			Message: "Supply assetId, domainId or communityId to see workflows applicable to that resource, or global=true for workflows tied to no specific resource. Listing every workflow definition regardless of scope is not supported: only these filters are checked against what the current user is actually allowed to start.",
		}
	}
	if input.Offset < 0 {
		return &Output{
			Status:  StatusValidationError,
			Message: fmt.Sprintf("offset must be 0 or greater (got %d). It is the index of the first result: omit it for the first page, then pass the previous offset plus the number of results returned.", input.Offset),
		}
	}
	if input.Limit < 0 || input.Limit > maxServerLimit {
		return &Output{
			Status:  StatusValidationError,
			Message: fmt.Sprintf("limit must be between 0 and %d (got %d). Omit it (or pass 0) for the default of %d; %d is the most the search endpoint will return.", maxServerLimit, input.Limit, defaultLimit, maxServerLimit),
		}
	}
	return nil
}

// lookupError maps a failed resource-scoped lookup onto a typed status. 403 and 404 are called out
// because their remedies differ and neither is "retry": a 404 here means the id does not resolve,
// and a 403 means it does but the caller may not read it (§6.3, §6.6).
func lookupError(code int, err error, input Input) Output {
	scope, id := suppliedScope(input)
	switch code {
	case http.StatusNotFound:
		return Output{
			Status:  StatusValidationError,
			Message: fmt.Sprintf("No %s found with id %s. Re-resolve it with search_asset_keyword or get_asset_details — a workflow list cannot be built for a resource that does not exist.", scope, id),
		}
	case http.StatusForbidden:
		return Output{
			Status:  StatusError,
			Message: fmt.Sprintf("You do not have permission to read that %s (HTTP 403), so its workflows cannot be listed. Do not retry — ask a Collibra administrator for access, or try global=true for workflows that need no resource.", scope),
		}
	case 0:
		return Output{
			Status:  StatusError,
			Message: fmt.Sprintf("Could not reach Collibra while listing workflows: %v. This is a network/transport failure — retrying is reasonable.", err),
		}
	default:
		return Output{
			Status:  StatusError,
			Message: fmt.Sprintf("Collibra could not list workflows for that %s (HTTP %d): %v", scope, code, err),
		}
	}
}

// suppliedScope names whichever resource lane the caller used, for error messages that point at
// the actual input rather than at a generic "resource".
func suppliedScope(input Input) (string, string) {
	switch {
	case strings.TrimSpace(input.AssetID) != "":
		return "asset", strings.TrimSpace(input.AssetID)
	case strings.TrimSpace(input.DomainID) != "":
		return "domain", strings.TrimSpace(input.DomainID)
	case strings.TrimSpace(input.CommunityID) != "":
		return "community", strings.TrimSpace(input.CommunityID)
	}
	return "resource", ""
}

// summarize states plainly what came back, including the empty case — "no workflows here" and "the
// call failed" must not look alike to the caller.
// summarize takes `seen` — the same running count hasMore is derived from — rather than
// recomputing it. Deriving the prose and the flag from different expressions previously let the
// message say "page on with offset" on the very last page, which invites an endless paging loop.
//
// It also names the filter when one was applied: "no workflow definitions found for asset X" is a
// statement about the resource, and if a name filter is what excluded everything the caller needs
// to know that rather than concluding the resource is empty.
func summarize(shown, total, seen int, needle string, truncated bool, input Input) string {
	scope, id := suppliedScope(input)
	where := "global scope (workflows tied to no specific resource)"
	if id != "" {
		where = fmt.Sprintf("%s %s", scope, id)
	}
	filter := ""
	if needle != "" {
		filter = fmt.Sprintf(" matching %q", needle)
	}
	caveat := ""
	if truncated {
		caveat = fmt.Sprintf(" NOTE: only the first %d definitions could be scanned for this filter, so further matches may exist that are not listed here.", maxServerLimit)
	}

	switch {
	case total == 0 && needle != "":
		return fmt.Sprintf("No startable workflow definitions for %s match %q. Retry without nameContains to see what is available there.%s", where, needle, caveat)
	case total == 0:
		return fmt.Sprintf("No startable workflow definitions found for %s.%s", where, caveat)
	case seen < total:
		return fmt.Sprintf("Showing %d of %d startable workflow definitions%s for %s — page on with offset %d.%s", shown, total, filter, where, seen, caveat)
	}
	return fmt.Sprintf("Found %d startable workflow definition(s)%s for %s.%s", total, filter, where, caveat)
}
