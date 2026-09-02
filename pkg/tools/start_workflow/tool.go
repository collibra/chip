// Package start_workflow implements the start_workflow MCP tool: inspects a workflow definition's
// start-form requirements and starts a new instance of it, behind a confirm checkpoint.
//
// FORMS: a workflow definition can need EITHER of two start-form models (see
// clients.WorkflowDefinition.StartFormJSONModelAvailable) — this tool fetches and submits through
// whichever one the definition actually uses; the caller never needs to know which.
//
// This tool WRITES to Collibra once started. Everything up to confirm=true is read-only.
package start_workflow

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/validation"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Status is the overall outcome of a start_workflow call.
type Status string

const (
	// StatusNeedsInput means either businessItemId is required and missing, or one or more
	// required start-form fields are missing or invalid — see MissingFields / Message.
	StatusNeedsInput Status = "needs_input"
	// StatusPreview means confirm was not set: nothing was started.
	StatusPreview Status = "preview"
	// StatusSuccess means the workflow instance was started.
	StatusSuccess Status = "success"
	// StatusValidationError means the input itself is invalid (bad UUID, unknown/disabled
	// workflow) — caught before any write.
	StatusValidationError Status = "validation_error"
	// StatusError means a downstream Collibra error occurred (permission, transport, or the
	// start request being rejected).
	StatusError Status = "error"
)

// Input — call with a workflowDefinitionId (from a prior list_workflow_definitions call) to
// inspect or start it.
type Input struct {
	WorkflowDefinitionID string `json:"workflowDefinitionId" jsonschema:"Required. UUID of the workflow to start — from a prior list_workflow_definitions call, or already known."`
	BusinessItemID       string `json:"businessItemId,omitempty" jsonschema:"UUID of the asset (or, for a domain-/community-scoped workflow, the domain/community) this workflow concerns — resolve it first via search_asset_keyword / get_asset_details. Required when the resolved workflow's scope is not GLOBAL (a needs_input response says so); omit for GLOBAL workflows."`

	FormProperties map[string]string `json:"formProperties,omitempty" jsonschema:"Values for the workflow's start-form fields, keyed by field id (see a prior needs_input response's formFields[].id) — e.g. {\"reason\": \"Need it for Q3 reporting\"}. Omit or partially supply to have the tool report which required fields are still missing or invalid. Not needed for workflows with no start form."`

	Confirm bool `json:"confirm,omitempty" jsonschema:"Safety checkpoint. false (default) returns a PREVIEW of exactly what will be started WITHOUT starting anything — review it with the user. true starts the workflow."`
}

// FormFieldOption is one allowed value for a FormField whose Options is set.
type FormFieldOption struct {
	Key   string `json:"key" jsonschema:"Value to submit in formProperties for this option — NOT the label."`
	Label string `json:"label" jsonschema:"Human-readable label to show the user for this option."`
}

// FormField is one field of a workflow's start form, glossed for an agent with no Collibra
// context. Shared shape regardless of which underlying form model the workflow actually uses.
type FormField struct {
	ID       string            `json:"id" jsonschema:"Field key to use in formProperties."`
	Name     string            `json:"name" jsonschema:"Human-readable field label to show/ask the user."`
	Type     string            `json:"type,omitempty" jsonschema:"The field's underlying type, for reference — free text unless options is set or resourcePicker is true."`
	Required bool              `json:"required" jsonschema:"Whether this field must be supplied before the workflow can start."`
	Options  []FormFieldOption `json:"options,omitempty" jsonschema:"The values this field accepts. Pass a key, never a label, and never invent one that is not listed. Whether the list is closed depends on optionsExhaustive."`
	// OptionsExhaustive distinguishes a closed choice list from a server-supplied shortlist.
	OptionsExhaustive bool   `json:"optionsExhaustive,omitempty" jsonschema:"True when options is the complete set of legal values, so anything else is rejected. False (with options present) means the list is a shortlist Collibra offered and another valid id would also be accepted."`
	MultiValue        bool   `json:"multiValue,omitempty" jsonschema:"True when this field takes SEVERAL values at once, given as one comma-separated string (e.g. 'a,b'). A field without this flag rejects a comma-separated list."`
	DefaultValue      string `json:"defaultValue,omitempty" jsonschema:"What the form pre-fills for this field. Leaving the field out of formProperties submits this value, matching what the product does; pass an explicit value only to override it."`
	HelpText          string `json:"helpText,omitempty" jsonschema:"The hint Collibra's own UI shows beside this field — worth relaying to the user when asking them for a value."`
	VisibleWhen       string `json:"visibleWhen,omitempty" jsonschema:"Present when the form only shows this field under a condition, quoted verbatim. The condition cannot be evaluated here, so the field is still reported with its declared required flag: supply a value if the condition plausibly holds, and say so to the user rather than assuming the field does not apply."`
	ReadOnly          bool   `json:"readOnly,omitempty" jsonschema:"True when the form disables this field. Do not supply a value — the server rejects a change to one."`
	Unsupported       string `json:"unsupported,omitempty" jsonschema:"Present when this tool cannot help produce a value for the field, with the reason. Relay it to the user rather than guessing or retrying; if the field is also required, the workflow cannot be started from here at all."`
	// Detected in both form models: by field type in the legacy one, by the collibra- palette
	// stencil in the JSON one — see clients.WorkflowFormFieldIsResourcePicker.
	ResourcePicker bool `json:"resourcePicker,omitempty" jsonschema:"When true, this field needs a real Collibra resource (e.g. a user, group, role, or another asset) as its value, not a plain value or one of a fixed list. Resolve it first (e.g. via search_asset_keyword) if possible — this tool cannot guess a valid id for it."`
}

// Output is the typed response.
type Output struct {
	Status  Status `json:"status" jsonschema:"needs_input | preview | success | validation_error | error."`
	Message string `json:"message" jsonschema:"Human-readable outcome and what to do next."`

	// Resolved workflow, surfaced from needs_input onward.
	WorkflowDefinitionID string `json:"workflowDefinitionId,omitempty" jsonschema:"The resolved workflow this call acted on — echoed back so a preview can be checked against the workflow the user actually meant."`
	Name                 string `json:"name,omitempty" jsonschema:"Resolved workflow name."`
	Description          string `json:"description,omitempty" jsonschema:"Human-authored explanation of what this workflow actually does — use this to explain to the user what they're about to start, not just its name."`
	Scope                string `json:"scope,omitempty" jsonschema:"Resolved workflow scope: ASSET, DOMAIN or COMMUNITY (started against that resource), or GLOBAL (started against none)."`

	// status=needs_input (form).
	FormFields    []FormField `json:"formFields,omitempty" jsonschema:"The workflow's start-form fields. Ask the user for the ones named in missingFields (see the message for WHY each is missing/invalid), then re-call with formProperties populated."`
	MissingFields []string    `json:"missingFields,omitempty" jsonschema:"Required field ids not yet supplied (or supplied blank) in formProperties. Does not include fields with an invalid value — see the message for those."`

	// status=preview / success. Every field the start request will carry appears here, so the
	// preview a user approves is the whole payload and not a summary of it.
	BusinessItemID string            `json:"businessItemId,omitempty" jsonschema:"The resource this workflow will be / was started against. Absent for a GLOBAL workflow, which is started against no resource — if you passed one and it is missing here, it was not used."`
	FormProperties map[string]string `json:"formProperties,omitempty" jsonschema:"The exact form field values that will be / were submitted, after trimming. Review these with the user on preview — they are what gets sent, character for character."`

	// status=success.
	WorkflowInstanceID string `json:"workflowInstanceId,omitempty" jsonschema:"The started workflow instance's id — share this with the user so they can track it. There is currently no companion tool to look up a running instance's status."`

	Guidance string `json:"guidance,omitempty" jsonschema:"On needs_input/validation_error/error, what to do next."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "start_workflow",
		Title: "Start Workflow",
		Description: "Inspects a workflow definition's start requirements and starts a new instance of it — a 'workflow' " +
			"here is a governed business process (built-in, like requesting dataset access or proposing a new business " +
			"term, OR a customer-built process created in Collibra's Workflow Designer) that Collibra runs and routes to " +
			"the right approvers. NOT a data pipeline or ETL job. Use list_workflow_definitions first to find the " +
			"workflowDefinitionId — this tool does not search.\n\n" +
			"Call first with just workflowDefinitionId. A needs_input response means something is still missing — " +
			"businessItemId or start-form values (see formFields/missingFields) — resolve what it asks for and re-call; " +
			"this can take more than one round. Nothing is written until status=preview, which appears only once every " +
			"requirement is satisfied. confirm=false (default) returns that preview without starting anything; " +
			"confirm=true starts it.\n\n" +
			"Even a clean preview does not guarantee the start will succeed: Collibra enforces who may actually start a " +
			"given workflow at start time, and runs the workflow's own logic synchronously up to its first wait state, " +
			"so a failure partway through creates NO instance at all — never assume a partial start.\n\n" +
			"Example user requests: \"Request access to the Customer Revenue dataset\"; \"I want to propose a new " +
			"business term\"; \"Log a potential security breach\".",
		Handler: handler(collibraClient),
		// No extra dgc.* scope, same as create_asset/edit_asset: whether a user may start a given
		// workflow is a per-workflow authorization decision Collibra makes in its service layer
		// (WORKFLOW_START plus the definition's own startRoles, walked with role inheritance), not
		// something granted by a scope. The wf.administration scope is an OAuth2
		// client-credentials scope mapping to the workflow-ADMIN bypass, not the ordinary start
		// requirement. See pkg/clients/workflow_client.go's package comment.
		Permissions: []string{},
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

		if err := validation.UUID("workflowDefinitionId", workflowID); err != nil {
			return Output{Status: StatusValidationError, Message: err.Error(), Guidance: "Call list_workflow_definitions to find a real workflowDefinitionId, or supply one you already have."}, nil
		}
		if err := validation.UUIDOptional("businessItemId", businessItemID); err != nil {
			return Output{Status: StatusValidationError, Message: err.Error(), Guidance: "Resolve the resource first via search_asset_keyword / get_asset_details and pass its UUID as businessItemId."}, nil
		}

		def, code, err := clients.GetWorkflowDefinition(ctx, collibraClient, workflowID)
		if err != nil {
			return lookupError(code, err, workflowID), nil
		}
		if !def.Enabled {
			return Output{
				Status:               StatusValidationError,
				WorkflowDefinitionID: def.ID,
				Name:                 def.Name,
				Message:              fmt.Sprintf("Workflow %q is disabled on this instance.", def.Name),
				Guidance:             "Ask a Collibra administrator to enable it, or call list_workflow_definitions to see other options.",
			}, nil
		}

		needsBusinessItem := def.BusinessItemResourceType != "" && def.BusinessItemResourceType != "GLOBAL"
		if needsBusinessItem && businessItemID == "" {
			return Output{
				Status:               StatusNeedsInput,
				WorkflowDefinitionID: def.ID,
				Name:                 def.Name,
				Description:          def.Description,
				Scope:                def.BusinessItemResourceType,
				Message:              fmt.Sprintf("Workflow %q is scoped to a specific %s.", def.Name, strings.ToLower(def.BusinessItemResourceType)),
				Guidance:             "Resolve it (e.g. search_asset_keyword / get_asset_details for an asset) and re-call with its UUID as businessItemId.",
			}, nil
		}

		formFields, formErr := fetchFormFields(ctx, collibraClient, def)
		if formErr != nil {
			return Output{Status: StatusError, WorkflowDefinitionID: def.ID, Name: def.Name, Message: fmt.Sprintf("Failed to fetch the start form for %q: %v", def.Name, formErr), Guidance: "Retry; if it persists, contact your Collibra administrator."}, nil
		}

		// Normalized once here, then used for validation, the preview and the write alike — see
		// normalizeFormProperties for why that single source matters.
		formProperties := normalizeFormProperties(input.FormProperties)

		missing, problems := validateFormProperties(formFields, formProperties)
		if len(problems) > 0 {
			return Output{
				Status:               StatusNeedsInput,
				WorkflowDefinitionID: def.ID,
				Name:                 def.Name,
				Description:          def.Description,
				Scope:                def.BusinessItemResourceType,
				FormFields:           toFormFields(formFields),
				MissingFields:        missing,
				Message:              fmt.Sprintf("Workflow %q needs %d form field issue(s) resolved: %s", def.Name, len(problems), strings.Join(problems, "; ")),
				Guidance:             "Ask the user for the fields listed in missingFields (see formFields for their labels, types and allowed options), fix any invalid values, then re-call with formProperties populated.",
			}, nil
		}

		// Exactly what the start request will carry — see effectiveFormProperties. Computed before
		// the preview so the preview and the write cannot disagree.
		effective := effectiveFormProperties(def, formFields, formProperties)

		// Only echo (and only send) the business item when it is actually part of the request.
		sentBusinessItemID := ""
		if needsBusinessItem {
			sentBusinessItemID = businessItemID
		}

		if !input.Confirm {
			return Output{
				Status:               StatusPreview,
				WorkflowDefinitionID: def.ID,
				Name:                 def.Name,
				Description:          def.Description,
				Scope:                def.BusinessItemResourceType,
				BusinessItemID:       sentBusinessItemID,
				// The form is echoed on preview too, not just on needs_input. A workflow whose
				// fields are all OPTIONAL reaches this point with nothing supplied and nothing
				// missing, so without this the caller is shown an empty preview and never learns
				// the form exists — then confirms and starts a process with every value unset.
				FormFields:     toFormFields(formFields),
				FormProperties: effective,
				Message:        fmt.Sprintf("Preview only — nothing started. Will start workflow %q%s. Review with the user, then call again with confirm=true.", def.Name, previewBusinessItemClause(needsBusinessItem, businessItemID)),
			}, nil
		}

		instance, code, startErr := startInstance(ctx, collibraClient, def, formFields, needsBusinessItem, businessItemID, effective)
		if startErr != nil {
			return startError(code, startErr, def.Name), nil
		}

		return Output{
			Status:               StatusSuccess,
			WorkflowDefinitionID: def.ID,
			Name:                 def.Name,
			Description:          def.Description,
			Scope:                def.BusinessItemResourceType,
			BusinessItemID:       sentBusinessItemID,
			FormProperties:       effective,
			WorkflowInstanceID:   instance.ID,
			Message:              fmt.Sprintf("Started workflow %q (instance %s). Let the user know they can ask about its status later.", def.Name, instance.ID),
		}, nil
	}
}

// normalizeFormProperties trims every supplied value once, so validation, the preview and the
// write all see exactly the same bytes. Previously only validateFormProperties trimmed, while the
// preview and the start request carried the raw input: a value like "high " passed the
// allowed-options check against the trimmed "high" and was then rejected by Collibra as an unknown
// enum key — the failure landing AFTER the user had approved the preview, which is precisely what
// the confirm checkpoint exists to prevent.
//
// Keys are deliberately left alone: a key with stray whitespace simply will not match a field id,
// so the field reads as missing and the caller is told so — a safe, visible failure.
func normalizeFormProperties(supplied map[string]string) map[string]string {
	if len(supplied) == 0 {
		return nil
	}
	normalized := make(map[string]string, len(supplied))
	for k, v := range supplied {
		normalized[k] = strings.TrimSpace(v)
	}
	return normalized
}

// lookupError maps a failure to resolve the workflow definition. 403 matters as much as 404 here:
// telling a caller to "retry" a permission failure sends it into a loop it can never win
// (TOOL_CONTRIBUTION_STANDARDS.md §6.3, §6.6).
func lookupError(code int, err error, workflowID string) Output {
	switch code {
	case http.StatusNotFound:
		return Output{
			Status:   StatusValidationError,
			Message:  fmt.Sprintf("No workflow found with id %s on this instance.", workflowID),
			Guidance: "Call list_workflow_definitions to see which workflows actually exist here.",
		}
	case http.StatusForbidden:
		return Output{
			Status:   StatusError,
			Message:  fmt.Sprintf("You do not have permission to read workflow %s (HTTP 403).", workflowID),
			Guidance: "Do not retry — ask a Collibra administrator for access to this workflow, or call list_workflow_definitions to see the ones you can already use.",
		}
	case 0:
		return Output{
			Status:   StatusError,
			Message:  fmt.Sprintf("Failed to reach Collibra while looking up workflow %s: %v", workflowID, err),
			Guidance: "A network/transport error occurred. Retry.",
		}
	default:
		return Output{
			Status:   StatusError,
			Message:  fmt.Sprintf("Failed to look up workflow %s (HTTP %d): %v", workflowID, code, err),
			Guidance: "Retry; if it persists, contact your Collibra administrator.",
		}
	}
}

// fetchFormFields returns nil (no error) for a workflow with no start form, and otherwise
// dispatches to whichever form model this definition actually uses — see the package comment.
func fetchFormFields(ctx context.Context, collibraClient *http.Client, def *clients.WorkflowDefinition) ([]clients.WorkflowFormField, error) {
	if !def.FormRequired {
		return nil, nil
	}
	if def.StartFormJSONModelAvailable {
		return clients.GetWorkflowStartFormJSONModel(ctx, collibraClient, def.ID)
	}
	return clients.GetWorkflowStartFormData(ctx, collibraClient, def.ID)
}

// validateFormProperties checks supplied form values against fields' requiredness and (for
// choice fields) their allowed options, before any write (TOOL_CONTRIBUTION_STANDARDS.md §6.1/
// §6.2 — "never invent a value outside options" is a promise this tool's own description makes,
// so it is enforced here, not left to a downstream 400). missing lists field ids not supplied at
// all (or supplied blank) — the exact re-call target; problems is the fuller human-readable list
// (missing AND invalid-value fields) surfaced in the response message.
//
// supplied MUST already have been through normalizeFormProperties. It deliberately does not trim
// again: the bug this replaced came from exactly that — one place trimming, another not, so the
// value checked here was not the value sent.
func validateFormProperties(fields []clients.WorkflowFormField, supplied map[string]string) (missing []string, problems []string) {
	for _, f := range fields {
		value, ok := supplied[f.ID]
		if f.Required && (!ok || value == "") {
			missing = append(missing, f.ID)
			problems = append(problems, describeMissingField(f))
			continue
		}
		if ok && value != "" {
			problems = append(problems, checkValue(f, value)...)
		}
	}
	return missing, problems
}

func describeMissingField(f clients.WorkflowFormField) string {
	switch {
	case f.Unsupported != "":
		// Never point at a lookup that cannot succeed — say what is actually true instead.
		return fmt.Sprintf("%q is required but cannot be filled in from here: %s", f.ID, f.Unsupported)
	case f.VisibleWhen != "":
		return fmt.Sprintf("%q is required, but the form only shows it when %s — supply a value if that holds, and tell the user you did; the server does not enforce requiredness for a hidden field, so leaving it out just starts the workflow with it unset", f.ID, f.VisibleWhen)
	case len(f.Options) > 0:
		return fmt.Sprintf("%q is required — pick one of the option keys listed for it in formFields", f.ID)
	case clients.WorkflowFormFieldIsResourcePicker(f):
		return fmt.Sprintf("%q needs a real Collibra resource as its value — resolve it first (e.g. via search_asset_keyword); this tool cannot guess a valid id for it", f.ID)
	}
	return fmt.Sprintf("%q is required", f.ID)
}

// checkValue reports what is wrong with a supplied value, if anything.
//
// Two rules that used to be missing. A multi-value field takes several option keys in ONE
// comma-separated string, so each part is checked separately — previously "a,b" was compared
// whole against the option list, failed, and was re-reported forever with no hint that a list was
// even legal. And an option list is only enforced when it is closed: for a resource picker the
// server sends a SHORTLIST, and rejecting anything outside it would refuse ids that are perfectly
// valid.
func checkValue(f clients.WorkflowFormField, value string) []string {
	if len(f.Options) == 0 || !f.OptionsExhaustive {
		return nil
	}
	parts := []string{value}
	if f.MultiValue {
		parts = strings.Split(value, ",")
	}
	var problems []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || hasOptionKey(f.Options, part) {
			continue
		}
		if f.MultiValue {
			problems = append(problems, fmt.Sprintf("%q lists %q, which is not one of its allowed option keys — this field takes a comma-separated list of those keys", f.ID, part))
			continue
		}
		problems = append(problems, fmt.Sprintf("%q's value %q is not one of this field's allowed options — pass one of the option keys shown in formFields, not a label", f.ID, part))
	}
	return problems
}

func hasOptionKey(options []clients.WorkflowFormFieldOption, key string) bool {
	for _, o := range options {
		if o.Key == key {
			return true
		}
	}
	return false
}

func toFormFields(fields []clients.WorkflowFormField) []FormField {
	out := make([]FormField, 0, len(fields))
	for _, f := range fields {
		tf := FormField{
			ID: f.ID, Name: f.Name, Type: f.Type, Required: f.Required,
			ResourcePicker:    clients.WorkflowFormFieldIsResourcePicker(f),
			OptionsExhaustive: f.OptionsExhaustive,
			MultiValue:        f.MultiValue,
			DefaultValue:      f.DefaultValue,
			VisibleWhen:       f.VisibleWhen,
			ReadOnly:          f.ReadOnly,
			HelpText:          f.HelpText,
			Unsupported:       f.Unsupported,
		}
		if len(f.Options) > 0 {
			tf.Options = make([]FormFieldOption, len(f.Options))
			for i, o := range f.Options {
				tf.Options[i] = FormFieldOption{Key: o.Key, Label: o.Label}
			}
		}
		out = append(out, tf)
	}
	return out
}

// startInstance dispatches to whichever start endpoint matches this definition's form model —
// see the package comment. Only the JSON-model path needs Map<String,Object> form values; the
// legacy/no-form path uses the public API's Map<String,String>.
func startInstance(ctx context.Context, collibraClient *http.Client, def *clients.WorkflowDefinition, formFields []clients.WorkflowFormField, needsBusinessItem bool, businessItemID string, formProperties map[string]string) (*clients.WorkflowInstance, int, error) {
	if def.StartFormJSONModelAvailable {
		req := clients.StartWorkflowInstanceWithFormRequest{
			WorkflowDefinitionID: def.ID,
			BusinessItemType:     def.BusinessItemResourceType,
			FormProperties:       toTypedMap(formFields, formProperties),
		}
		if needsBusinessItem {
			req.BusinessItemIDs = []string{businessItemID}
		}
		return clients.StartWorkflowInstanceWithForm(ctx, collibraClient, req)
	}
	req := clients.StartWorkflowInstanceRequest{
		WorkflowDefinitionID: def.ID,
		BusinessItemType:     def.BusinessItemResourceType,
		FormProperties:       formProperties,
	}
	if needsBusinessItem {
		req.BusinessItemIDs = []string{businessItemID}
	}
	return clients.StartWorkflowInstance(ctx, collibraClient, req)
}

// effectiveFormProperties is exactly what the start request will carry — the caller's values laid
// over the defaults the form itself declares. Computed once and used for BOTH the preview and the
// write, so the payload a user approves is the payload that gets sent (§5.2). Deriving them
// separately is how the preview silently drifted from the request once before.
//
// Filling the defaults in is not cosmetic. A JSON-model start form's script task reads its inputs
// as plain Groovy identifiers, and an unbound identifier raises MissingPropertyException — which,
// since the script runs synchronously as part of the start, kills the whole start with an opaque
// HTTP 500 that names no field. Confirmed on OOTB "Issue Creation", whose form marks `priority`
// and `responsibleCommunity` optional while its script dereferences both: send nothing for them
// and the start fails; send the defaults the form already declares and it succeeds. Dropping a
// default would also quietly diverge from the product, which pre-fills it.
//
// What this deliberately does NOT do is invent a value for every other declared field. Nulling
// them would also satisfy the unbound-variable problem, but only at start time, where no variable
// exists yet; the same helper applied to completing a user task would overwrite variables the
// process had already set. Sending only what the form itself declares has no such failure mode.
//
// The legacy model is left alone: there the server applies the BPMN's own `default=` values, so
// adding anything here would be duplicating work the engine already does.
func effectiveFormProperties(def *clients.WorkflowDefinition, fields []clients.WorkflowFormField, supplied map[string]string) map[string]string {
	if !def.StartFormJSONModelAvailable {
		return supplied
	}
	out := make(map[string]string, len(fields)+len(supplied))
	for _, f := range fields {
		if f.DefaultValue != "" {
			out[f.ID] = f.DefaultValue
		}
	}
	for k, v := range supplied {
		out[k] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// toTypedMap widens the string-keyed form values into the object map the form engine expects,
// converting the two types where a string would be actively wrong.
//
// The whole reason this path uses the form-engine endpoint is that it carries typed values;
// handing it the string "false" for a boolean field defeats that, because in Groovy a non-empty
// string is truthy, so a start script's `if (urgent)` takes the branch the user did not choose.
// Numbers have the same problem in arithmetic. Everything else stays a string: the form model has
// many textual types and guessing beyond the two unambiguous cases would be worse than not trying.
// A value that does not parse is passed through untouched so the server can reject it plainly
// rather than this client silently substituting something.
func toTypedMap(fields []clients.WorkflowFormField, m map[string]string) map[string]interface{} {
	if len(m) == 0 {
		return nil
	}
	kind := make(map[string]string, len(fields))
	for _, f := range fields {
		kind[f.ID] = strings.ToLower(f.Type)
	}
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		switch kind[k] {
		case "boolean", "checkbox":
			if b, err := strconv.ParseBool(v); err == nil {
				out[k] = b
				continue
			}
		case "integer", "number", "decimal":
			if n, err := strconv.ParseFloat(v, 64); err == nil {
				out[k] = n
				continue
			}
		}
		out[k] = v
	}
	return out
}

func previewBusinessItemClause(needsBusinessItem bool, businessItemID string) string {
	if needsBusinessItem {
		return fmt.Sprintf(" for %s", businessItemID)
	}
	return ""
}

func startError(code int, err error, workflowName string) Output {
	switch code {
	case http.StatusForbidden:
		return Output{Status: StatusError, Message: fmt.Sprintf("You do not have permission to start workflow %q (HTTP 403).", workflowName), Guidance: "Ask a Collibra administrator to grant you the role required to start this workflow, then retry."}
	case http.StatusNotFound:
		return Output{Status: StatusError, Message: fmt.Sprintf("Workflow %q could not be found when starting it (HTTP 404).", workflowName), Guidance: "It may have been disabled or removed between the preview and this call — call list_workflow_definitions to see current options."}
	case http.StatusBadRequest:
		return Output{Status: StatusError, Message: fmt.Sprintf("Collibra rejected the request to start %q (HTTP 400): %v", workflowName, err), Guidance: "A form field value is likely invalid, or businessItemId may be the wrong resource type for it — check it against formFields' options and retry. Nothing was created."}
	case http.StatusUnprocessableEntity:
		// Distinct from 400: the request parsed fine, but Collibra refused it on the workflow's own
		// rules (an exclusivity constraint, an assignment rule, an already-running instance).
		// Retrying the identical call cannot help, so do not suggest it.
		return Output{Status: StatusError, Message: fmt.Sprintf("Collibra could not process the request to start %q (HTTP 422): %v", workflowName, err), Guidance: "The request was well-formed but rejected by this workflow's own rules — e.g. it is already running for that resource, or the resource does not satisfy its assignment rules. Do not retry unchanged; report the message to the user. Nothing was created."}
	case 0:
		return Output{Status: StatusError, Message: fmt.Sprintf("Failed to start %q: %v", workflowName, err), Guidance: "A network/transport error occurred contacting Collibra. Retry."}
	default:
		return Output{Status: StatusError, Message: fmt.Sprintf("Failed to start %q (HTTP %d): %v", workflowName, code, err), Guidance: "This is likely a server-side error. Collibra runs a workflow's own logic synchronously as part of starting it, so this failure means NO instance was created — it is not a partial start. Retry shortly; if it persists, contact your Collibra administrator."}
	}
}
