// Package start_collibra_workflow implements the start_collibra_workflow MCP tool — lets an agent
// discover, inspect, and start ANY Collibra workflow (OOTB or customer-built via Workflow
// Designer) on the user's behalf, identified by its workflowDefinitionId (there is no allowlist:
// custom workflows are explicitly in scope, per team alignment 2026-08-20 / DEV-213248).
//
// This is the single-tool delivery of DEV-213248's three requested capabilities
// (list_workflow_definitions, get_workflow_start_form, start_workflow), per
// docs/TOOL_CONTRIBUTION_STANDARDS.md §1.1 ("no prepare_/create_ pairs... couples two tools
// through the LLM, which is not guaranteed to call both") — the same reasoning that shaped this
// tool from the start. See the DEV-213248 comment for the rationale in full.
//
// FORMS: supported. A workflow whose start form has required fields returns status=needs_input
// with the field schema (id, name, type, required, allowed options); re-call with formProperties
// populated. (An earlier phase of this tool refused form-required workflows outright — DEV-213248
// formally settled that ambiguity in favor of support via this introspection step.)
//
// AUTHORIZATION: discovery (no workflowDefinitionId given) filters to workflows the calling user
// is authorized to start, per clients.IsAuthorizedToStart. That check is NOT a complete model of
// Collibra's authorization — role inheritance down the community/domain/asset hierarchy isn't
// modeled — so it only hard-filters on the one reliable, non-inherited signal (the global
// WORKFLOW_START permission) and annotates (never hides) options where the rest of the check is
// uncertain. The real 403 from Collibra at start time remains authoritative.
//
// This single tool spans discovery, form introspection, preview, and start, built around a
// confirm checkpoint: confirm=false (default) never writes; confirm=true starts the workflow.
//
// BACKEND APIS (verified live against premierconfig-dg, dg-uat and compliance, 2026-08-19/20/21):
//
//	GET  /rest/2.0/workflowDefinitions/{workflowDefinitionId}                                    fetch a definition by UUID (OOTB or custom)
//	GET  /rest/2.0/workflowDefinitions?assetIds={uuid}                                            workflows applicable to an asset (discovery)
//	GET  /rest/2.0/workflowDefinitions?limit=N                                                    workflows instance-wide (discovery, no asset)
//	GET  /rest/2.0/workflowDefinitions/workflowDefinition/{workflowDefinitionId}/startFormData     start-form schema (doubled segment, not a typo)
//	GET  /rest/2.0/users/current/globalPermissions                                                current user's global permissions (WORKFLOW_START check)
//	GET  /rest/2.0/users/current                                                                  current user id (for the responsibilities lookup)
//	GET  /rest/2.0/responsibilities?ownerIds={userId}                                              roles the current user holds, globally or per-resource
//	POST /rest/2.0/workflowInstances                                                               start
package start_collibra_workflow

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

// maxOptions caps how many workflows discovery returns in one response.
const maxOptions = 50

type Status string

const (
	StatusOptions         Status = "options"
	StatusNeedsInput      Status = "needs_input"
	StatusPreview         Status = "preview"
	StatusSuccess         Status = "success"
	StatusValidationError Status = "validation_error"
	StatusError           Status = "error"
)

// Input — call with no workflowDefinitionId (optionally with a businessItemId) to discover
// authorized workflows; call again with a chosen workflowDefinitionId to fetch its start-form
// requirements or preview it, then with confirm=true to start it.
type Input struct {
	WorkflowDefinitionID string `json:"workflowDefinitionId,omitempty" jsonschema:"UUID of the workflow to start (OOTB or a customer-built custom workflow) — from a prior options response, or already known. Omit to discover authorized workflows (see businessItemId)."`
	BusinessItemID       string `json:"businessItemId,omitempty" jsonschema:"UUID of the asset (or, for domain-/community-scoped workflows, the domain/community) this workflow concerns — resolve an asset first via search_asset_keyword / get_asset_details. Required when the resolved workflow is scoped to a specific asset/domain/community (see a needs_input response); omit for workflows that concern no specific resource (scope GLOBAL). When workflowDefinitionId is also omitted, supplying an asset's businessItemId scopes discovery to workflows applicable to that asset instead of the instance-wide list."`

	FormProperties map[string]string `json:"formProperties,omitempty" jsonschema:"Values for the workflow's start-form fields, keyed by field id (see a prior needs_input response's formFields[].id) — e.g. {\"usageRequestReason\": \"Need it for Q3 reporting\"}. Omit or partially supply to have the tool report which required fields are still missing. Not needed for workflows with no start form."`

	Confirm bool `json:"confirm,omitempty" jsonschema:"Safety checkpoint. false (default) returns a PREVIEW of exactly what will be started WITHOUT starting anything — review it with the user. true starts the workflow."`
}

// WorkflowOption is one workflow the caller can (or, per AuthorizationNote, may not) start.
type WorkflowOption struct {
	WorkflowDefinitionID string `json:"workflowDefinitionId" jsonschema:"Pass this as workflowDefinitionId to inspect or start this workflow."`
	Name                 string `json:"name" jsonschema:"Human-readable workflow name."`
	Description          string `json:"description,omitempty" jsonschema:"Human-authored explanation of what this workflow actually does — the main signal for picking the right one among several similarly-named options. May be empty if the workflow was deployed without one."`
	Scope                string `json:"scope" jsonschema:"ASSET, DOMAIN, or COMMUNITY (concerns a specific resource — pass its id as businessItemId) or GLOBAL (concerns no specific resource)."`
	FormRequired         bool   `json:"formRequired" jsonschema:"Whether starting this workflow requires filling in start-form fields — call this tool again with this workflowDefinitionId to see them."`
	AuthorizationNote    string `json:"authorizationNote,omitempty" jsonschema:"Empty when the user is confirmed authorized to start this. Otherwise a short note that authorization could not be fully confirmed (e.g. role inheritance isn't checked) — still worth offering, Collibra makes the final call at start time."`
}

// FormField is one field of a workflow's start form, glossed for an agent with no Collibra
// context.
type FormField struct {
	ID       string   `json:"id" jsonschema:"Field key to use in formProperties."`
	Name     string   `json:"name" jsonschema:"Human-readable field label to show/ask the user."`
	Type     string   `json:"type" jsonschema:"Field's data type (e.g. string, textarea, datetime, assetType). Free text unless options is set."`
	Required bool     `json:"required" jsonschema:"Whether this field must be supplied before the workflow can start."`
	Options  []string `json:"options,omitempty" jsonschema:"When set, the field's value MUST be one of these — do not invent other values."`
}

type Output struct {
	Status  Status `json:"status" jsonschema:"options | needs_input | preview | success | validation_error | error."`
	Message string `json:"message" jsonschema:"Human-readable outcome and what to do next."`

	// status=options
	Options []WorkflowOption `json:"options,omitempty" jsonschema:"Workflows available to start. Pick one and re-call with its workflowDefinitionId."`

	// status=needs_input (form)
	FormFields    []FormField `json:"formFields,omitempty" jsonschema:"The workflow's start-form fields. Ask the user for the ones in missingFields and re-call with formProperties populated."`
	MissingFields []string    `json:"missingFields,omitempty" jsonschema:"Required field ids not yet supplied in formProperties."`

	// status=preview / success
	Name           string            `json:"name,omitempty" jsonschema:"Resolved workflow name."`
	Description    string            `json:"description,omitempty" jsonschema:"Human-authored explanation of what this workflow actually does — use this to explain to the user what they're about to start, not just its name."`
	Scope          string            `json:"scope,omitempty" jsonschema:"Resolved workflow scope: ASSET | DOMAIN | COMMUNITY | GLOBAL."`
	FormProperties map[string]string `json:"formProperties,omitempty" jsonschema:"The exact form field values that will be / were submitted. Review with the user on preview."`

	// status=success
	WorkflowInstanceID string `json:"workflowInstanceId,omitempty" jsonschema:"The started workflow instance's id — share this with the user so they can track it."`

	Guidance string `json:"guidance,omitempty" jsonschema:"On needs_input/validation_error/error, what to do next."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "start_collibra_workflow",
		Title: "Start Collibra Workflow",
		Description: "Discovers, inspects, and starts a Collibra workflow on the user's behalf — a 'workflow' here is a " +
			"multi-step business process (built-in, like requesting dataset access or proposing a new business term, OR a " +
			"customer-built process created in Collibra's Workflow Designer) that Collibra runs and routes to the right " +
			"approvers. It is NOT a data pipeline or ETL job. Any workflow on the instance can be started this way, " +
			"identified by its workflowDefinitionId — there is no restriction to built-in workflows.\n\n" +
			"DISCOVERY: call with no workflowDefinitionId to see workflows the current user is authorized to start. Pass " +
			"businessItemId (an asset's UUID — resolve it first via search_asset_keyword / get_asset_details) to scope this " +
			"to workflows relevant to that asset; omit it to see workflows that concern no specific resource. Returns " +
			"status=options. Authorization filtering is best-effort — an option with a non-empty authorizationNote is still " +
			"worth offering; Collibra makes the final call when the workflow actually starts.\n\n" +
			"START-FORM: call again with a workflowDefinitionId from the options. If the workflow requires form fields and " +
			"formProperties is missing any required one, returns status=needs_input with formFields describing exactly " +
			"what to ask the user for (glossed field names, types, and — when the field is a fixed choice — the allowed " +
			"options; never invent a value outside options).\n\n" +
			"PREVIEW: once workflowDefinitionId, businessItemId (if the workflow's scope requires one), and all required " +
			"formProperties are supplied, returns status=preview — the exact workflow and form values that would be " +
			"submitted. This WRITES to Collibra once started, so it defaults to a preview: confirm=false never starts " +
			"anything; review it with the user, then call again with confirm=true. Everything up to confirm=true is " +
			"read-only.\n\n" +
			"SUCCESS: returns workflowInstanceId — tell the user they can ask to check on it later (there is currently no " +
			"companion tool to look up a running instance's status).\n\n" +
			"Example user requests: \"Request access to the Customer Revenue dataset\"; \"I want to propose a new business " +
			"term\"; \"What workflows can I start for this asset?\"; \"Start the approval process for this data asset\"; " +
			"\"What workflows can I start?\".",
		Handler:     handler(collibraClient),
		Permissions: []string{}, // resource permissions apply — enforced by the workflow definition's own startRoles/WORKFLOW_START permission, same convention as create_asset/edit_asset
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: chip.Ptr(false),
			IdempotentHint:  false,
			OpenWorldHint:   chip.Ptr(false),
		},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		workflowID := strings.TrimSpace(input.WorkflowDefinitionID)
		businessItemID := strings.TrimSpace(input.BusinessItemID)

		if err := validation.UUIDOptional("businessItemId", businessItemID); err != nil {
			return Output{Status: StatusValidationError, Message: err.Error(), Guidance: "Resolve the asset first via search_asset_keyword / get_asset_details and pass its UUID as businessItemId."}, nil
		}

		if workflowID == "" {
			return discoverOptions(ctx, collibraClient, businessItemID)
		}
		if err := validation.UUID("workflowDefinitionId", workflowID); err != nil {
			return Output{Status: StatusValidationError, Message: err.Error(), Guidance: "Call this tool with no workflowDefinitionId to discover valid options, or supply a real workflow UUID."}, nil
		}

		def, code, err := clients.GetWorkflowDefinition(ctx, collibraClient, workflowID)
		if err != nil {
			if code == http.StatusNotFound {
				return Output{
					Status:   StatusValidationError,
					Message:  fmt.Sprintf("No workflow found with id %s on this instance.", workflowID),
					Guidance: "Call this tool with no workflowDefinitionId to see which workflows are actually available here.",
				}, nil
			}
			return Output{Status: StatusError, Message: fmt.Sprintf("Failed to look up workflow %s: %v", workflowID, err), Guidance: "Retry; if it persists, contact your Collibra administrator."}, nil
		}
		if !def.Enabled {
			return Output{
				Status:   StatusValidationError,
				Name:     def.Name,
				Message:  fmt.Sprintf("Workflow %q is disabled on this instance.", def.Name),
				Guidance: "Ask a Collibra administrator to enable it, or call this tool with no workflowDefinitionId to see other options.",
			}, nil
		}

		// Authorization: only the reliable, non-inherited signal (WORKFLOW_START) hard-blocks.
		// Anything uncertain proceeds — Collibra's own 403 at start time is authoritative.
		authorized, uncertain, authErr := clients.IsAuthorizedToStart(ctx, collibraClient, def, businessItemID)
		if authErr == nil && !authorized && !uncertain {
			return Output{
				Status:   StatusValidationError,
				Name:     def.Name,
				Message:  fmt.Sprintf("You do not have permission to start workflow %q (missing the %s permission).", def.Name, clients.PermWorkflowStart),
				Guidance: "Ask a Collibra administrator to grant you a role with the WORKFLOW_START permission, then retry.",
			}, nil
		}

		needsBusinessItem := def.BusinessItemResourceType != "" && def.BusinessItemResourceType != "GLOBAL"
		if needsBusinessItem && businessItemID == "" {
			return Output{
				Status:      StatusNeedsInput,
				Name:        def.Name,
				Description: def.Description,
				Scope:       def.BusinessItemResourceType,
				Message:     fmt.Sprintf("Workflow %q is scoped to a specific %s.", def.Name, strings.ToLower(def.BusinessItemResourceType)),
				Guidance:    "Resolve it (e.g. search_asset_keyword / get_asset_details for an asset) and re-call with its UUID as businessItemId.",
			}, nil
		}

		var formFields []FormField
		var missing []string
		if def.FormRequired {
			form, formErr := clients.GetWorkflowStartFormData(ctx, collibraClient, def.ID)
			if formErr != nil {
				return Output{Status: StatusError, Name: def.Name, Message: fmt.Sprintf("Failed to fetch the start form for %q: %v", def.Name, formErr), Guidance: "Retry; if it persists, contact your Collibra administrator."}, nil
			}
			formFields = toFormFields(form.FormProperties)
			for _, f := range formFields {
				if f.Required {
					if v, ok := input.FormProperties[f.ID]; !ok || strings.TrimSpace(v) == "" {
						missing = append(missing, f.ID)
					}
				}
			}
		}
		if len(missing) > 0 {
			return Output{
				Status:        StatusNeedsInput,
				Name:          def.Name,
				Description:   def.Description,
				Scope:         def.BusinessItemResourceType,
				FormFields:    formFields,
				MissingFields: missing,
				Message:       fmt.Sprintf("Workflow %q needs %d more field(s).", def.Name, len(missing)),
				Guidance:      "Ask the user for the fields listed in missingFields (see formFields for their labels, types and allowed options), then re-call with formProperties populated.",
			}, nil
		}

		if !input.Confirm {
			return Output{
				Status:         StatusPreview,
				Name:           def.Name,
				Description:    def.Description,
				Scope:          def.BusinessItemResourceType,
				FormProperties: input.FormProperties,
				Message:        fmt.Sprintf("Preview only — nothing started. Will start workflow %q%s. Review with the user, then call again with confirm=true.", def.Name, previewBusinessItemClause(needsBusinessItem, businessItemID)),
			}, nil
		}

		startReq := clients.StartWorkflowInstanceRequest{
			WorkflowDefinitionID: def.ID,
			BusinessItemType:     def.BusinessItemResourceType,
			FormProperties:       input.FormProperties,
		}
		if needsBusinessItem {
			startReq.BusinessItemIDs = []string{businessItemID}
		}

		instance, code, startErr := clients.StartWorkflowInstance(ctx, collibraClient, startReq)
		if startErr != nil {
			return startError(code, startErr, def.Name)
		}

		return Output{
			Status:             StatusSuccess,
			Name:               def.Name,
			Description:        def.Description,
			Scope:              def.BusinessItemResourceType,
			WorkflowInstanceID: instance.ID,
			Message:            fmt.Sprintf("Started workflow %q (instance %s). Let the user know they can ask about its status later.", def.Name, instance.ID),
		}, nil
	}
}

// discoverOptions lists workflows available to start: instance-wide when businessItemID is empty,
// or scoped to that asset otherwise. Each candidate is run through IsAuthorizedToStart — one
// definitively unauthorized (missing WORKFLOW_START) is dropped; everything else is kept, with an
// authorizationNote when the rest of the check couldn't be confirmed.
func discoverOptions(ctx context.Context, collibraClient *http.Client, businessItemID string) (Output, error) {
	var defs []clients.WorkflowDefinition
	var err error
	if businessItemID == "" {
		defs, err = clients.ListWorkflowDefinitions(ctx, collibraClient)
	} else {
		defs, err = clients.ListWorkflowDefinitionsForAsset(ctx, collibraClient, businessItemID)
	}
	if err != nil {
		return Output{Status: StatusError, Message: fmt.Sprintf("Failed to look up workflows: %v", err), Guidance: "Retry; if scoped to an asset, verify the UUID."}, nil
	}

	var options []WorkflowOption
	for _, def := range defs {
		if !def.Enabled {
			continue
		}
		authorized, uncertain, authErr := clients.IsAuthorizedToStart(ctx, collibraClient, &def, businessItemID)
		if authErr == nil && !authorized && !uncertain {
			continue // definitively unauthorized (missing WORKFLOW_START) — the one reliable signal
		}
		opt := toOption(def)
		if authErr != nil {
			opt.AuthorizationNote = "Could not verify your permission to start this — offered anyway; Collibra will enforce it when you try."
		} else if uncertain {
			opt.AuthorizationNote = "Authorization could not be fully confirmed (e.g. inherited access isn't checked) — offered anyway; Collibra will enforce it when you try."
		}
		options = append(options, opt)
		if len(options) >= maxOptions {
			break
		}
	}

	msg := fmt.Sprintf("%d workflow(s) available. Pick one and re-call with its workflowDefinitionId.", len(options))
	if len(options) == 0 {
		msg = "No workflows available."
		if businessItemID == "" {
			msg += " Pass businessItemId to also see workflows specific to an asset."
		}
	}
	return Output{Status: StatusOptions, Options: options, Message: msg}, nil
}

func toOption(def clients.WorkflowDefinition) WorkflowOption {
	return WorkflowOption{WorkflowDefinitionID: def.ID, Name: def.Name, Description: def.Description, Scope: def.BusinessItemResourceType, FormRequired: def.FormRequired}
}

func toFormFields(props []clients.WorkflowFormProperty) []FormField {
	out := make([]FormField, 0, len(props))
	for _, p := range props {
		var opts []string
		for _, o := range p.Options {
			opts = append(opts, o.Text)
		}
		out = append(out, FormField{ID: p.ID, Name: p.Name, Type: p.Type, Required: p.Required, Options: opts})
	}
	return out
}

func previewBusinessItemClause(needsBusinessItem bool, businessItemID string) string {
	if needsBusinessItem {
		return fmt.Sprintf(" for %s", businessItemID)
	}
	return ""
}

func startError(code int, err error, workflowName string) (Output, error) {
	switch code {
	case http.StatusForbidden:
		return Output{Status: StatusError, Message: fmt.Sprintf("You do not have permission to start workflow %q (HTTP 403).", workflowName), Guidance: "Ask a Collibra administrator to grant you the role required to start this workflow, then retry."}, nil
	case http.StatusNotFound:
		return Output{Status: StatusError, Message: fmt.Sprintf("Workflow %q could not be found when starting it (HTTP 404).", workflowName), Guidance: "It may have been disabled or removed between the preview and this call — call this tool with no workflowDefinitionId to see current options."}, nil
	case http.StatusBadRequest:
		return Output{Status: StatusError, Message: fmt.Sprintf("Collibra rejected the request to start %q (HTTP 400): %v", workflowName, err), Guidance: "A form field value is likely invalid, or the businessItemId may be the wrong resource type for it — check it against formFields' options and retry."}, nil
	case 0:
		return Output{Status: StatusError, Message: fmt.Sprintf("Failed to start %q: %v", workflowName, err), Guidance: "A network/transport error occurred contacting Collibra. Retry."}, nil
	default:
		return Output{Status: StatusError, Message: fmt.Sprintf("Failed to start %q (HTTP %d): %v", workflowName, code, err), Guidance: "This is likely a server-side error. Retry shortly; if it persists, contact your Collibra administrator."}, nil
	}
}
