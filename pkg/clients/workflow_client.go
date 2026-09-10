package clients

// This file backs the list_workflow_definitions and start_workflow MCP tools (DEV-213248).
// A "workflow" here is a Collibra business process (built-in or built in Workflow Designer) that
// Collibra runs and routes to the right approvers — not a data pipeline.
//
// Two Collibra start-form models exist, and a workflow definition can only ever use one of them
// (see WorkflowDefinition.StartFormJSONModelAvailable):
//
//   - LEGACY (BPMN <formProperty>): GetWorkflowStartFormData, over the public REST API.
//   - JSON (a .form file authored in Workflow Designer, serialized as an ENTERPRISE Flowable
//     com.flowable.form.model.FlowableFormModel — rows[].cols[], NOT org.flowable's flat
//     SimpleFormModel): GetWorkflowStartFormJSONModel + StartWorkflowInstanceWithForm.
//
// The JSON model is NOT reachable through the public REST surface at all:
//
//   - READING it: there is no REST resource for the JSON start-form schema. It is served only as
//     a GraphQL field, api.workflowStartFormJsonModel.
//
//   - STARTING it: POST /rest/2.0/internal/workflow/startWithForm is the only route that binds the
//     deployed .form definition. The two start endpoints are not two encodings of one operation —
//     they drive different form mechanisms in the workflow engine. The public
//     POST /rest/2.0/workflowInstances submits values through the legacy BPMN <formProperty>
//     channel; the internal one submits them to Flowable's form engine, which is what a .form
//     definition is bound by. Starting a JSON-model workflow through the public endpoint does not
//     fail loudly: the process starts with the form never bound, silently discarding everything
//     the user filled in. That is the reason the internal endpoint is unavoidable — NOT the
//     Map<String,String> vs Map<String,Object> difference between the two request bodies, which is
//     a consequence of the two mechanisms and would not justify it on its own (this client's
//     FormProperties are map[string]string either way).
//
// Per TOOL_CONTRIBUTION_STANDARDS.md §8.1 (prefer the public API; say why when only an internal
// one exists): there is no public alternative for this path today. If a public endpoint that binds
// the JSON form model appears, switch to it and delete this note.
//
// AUTHENTICATION / AUTHORIZATION of these endpoints, including the internal one:
//   - Authentication is global and path-based, not per-resource: /rest/** is authenticated
//     wholesale, with a short allowlist of anonymous paths (health, auth sessions, i18n, password
//     reset). /rest/2.0/internal/workflow/startWithForm is not on that allowlist, so it
//     authenticates exactly like the public endpoints.
//   - Authorization is enforced server-side, identically for both start endpoints: each runs the
//     same constraint-and-permission check before doing anything.
//   - The @SecurityRequirement(scopes = {"kg.view-all"} / {"wf.administration"}) annotations in
//     the published API spec describe the OAUTH2 CLIENT-CREDENTIALS flow only. The spec declares
//     three auth schemes (basicAuth, jwtAuth, oauth2) and attaches scopes to oauth2 alone: they
//     are read from a JWT's "scope" claim and mapped onto global permissions
//     (wf.administration -> WORKFLOW_ADMINISTRATION). Under Basic auth there is no such claim, no
//     such mapping happens, and the user's own roles/permissions apply directly. chip
//     authenticates with Basic auth when a username/password is configured, otherwise it forwards
//     the caller's Authorization header (see cmd/chip/http.go), so the scoped path only applies in
//     the forwarded-JWT case.
//     Either way these are NOT the dgc.* scopes chip lists in a tool's Permissions field, and
//     wf.administration is in any case the workflow-admin bypass, not what an ordinary user needs
//     to start a workflow they are entitled to start — so copying it into Permissions would
//     misstate the requirement.
// So neither the internal endpoint nor the JSON-model path is less protected than the public one.
//
// Authorization is intentionally NOT reimplemented client-side. The server runs a real
// authorization check (system-workflow bypass, global-admin bypass, the WORKFLOW_START permission
// gated behind its own feature toggle, an accessibility check, then the definition's start roles
// walked WITH role inheritance) in two different places:
//   - Automatically, whenever FindWorkflowDefinitions is called WITH a business item
//     (asset/domain/community) — see the FindWorkflowDefinitionsFilter doc comment.
//   - Always, when a start is actually attempted (the start endpoints reject an unauthorized start
//     with 403 and errorCode START_WORKFLOW_NO_PERMISSION).
// Client-side filtering would either under- or over-block relative to that model. Collibra's own
// checks are the only authority; this client passes through what the server returns and errors.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// WorkflowDefinition mirrors the subset of the server's workflow-definition resource
// (GET /rest/2.0/workflowDefinitions and GET /rest/2.0/workflowDefinitions/{id})
// this client needs. Deliberately thin: fields the
// server's own authorization model needs (startRoles, registeredUserAccessible, ...) are NOT
// mapped here — see the package comment on why chip does not reimplement that model.
type WorkflowDefinition struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Enabled     bool   `json:"enabled"`
	// StartLabel is the caption the product shows on the button/menu entry that starts this
	// workflow — often NOT the same string as Name (e.g. name "Propose New Business Term" vs label
	// "Propose Business Term"; how often is measured in list_workflow_definitions' filterByName). It
	// is what a user actually sees and will quote, so search matches on it as well as on Name.
	StartLabel string `json:"startLabel,omitempty"`
	// FormRequired is true when starting this workflow requires filling in a start form — of
	// EITHER model. Check StartFormJSONModelAvailable to know which.
	FormRequired bool `json:"formRequired"`
	// StartFormJSONModelAvailable distinguishes the two form models this workflow's start form
	// can use (see the package comment). It is only meaningful when FormRequired is true; the
	// combination FormRequired=false + this=true does not occur (the server sets both together at
	// deploy time). Fetching the LEGACY form endpoint for a workflow
	// where this is true returns an EMPTY field list, not an error — always branch on this flag
	// before deciding which form-fetch path to call.
	StartFormJSONModelAvailable bool `json:"startFormJsonModelAvailable"`
	// BusinessItemResourceType is ASSET | DOMAIN | COMMUNITY | GLOBAL: the kind of resource (if
	// any) this workflow concerns. GLOBAL means no specific resource — starting it needs no
	// business item.
	//
	// NOT a full picture of the server's own scopes, deliberately: internally a workflow may also
	// be USER-scoped, but the public REST v2 enum has no such value and the server maps USER to
	// GLOBAL on the way out, so this field never carries it. The non-deprecated replacement
	// (businessItemDiscriminator, a plain string) does report "USER" — chip does not read it,
	// since USER-scope workflows are driven by user lifecycle events rather than started by hand.
	// This field is deprecated server-side in favour of that string, but is still populated and is
	// still what StartWorkflowInstanceRequest.BusinessItemType expects — used as-is.
	BusinessItemResourceType string `json:"businessItemResourceType,omitempty"`
}

// WorkflowDefinitionsPage is the paged envelope the search endpoint returns.
type WorkflowDefinitionsPage struct {
	Total   int                  `json:"total"`
	Offset  int                  `json:"offset"`
	Limit   int                  `json:"limit"`
	Results []WorkflowDefinition `json:"results"`
}

// FindWorkflowDefinitionsFilter mirrors the query params of the workflow-definitions search
// endpoint. Every field is optional.
//
// IMPORTANT: setting AssetID / DomainID / CommunityID does not just narrow the result set — it
// puts the server on an entirely different code path, one that ALSO applies the
// server's own start-authorization check, exclusivity (is a workflow already running for
// this exact resource), and assignment-rule matching (is this workflow even applicable to this
// resource's type/status) — none of which run on the unscoped path. Both paths drop system
// workflows. The list_workflow_definitions tool description must say this plainly: a scoped call
// returns only what the caller can actually start there; an unscoped call returns everything.
//
// AssetID / DomainID / CommunityID bind to the server's assetId / domainId / communityId query
// params — SINGULAR, despite each being a List<UUID> server-side (JAX-RS binds repeated
// same-name params into a list; this client only ever sends one value per scope, which the
// server accepts as a one-element list). Sending the plural form (assetIds, ...) is a silent
// no-op: the server ignores the unrecognized param name and returns the full unfiltered list —
// this was the root cause of the "asset scoping does not work" finding on an earlier attempt at
// this tool (DEV-213248 analysis, 2026-09-01).
//
// Enabled / Global are *bool, not bool: the server treats them as tri-state (nullable Boolean) —
// omit to not filter, true/false to filter on that exact value. A plain bool with `omitempty`
// could never send `false` (go-querystring treats a nil *bool, and only a nil *bool, as empty —
// confirmed against google/go-querystring v1.2.0's isEmptyValue). Setting AssetID/DomainID/
// CommunityID together with Global=true returns an empty page server-side (mutually exclusive).
type FindWorkflowDefinitionsFilter struct {
	AssetID     string `url:"assetId,omitempty"`
	DomainID    string `url:"domainId,omitempty"`
	CommunityID string `url:"communityId,omitempty"`
	Enabled     *bool  `url:"enabled,omitempty"`
	Global      *bool  `url:"global,omitempty"`
	// Name and Description are partial (substring), case-sensitivity server-defined — the API
	// documents both as "(could be partial)".
	Name        string `url:"name,omitempty"`
	Description string `url:"description,omitempty"`
	Offset      int    `url:"offset,omitempty"`
	Limit       int    `url:"limit,omitempty"`
}

// FindWorkflowDefinitions lists workflow definitions matching filter. See
// FindWorkflowDefinitionsFilter's doc comment for the scoped-vs-unscoped authorization asymmetry.
// Returns the HTTP status code alongside the error, so callers can tell a permission failure or a
// bad resource id from a transport problem — same contract as GetWorkflowDefinition below. The
// code is 0 when the request never reached the server.
func FindWorkflowDefinitions(ctx context.Context, client *http.Client, filter FindWorkflowDefinitionsFilter) (*WorkflowDefinitionsPage, int, error) {
	endpoint, err := buildUrl("/rest/2.0/workflowDefinitions", filter)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to build endpoint: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}
	body, code, err := executeCollibraRequestWithStatus(client, req)
	if err != nil {
		return nil, code, err
	}
	var page WorkflowDefinitionsPage
	if jsonErr := json.Unmarshal(body, &page); jsonErr != nil {
		return nil, code, fmt.Errorf("failed to parse workflow definitions: %w", jsonErr)
	}
	return &page, code, nil
}

// GetWorkflowDefinition fetches a single workflow definition by UUID — works for both OOTB and
// customer-built (Workflow Designer) workflows; the API makes no distinction between them.
// Returns the HTTP status code so callers can distinguish "no such workflow" (404) from anything
// else.
func GetWorkflowDefinition(ctx context.Context, client *http.Client, workflowDefinitionID string) (*WorkflowDefinition, int, error) {
	endpoint := "/rest/2.0/workflowDefinitions/" + url.PathEscape(workflowDefinitionID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}
	body, code, err := executeCollibraRequestWithStatus(client, req)
	if err != nil {
		return nil, code, err
	}
	var def WorkflowDefinition
	if jsonErr := json.Unmarshal(body, &def); jsonErr != nil {
		return nil, code, fmt.Errorf("failed to parse workflow definition: %w", jsonErr)
	}
	return &def, code, nil
}

// globalWorkflowDefinitionsQuery calls the workflowDefinitionsGlobal GraphQL field ("api"
// context, same as GetWorkflowStartFormJSONModel) — a non-deprecated source for global
// (no-resource) workflow definitions, and an authorization-checked one: every candidate it
// returns has passed the same start-permission check the server applies at start time.
//
// Why not the public REST search, which has a `global` query param? Because that param only
// narrows the rows, it does not change how they are selected: the server picks its
// authorization-checked code path purely on whether a business item was supplied, so `global=true`
// with no resource lands in the raw, unchecked query. Measured on a live instance in September
// 2026, of 26 enabled global definitions only 8 were startable by a user without an admin role —
// REST would have offered all 26 and 18 of them would have failed with 403 at start time. The trap
// is that both sources returned an identical 26 for an ADMIN, since admin rights short-circuit the
// check before it runs: the bug is invisible in testing and only appears for ordinary users.
//
// globalCreate is deliberately NOT passed. The server only narrows by that flag when the argument
// is present, so omitting it returns every global-scope definition the user is permitted to start
// — not just the subset an author flagged for the product's global Create menu. That flag records
// INTENT ("offer this in the Create menu"), not capability: workflows triggered by a timer, or
// called by another workflow, are commonly global with the flag unset, yet a permitted user can
// still start them. The tool's contract is "what can I start", so capability is the right
// criterion.
//
// The cost of that choice, measured on a live instance in September 2026: omitting the argument
// returned 26 definitions instead of 23, and the three extra ones — "Escalation Process" (fires
// when a task exceeds its due date), "AssessmentPrePopulationFlow" (event-driven), plus a stray
// test workflow
// — are startable but not meaningful to start by hand. The tool description therefore tells the
// caller that some listed workflows are normally triggered automatically, so it can say so rather
// than blithely offering to start an escalation process.
//
// No `enabled` field in the response: the GraphQL Workflow type does not expose one at
// all — confirmed live (a query including it fails GraphQL
// validation). Harmless to omit: every result is enabled by construction, per the argument above.
const globalWorkflowDefinitionsQuery = `query WorkflowDefinitionsGlobal {
  api {
    workflowDefinitionsGlobal(enabled: true) {
      id
      name
      description
      startLabel
      formRequired
      startFormJsonModelAvailable
    }
  }
}`

type globalWorkflowWire struct {
	ID                          string `json:"id"`
	Name                        string `json:"name"`
	Description                 string `json:"description,omitempty"`
	StartLabel                  string `json:"startLabel,omitempty"`
	FormRequired                bool   `json:"formRequired"`
	StartFormJSONModelAvailable bool   `json:"startFormJsonModelAvailable"`
}

type globalWorkflowDefinitionsResponse struct {
	Data *struct {
		API *struct {
			WorkflowDefinitionsGlobal []globalWorkflowWire `json:"workflowDefinitionsGlobal"`
		} `json:"api"`
	} `json:"data"`
	Errors []Error `json:"errors"`
}

// ListGlobalWorkflowDefinitions lists global-scope workflow definitions Collibra confirms the
// current user is authorized to start — see globalWorkflowDefinitionsQuery's doc comment for how.
// Returns the HTTP status code alongside the error so callers can tell a permission failure from a
// transport problem — same contract as FindWorkflowDefinitions. The code is 0 when the request
// never reached the server.
func ListGlobalWorkflowDefinitions(ctx context.Context, client *http.Client) ([]WorkflowDefinition, int, error) {
	reqBody := Request{Query: globalWorkflowDefinitionsQuery}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to marshal global workflow definitions query: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/graphql", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	body, code, err := executeCollibraRequestWithStatus(client, req)
	if err != nil {
		return nil, code, err
	}
	var resp globalWorkflowDefinitionsResponse
	if jsonErr := json.Unmarshal(body, &resp); jsonErr != nil {
		return nil, code, fmt.Errorf("failed to parse global workflow definitions response: %w", jsonErr)
	}
	if len(resp.Errors) > 0 {
		return nil, code, fmt.Errorf("global workflow definitions query errors: %v", resp.Errors)
	}
	if resp.Data == nil || resp.Data.API == nil {
		// A 200 carrying no data is NOT "there are no global workflows". It means something
		// answered that was not this GraphQL field: a gateway or SSO page returning 200 with an
		// unrelated JSON body, or the field having been renamed or removed. Reporting it as an
		// empty list would tell the user they have nothing they can start, confidently and wrongly.
		// The sibling GraphQL helper in this package (dgc_gql_client.go) takes the same stance.
		return nil, code, fmt.Errorf("GraphQL response contained no data for workflowDefinitionsGlobal (HTTP %d) — the field may be unavailable on this instance", code)
	}
	defs := make([]WorkflowDefinition, 0, len(resp.Data.API.WorkflowDefinitionsGlobal))
	for _, w := range resp.Data.API.WorkflowDefinitionsGlobal {
		defs = append(defs, WorkflowDefinition{
			ID:                          stripGraphQLGlobalIDPrefix(w.ID),
			Name:                        w.Name,
			Description:                 w.Description,
			StartLabel:                  w.StartLabel,
			Enabled:                     true, // by construction — see the query's doc comment
			FormRequired:                w.FormRequired,
			StartFormJSONModelAvailable: w.StartFormJSONModelAvailable,
			// By construction: this query only ever returns businessItemResourceType==GLOBAL
			// workflows — the field is defined as the no-resource set.
			BusinessItemResourceType: "GLOBAL",
		})
	}
	return defs, code, nil
}

// stripGraphQLGlobalIDPrefix strips the "<type>:" prefix Collibra's GraphQL id fields carry —
// confirmed live (a raw call returned "Workflow:697ee7bd-..." for a workflow whose real UUID is
// "697ee7bd-..."). The prefix is a schema-wide convention applied to every GraphQL entity's `id`,
// not something workflow-specific. A raw UUID never contains ':', so splitting on the
// first one is safe. Without this, the id returned here would fail GetWorkflowDefinition /
// start_workflow, which expect the plain UUID the REST API uses everywhere else.
func stripGraphQLGlobalIDPrefix(id string) string {
	if _, rawID, found := strings.Cut(id, ":"); found {
		return rawID
	}
	return id
}

// --- Legacy start form (BPMN <formProperty>) ---

// legacyFormFieldOptionWire is the wire shape of a DropdownValue on the legacy FormProperty.
// Three fields carry it and all three are mapped: enumValues (the "enum" and "dynamicEnum" types)
// and the proposedDropdownValues / defaultDropdownValues pair the server fills for every
// resource-PICKER type (legacyResourcePickerFormTypes). The fourth, multiProposedDropdownValues,
// is a DIFFERENT shape — a map keyed by resource type — and only AbstractMultiDropdownFormType
// populates it; its one concrete subclass is roleInCommunity, which this client reports as
// unsupported regardless (legacyRoleInCommunityFormType).
//
// idAsString, not id, is the value that must be sent back. The server parses `id` as a UUID and
// leaves it null whenever the option key is not one — which, for an ordinary textual enum, is
// always — while `idAsString` always carries the raw key. Submission is validated against that raw
// key, so idAsString is the only field that is reliably correct.
type legacyFormFieldOptionWire struct {
	IDAsString string `json:"idAsString"`
	Text       string `json:"text"`
}

// legacyFormFieldCheckOptionWire is the wire shape for checkButtons / radioButtons (populated for
// "checkbox", "dynamicCheckbox", "radiobox", "dynamicRadiobox" field types). Unlike the dropdown
// shape above, `value` maps straight through with no conversion — it is what must be sent back.
type legacyFormFieldCheckOptionWire struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type legacyFormPropertyWire struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Required   bool   `json:"required"`
	HelpText   string `json:"helpText,omitempty"`
	MultiValue bool   `json:"multiValue"`
	// Writable is TRUE for an ordinary field; the server rejects a submitted value for one that is
	// not. Note the inverted sense against WorkflowFormField.ReadOnly.
	//
	// A POINTER because absent must not mean read-only. The server declares this as a primitive
	// boolean and so always sends it, but the two failure directions are not symmetric: reading a
	// missing key as false marks EVERY field read-only, and a read-only field accepts no value —
	// which silently turns any form into one that cannot be filled in at all. Absent therefore
	// means writable, the behaviour from before this flag was honoured.
	Writable               *bool                            `json:"writable"`
	Value                  string                           `json:"value,omitempty"`
	EnumValues             []legacyFormFieldOptionWire      `json:"enumValues,omitempty"`
	CheckButtons           []legacyFormFieldCheckOptionWire `json:"checkButtons,omitempty"`
	RadioButtons           []legacyFormFieldCheckOptionWire `json:"radioButtons,omitempty"`
	ProposedDropdownValues []legacyFormFieldOptionWire      `json:"proposedDropdownValues,omitempty"`
	DefaultDropdownValues  []legacyFormFieldOptionWire      `json:"defaultDropdownValues,omitempty"`
	// MultiProposedDropdownValues is the roleInCommunity shape, and it is a MAP rather than the
	// flat array the single-dropdown types use: keyed by the server's ResourceType enum name, one
	// list of roles and one of their communities. A null entry decodes to a zero struct, which is
	// how "this role has no community" arrives — see toRolePairOptions.
	//
	// multiDefaultDropdownValues is deliberately not read: the DECLARED value already arrives in
	// Value, rendered as the `role(Name,Community)` expression the engine accepts straight back
	// (LegacyRoleExpressionPrefix), so reading the map would only add a second spelling of it.
	MultiProposedDropdownValues map[string][]legacyFormFieldOptionWire `json:"multiProposedDropdownValues,omitempty"`
	// ProposedFixed says whether the proposed values are the ONLY permitted ones. When false they
	// are suggestions and the server will accept an id outside the list.
	ProposedFixed bool `json:"proposedFixed"`
}

// The ResourceType enum names multiProposedDropdownValues is keyed by, spelled as the server
// serializes them (com.collibra.dgc.core.model.ResourceType: RL a role, CO a community).
const (
	legacyMultiDropdownRoleKey      = "RL"
	legacyMultiDropdownCommunityKey = "CO"
)

type legacyStartFormDataWire struct {
	ProcessID      string                   `json:"processId"`
	FormProperties []legacyFormPropertyWire `json:"formProperties"`
}

// WorkflowFormField is one field of a workflow's start form, normalized from either the legacy or
// the JSON model into a single shape. Value must be submitted keyed by ID.
//
// Type is the source model's own raw type name, not a normalized enum: a legacy FormProperty type
// ("enum", "user", "date", …) or a JSON-model col type ("text", "number", "checkboxGroup", …). It
// is set and load-bearing on BOTH models, each reading only its own vocabulary — the legacy names
// decide ResourcePicker (legacyResourcePickerFormTypes), the JSON ones decide which values leave
// as typed JSON rather than as strings (start_workflow's toTypedMap). Dropping it would silently
// turn every boolean and number back into a string.
type WorkflowFormField struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type,omitempty"`
	Required bool   `json:"required"`
	// Options is set only when this field's value must be one of a fixed set — submit one of
	// Options[].Key, never invent another value. Empty means free text (still subject to
	// whatever the server itself enforces, e.g. a date format).
	Options []WorkflowFormFieldOption `json:"options,omitempty"`
	// ResourcePicker is true when this field asks for a real Collibra resource (a user, group,
	// role, community, term, or similar) rather than a plain value or a fixed choice. Resolving one
	// is out of scope for this client — see ResolveWith for who does it.
	ResourcePicker bool `json:"resourcePicker,omitempty"`
	// ResolveWith names the route that produces a value for a ResourcePicker field, per resource
	// type — see legacyResourcePickerFormTypes. Empty means no tool here resolves that type.
	ResolveWith string `json:"resolveWith,omitempty"`
	// OptionsExhaustive reports whether Options is the COMPLETE set of legal values. Choice fields
	// (enum, checkbox, radio) and fixed pickers are exhaustive; a picker whose server-supplied list
	// is merely "proposed" is not, and there an id outside the list is still accepted.
	OptionsExhaustive bool `json:"optionsExhaustive,omitempty"`
	// MultiValue reports that the field takes SEVERAL values in one string, comma-separated. The
	// server splits on commas, and a single-value field rejects a list outright.
	MultiValue bool `json:"multiValue,omitempty"`
	// HelpText is the authored hint shown beside the field in Collibra's own UI, when there is one.
	HelpText string `json:"helpText,omitempty"`
	// VisibleWhen carries the field's visibility condition verbatim when it has one, instead of a
	// plain true. The condition cannot be evaluated here, so Required is still reported from the
	// field's own isRequired: guessing it away would be worse, because the server does NOT enforce
	// requiredness for a hidden field (its validator ignores visibility, and these start events
	// commonly disable field validation entirely) — so a field wrongly called optional is simply
	// never filled, and the process starts with it unset and no error anywhere.
	VisibleWhen string `json:"visibleWhen,omitempty"`
	// ReadOnly marks a field the form itself disables. Submitting a value for one is rejected.
	ReadOnly bool `json:"readOnly,omitempty"`
	// DefaultValue is the value the form itself pre-fills when a person opens it. Submitting
	// nothing for such a field is NOT the same as the user leaving it alone: the product would
	// have sent the default, so dropping it silently changes the outcome (e.g. an issue created
	// with no priority where the form says "Normal").
	DefaultValue string `json:"defaultValue,omitempty"`
	// IDPairs marks a field whose value is a JSON array of [id, id] PAIRS rather than a plain
	// value. Legacy-model only, and currently only roleInCommunity, where each element is
	// [roleId, communityId] and the community half may be an empty string. Options[].Key are
	// exactly those elements, so the caller composes the value by putting the keys it wants inside
	// one array.
	//
	// It deliberately does NOT follow the comma-separated convention MultiValue describes: the
	// engine reads this field with an ObjectMapper, so the commas are structure and splitting on
	// them would edit the payload. MultiValue still applies in its own right — it says whether more
	// than one pair is allowed, and the engine rejects a second pair when it is false.
	IDPairs bool `json:"idPairs,omitempty"`
	// Unsupported, when non-empty, explains why this client cannot help produce a value for the
	// field. It does not block submission — a caller that already knows a valid value may still
	// supply one — but it stops the tool from suggesting a resolution route that cannot work.
	Unsupported string `json:"unsupported,omitempty"`
}

// WorkflowFormFieldOption is one allowed value for a WorkflowFormField whose Options is set.
type WorkflowFormFieldOption struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// legacyResourcePickerFormTypes are the legacy-model field types that ask for a real Collibra
// resource rather than a plain value or a fixed choice: the concrete subclasses of the server's
// AbstractSingleDropdownFormType, which is exactly the set it populates proposedDropdownValues /
// defaultDropdownValues for.
//
// The value is the route that actually resolves one, and it is per type on purpose. "Resolve it via
// search_asset_keyword" is true only for the types search can return; for an assetType or a
// relationType it names a lookup that cannot succeed, and the caller then searches, fails and
// retries — the loop the Unsupported field exists to prevent, reintroduced by a generic hint.
//
// An empty value means NOTHING here resolves that type. That is a different statement from
// Unsupported: the value is perfectly producible, just not from this server, so the field stays
// answerable by a caller who knows the id.
var legacyResourcePickerFormTypes = map[string]string{
	"term":      `resolve it with search_asset_keyword (resourceTypeFilters: ["Asset"])`,
	"user":      `resolve it with search_asset_keyword (resourceTypeFilters: ["User"])`,
	"group":     `resolve it with search_asset_keyword (resourceTypeFilters: ["UserGroup"])`,
	"community": `resolve it with search_asset_keyword (resourceTypeFilters: ["Community"])`,
	// A vocabulary IS a domain — the older name for the same resource, which is why search's
	// Domain filter answers it.
	"vocabulary":    `resolve it with search_asset_keyword (resourceTypeFilters: ["Domain"])`,
	"assetType":     "resolve it with list_asset_types (id)",
	"domainType":    "resolve it with prepare_create_asset (domainTypeId, from a resolved domain)",
	"attributeType": "resolve it with prepare_create_asset (attributeSchema[].attributeTypeId)",
	"relationType":  "resolve it with prepare_create_asset (relationTypes[].relationTypeId)",
	// Deliberately empty: no tool here turns a role name into its id. Same gap as the role half of
	// unsupportedRoleInCommunity, and it closes the same way — by a role-resolution tool, not by
	// this package resolving one itself.
	"role": "",
}

// legacyFileUploadFormType cannot be filled via a plain form-property value at all.
const legacyFileUploadFormType = "fileUpload"

// The two palette stencils whose values this client cannot help produce — the JSON-model twins of
// legacyFileUploadFormType / legacyRoleInCommunityFormType below.
const (
	jsonFormFileUploadStencil      = "collibra-fileUpload"
	jsonFormRoleInCommunityStencil = "collibra-roleInCommunity"
)

const unsupportedSubform = "This is a sub-form: its fields live in a separate form definition that " +
	"is not included in this response, so they cannot be listed or filled in from here. Start the " +
	"workflow from Collibra's UI if any of them are needed."

// legacyRoleInCommunityFormType is deliberately NOT in legacyResourcePickerFormTypes: it is the
// one legacy multi-dropdown, and its value is not a resource id but a JSON array of positional
// [roleId, communityId] PAIRS — anything else is rejected outright. Treating it as an ordinary
// picker told the caller to "resolve a resource", after which a bare role id came back as a 400.
// The JSON model's twin stencil wants a different shape again; see unsupportedRoleInCommunityJSON.
const legacyRoleInCommunityFormType = "roleInCommunity"

// Reasons a legacy field cannot be answered from this client alone. Both are surfaced as
// WorkflowFormField.Unsupported so the caller is told what IS true instead of being pointed at a
// lookup that cannot succeed.
const (
	unsupportedFileUpload = "This field takes an uploaded file, which cannot be supplied through this API at all. " +
		"The workflow has to be started from Collibra's own UI if this field is required."
	// roleInCommunity is TWO messages, because the value shape differs by form model and telling a
	// caller the other model's shape is telling it something untrue for the form in front of it.
	// Legacy takes positional pairs, parsed with element.get(0)/get(1). The JSON model takes
	// objects keyed role/community — OOTB IssueManagementApp binds the component to `role` and its
	// script does `roleInCommunity.containsKey('role')` and `.get('community')` over that variable,
	// which only a map answers.
	//
	// The resolution advice is identical either way, so it is stated once in the shared tail. Its
	// two halves are NOT the same kind of statement, and only one is permanent: a community UUID is
	// resolvable today, so the tail names the tool that does it, the same shape ResourcePicker
	// fields use. The role half is a PLACEHOLDER — "you must already know the UUID" holds only
	// because no tool here resolves a role name yet, not because the value cannot be produced. The
	// client plumbing exists already (ListRoles).
	//
	// When a role-resolution tool is added, REPLACE the role clause with a pointer to it. Do not
	// instead call ListRoles from start_workflow: resolving a resource is not that tool's job, and
	// doing it inline would duplicate whatever the new tool does — the reason ResourcePicker fields
	// delegate rather than resolve.
	unsupportedRoleInCommunityTail = " A community UUID can be resolved with " +
		`search_asset_keyword (resourceTypeFilters: ["Community"])` + ", and the community half is optional when " +
		"the role is not scoped to one. No tool here resolves a role NAME to its id, so the role UUID has to be " +
		"one you already know; otherwise start the workflow from Collibra's UI."
	unsupportedRoleInCommunityLegacy = "This field takes a JSON array of positional [roleId, communityId] pairs, " +
		"e.g. " + `[["<role-uuid>","<community-uuid>"]]` + " — not a single id; pass an empty string for an " +
		"absent community." + unsupportedRoleInCommunityTail
	unsupportedRoleInCommunityJSON = "This field takes a JSON array of objects keyed role/community, e.g. " +
		`[{"role":"<role-uuid>","community":"<community-uuid>"}]` + " — not a single id, and NOT the legacy " +
		"model's positional array; omit the community key when there is none." + unsupportedRoleInCommunityTail
	unsupportedFullStorage = "This form stores the WHOLE picked resource for this field, not its id, so the process " +
		"reads properties off it. Only an id can be produced here, and an id where an object is expected fails the " +
		"start outright — so start this workflow from Collibra's UI if this field is needed."
)

// jsonFormStorageMode reports how a picker's value is stored. "Id" (and an absent setting) means
// the variable is the plain resource id, which is what this client can supply. "Full" means the
// variable is the whole picked resource and the process reads properties off it — seen in the OOTB
// corpus, where a start script does UUID.fromString("${responsibleCommunity.value}"). Handing that
// a bare id string resolves .value to nothing and the start dies inside its transaction, so such a
// field is declared unsupported rather than filled in with a guess at the object's shape.
func jsonFormStorageMode(col map[string]any) string {
	extraSettings, ok := col["extraSettings"].(map[string]any)
	if !ok {
		return ""
	}
	return stringFromMap(extraSettings, "storage")
}

// GetWorkflowStartFormData fetches the LEGACY (BPMN <formProperty>) start-form schema for a
// workflow definition. Call only when StartFormJSONModelAvailable is false — see the package
// comment; the endpoint returns an empty field list (not an error) for a JSON-model workflow.
// Returns the HTTP status alongside the error, like the rest of this client, so the caller can
// tell a permission failure from a transport one instead of emitting one message for both.
func GetWorkflowStartFormData(ctx context.Context, client *http.Client, workflowDefinitionID string) ([]WorkflowFormField, int, error) {
	endpoint := "/rest/2.0/workflowDefinitions/workflowDefinition/" + url.PathEscape(workflowDefinitionID) + "/startFormData"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}
	body, code, err := executeCollibraRequestWithStatus(client, req)
	if err != nil {
		return nil, code, err
	}
	var wire legacyStartFormDataWire
	if jsonErr := json.Unmarshal(body, &wire); jsonErr != nil {
		return nil, code, fmt.Errorf("failed to parse start form data: %w", jsonErr)
	}
	fields := make([]WorkflowFormField, 0, len(wire.FormProperties))
	for _, p := range wire.FormProperties {
		fields = append(fields, toLegacyFormField(p))
	}
	return fields, code, nil
}

// legacyButtonFormTypes render as an action button. They are ordinary submitted booleans as far as
// the engine is concerned — the process may well branch on one — so they are NOT hidden. What is
// dropped is their reported value: the button renderer answers "false" for every model value it is
// given, null included, so the value says nothing about the field and only invites the caller to
// submit a "default" that was never declared. Measured September 2026 over 225 BPMN files (75
// button declarations): the declared default was "false" 52 times and absent 23 times, never
// anything else — so dropping it discarded no information in any observed case.
//
// KNOWN LIMITATION, accepted deliberately. Dropping the value also drops the signal
// validateFormProperties uses to tell "the engine has a default for this" from "nobody has one",
// so a REQUIRED button with a declared default would be demanded from the caller although the
// engine would have resolved it. That costs one extra question whose answer is legal either way —
// a safe failure. Restoring the signal would mean carrying a second default-ish field through the
// public shape for a case that does not occur: in that same corpus every required button is an
// approve/reject on a USER TASK, and this tool only ever reads START forms.
var legacyButtonFormTypes = map[string]bool{"button": true, "activityButton": true, "taskButton": true}

func toLegacyFormField(p legacyFormPropertyWire) WorkflowFormField {
	resolveWith, isPicker := legacyResourcePickerFormTypes[p.Type]
	defaultValue := p.Value
	if legacyButtonFormTypes[p.Type] {
		defaultValue = ""
	}
	// A form property may carry no label at all; falling back to the id keeps the caller from
	// being shown a nameless field, which is what the JSON path already does.
	name := p.Name
	if name == "" {
		name = p.ID
	}
	field := WorkflowFormField{
		ID:             p.ID,
		Name:           name,
		Type:           p.Type,
		Required:       p.Required,
		HelpText:       p.HelpText,
		MultiValue:     p.MultiValue,
		DefaultValue:   defaultValue,
		ReadOnly:       p.Writable != nil && !*p.Writable,
		ResourcePicker: isPicker,
		ResolveWith:    resolveWith,
	}
	switch p.Type {
	case legacyFileUploadFormType:
		field.Unsupported = unsupportedFileUpload
	case legacyRoleInCommunityFormType:
		field.Unsupported = unsupportedRoleInCommunityLegacy
	}

	switch {
	case len(p.EnumValues) > 0:
		field.Options, field.OptionsExhaustive = toDropdownOptions(p.EnumValues), true
	case len(p.CheckButtons) > 0:
		field.Options, field.OptionsExhaustive = toCheckOptions(p.CheckButtons), true
	case len(p.RadioButtons) > 0:
		field.Options, field.OptionsExhaustive = toCheckOptions(p.RadioButtons), true
	case len(p.ProposedDropdownValues) > 0, len(p.DefaultDropdownValues) > 0:
		// A resource picker whose legal values the server already sent. Dropping these used to
		// strand the caller: the field said "resolve a real resource first", while this client has
		// no way to resolve a role, group, domain type, attribute type or relation type — a dead
		// end even though the answer was in the response all along. Only proposedFixed makes the
		// list closed; otherwise these are suggestions and an id outside them is still valid.
		field.Options = toDropdownOptions(append(append([]legacyFormFieldOptionWire{}, p.ProposedDropdownValues...), p.DefaultDropdownValues...))
		field.OptionsExhaustive = p.ProposedFixed
	case len(p.MultiProposedDropdownValues) > 0:
		// The pairs the form itself proposes. Clearing Unsupported is the point of the branch: the
		// field stays unanswerable only while nothing here can produce a value for it, and once the
		// server has handed over the legal pairs that is no longer true.
		if options := toRolePairOptions(p.MultiProposedDropdownValues); len(options) > 0 {
			field.Options, field.OptionsExhaustive = options, p.ProposedFixed
			field.IDPairs = true
			field.Unsupported = ""
		}
	}
	return field
}

// LegacyRoleExpressionPrefix marks the OTHER shape a roleInCommunity value may take: a
// `role(Name,Community)` expression. That is what the engine renders a DECLARED value as, so it is
// what arrives in WorkflowFormField.DefaultValue — and the engine accepts it back untouched
// (RoleInCommunityFormType#convertFormValueToModelValue returns early on it). Anything validating
// such a value has to allow both shapes, or it rejects the form's own value handed straight back.
const LegacyRoleExpressionPrefix = "role("

// WorkflowFormFieldIDPairKey renders one [id, id] pair the way the engine parses it back: compact
// JSON, escaped by the encoder rather than by hand. Options[].Key for an IDPairs field are built
// with it, and a caller's value is normalized through it before being compared against them — so
// spacing the caller happened to write cannot make a legal pair look unlisted.
func WorkflowFormFieldIDPairKey(pair []string) string {
	encoded, _ := json.Marshal(pair)
	return string(encoded)
}

// toRolePairOptions pairs the role list with the community list BY INDEX. That is how the server
// builds them — RoleInCommunityFormType#getDropdownValues appends one entry to each list per
// declared expression, and appends a null community for a role that has none — so index i of "CO"
// belongs to index i of "RL" and may be absent.
//
// Reading this map is what makes the field answerable at all. Without it the caller was told to
// resolve a role id, which no tool here can do, while the legal pairs sat unread in this very
// response — the same dead end proposedDropdownValues used to produce.
func toRolePairOptions(multi map[string][]legacyFormFieldOptionWire) []WorkflowFormFieldOption {
	roles := multi[legacyMultiDropdownRoleKey]
	communities := multi[legacyMultiDropdownCommunityKey]
	out := make([]WorkflowFormFieldOption, 0, len(roles))
	seen := make(map[string]bool, len(roles))
	for i, role := range roles {
		if role.IDAsString == "" {
			continue // a null role carries nothing that could be submitted
		}
		var community legacyFormFieldOptionWire
		if i < len(communities) {
			community = communities[i]
		}
		key := WorkflowFormFieldIDPairKey([]string{role.IDAsString, community.IDAsString})
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, WorkflowFormFieldOption{Key: key, Label: rolePairLabel(role.Text, community.Text)})
	}
	return out
}

// rolePairLabel names the pair the way a person would read it out. The community half is optional,
// and a trailing " in " with nothing after it would read as a truncated label.
func rolePairLabel(roleName, communityName string) string {
	if communityName == "" {
		return roleName
	}
	return roleName + " in " + communityName
}

func toDropdownOptions(opts []legacyFormFieldOptionWire) []WorkflowFormFieldOption {
	out := make([]WorkflowFormFieldOption, 0, len(opts))
	seen := make(map[string]bool, len(opts))
	for _, v := range opts {
		if v.IDAsString == "" || seen[v.IDAsString] {
			continue // proposed and default lists overlap; the same id twice helps nobody
		}
		seen[v.IDAsString] = true
		out = append(out, WorkflowFormFieldOption{Key: v.IDAsString, Label: v.Text})
	}
	return out
}

func toCheckOptions(opts []legacyFormFieldCheckOptionWire) []WorkflowFormFieldOption {
	out := make([]WorkflowFormFieldOption, len(opts))
	for i, v := range opts {
		out[i] = WorkflowFormFieldOption{Key: v.Value, Label: v.Label}
	}
	return out
}

// WorkflowFormFieldIsResourcePicker reports whether field asks for a real Collibra resource (or
// a file) that this client cannot resolve a value for on its own. Set for both form models: by
// field type in the legacy one (legacyResourcePickerFormTypes), by the collibra- palette stencil
// in the JSON one (jsonFormCollibraStencilPrefix).
func WorkflowFormFieldIsResourcePicker(field WorkflowFormField) bool {
	return field.ResourcePicker
}

// --- JSON start form (Workflow Designer .form model) ---

// flowableFormModelWire mirrors com.flowable.form.model.FlowableFormModel — the ENTERPRISE
// Flowable form model (note: com.flowable, not org.flowable; it is NOT org.flowable.form.model.
// SimpleFormModel, which has a flat `fields` array and does not apply here). Verified against
// the published enterprise form-model classes and against real .form files produced by the
// designer.
//
// Fields are NOT a flat list: they live in rows[].cols[], and each col is an untyped
// Map<String,Object> (RowDefinition.cols is literally List<Map<String,Object>>), so the field
// properties are parsed by key rather than into a typed struct.
type flowableFormModelWire struct {
	Rows []rowDefinitionWire `json:"rows"`
}

type rowDefinitionWire struct {
	Cols []map[string]any `json:"cols"`
}

// jsonFormContainerTypes are FormFieldTypes.CONTAINER_TYPES — layout wrappers, not input fields.
// They hold nested rows rather than a value of their own, so they are skipped as fields and
// recursed into instead.
var jsonFormContainerTypes = map[string]bool{
	"accordion": true, "columns": true, "flexLayout": true, "panel": true,
	"subform": true, "tabs": true, "wizard": true, "masterDetail": true,
}

// jsonFormCollibraStencilPrefix marks Collibra's own form components in the Workflow Designer
// palette — collibra-user, collibra-group, collibra-role, collibra-asset, collibra-domain,
// collibra-community, collibra-assetType, collibra-domainType, collibra-attributeType,
// collibra-relationType, collibra-roleInCommunity, collibra-fileUpload (the full set, from
// the designer's own palette reference). Every one of them asks for a real
// Collibra resource, so the prefix alone is a reliable resource-picker signal — the generic
// cloud-* stencils never carry it. Read from designInfo.stencilId, since the serialized `type`
// for these collapses to a generic FormFieldTypes value and cannot distinguish them.
const jsonFormCollibraStencilPrefix = "collibra-"

// workflowStartFormJSONModelQuery calls the ONLY entry point for this data — see the package
// comment: the JSON start-form schema has no REST resource, only this GraphQL field.
const workflowStartFormJSONModelQuery = `query WorkflowStartFormJsonModel($workflowDefinitionId: ID!) {
  api {
    workflowStartFormJsonModel(workflowDefinitionId: $workflowDefinitionId)
  }
}`

type workflowStartFormJSONModelResponse struct {
	Data *struct {
		API *struct {
			WorkflowStartFormJSONModel *string `json:"workflowStartFormJsonModel"`
		} `json:"api"`
	} `json:"data"`
	Errors []Error `json:"errors"`
}

// GetWorkflowStartFormJSONModel fetches the JSON-model start-form schema for a workflow
// definition. Call only when StartFormJSONModelAvailable is true.
//
// The response's workflowStartFormJsonModel is itself a JSON-encoded STRING (a serialized
// FlowableFormModel — see flowableFormModelWire) — Collibra's GraphQL field returns the form
// model as a string, not a native GraphQL object, so this function unmarshals twice.
// Returns the HTTP status alongside the error — see GetWorkflowStartFormData. It also goes through
// the envelope-aware helper, so a Collibra errorCode/userMessage reaches the caller rather than
// being flattened into a bare transport error.
func GetWorkflowStartFormJSONModel(ctx context.Context, client *http.Client, workflowDefinitionID string) ([]WorkflowFormField, int, error) {
	reqBody := Request{
		Query:     workflowStartFormJSONModelQuery,
		Variables: map[string]interface{}{"workflowDefinitionId": workflowDefinitionID},
	}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to marshal start form json model query: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/graphql", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	body, code, err := executeCollibraRequestWithStatus(client, req)
	if err != nil {
		return nil, code, err
	}
	var resp workflowStartFormJSONModelResponse
	if jsonErr := json.Unmarshal(body, &resp); jsonErr != nil {
		return nil, code, fmt.Errorf("failed to parse start form json model response: %w", jsonErr)
	}
	if len(resp.Errors) > 0 {
		return nil, code, fmt.Errorf("start form json model query errors: %v", resp.Errors)
	}
	if resp.Data == nil || resp.Data.API == nil || resp.Data.API.WorkflowStartFormJSONModel == nil {
		// This is only ever called for a definition whose startFormJsonModelAvailable flag is set,
		// so a null model means the flag and the stored form disagree — a real failure, not a
		// workflow that happens to have no fields. Returning no fields here would let the caller
		// submit an empty form and report success. An EMPTY rows array is a different thing and
		// remains legitimate: it parses below and yields zero fields.
		return nil, code, fmt.Errorf("workflow %s is flagged as having a JSON start form, but the server returned no form model for it", workflowDefinitionID)
	}
	var model flowableFormModelWire
	if jsonErr := json.Unmarshal([]byte(*resp.Data.API.WorkflowStartFormJSONModel), &model); jsonErr != nil {
		return nil, code, fmt.Errorf("failed to parse embedded form model: %w", jsonErr)
	}
	var fields []WorkflowFormField
	collectJSONFormFields(model.Rows, &fields)
	return fields, code, nil
}

// jsonFormFieldBinding matches a col's `value` when it is a single {{variable}} expression. That
// variable — NOT the col's `id` — is the process variable the field writes to, and therefore the
// key a start request must submit it under. `id` is the designer's element id and routinely
// differs: in the OOTB "Propose New Asset" form the ids are text1 / parent-asset-type /
// collibra-assetType6 while the bound variables are signifier / intakeVocabulary / conceptType.
// Submitting under the id leaves those variables unset, and because such start events commonly
// carry flowable:formFieldValidation="false" the start SUCCEEDS — silently creating an empty
// result. (Measured over the OOTB form corpus, September 2026: 20 of 35 fields diverge.)
//
// Deliberately conservative: only a plain identifier counts as a binding. Anything else (a
// computed expression, a literal) falls back to the id rather than inventing a key.
var jsonFormFieldBinding = regexp.MustCompile(`^\s*\{\{\s*([A-Za-z_][A-Za-z0-9_]*)\s*\}\}\s*$`)

// jsonFormBindingBraces finds a {{...}} binding anywhere in a col's `value`. The engine's own rule
// is simply "if value contains {{, the field writes to a variable"; display-only components
// (a horizontal rule, a link, a read-only output) have no binding, and emitting them as fillable
// fields invites the caller to overwrite an existing value or to invent junk process variables.
//
// It is a FALLBACK that fires on nothing measured — every binding across the OOTB and
// customizations corpora is an exact single expression matched by the strict pattern above
// (September 2026: 37 of 37). It stays regardless: dropping it would make this client stricter
// than the engine, so a field the engine WOULD bind (a binding written with surrounding text, a
// shape the designer does not currently emit) would vanish from the form with no error and the
// caller would start the workflow with that variable unset. That silent omission is the failure
// this whole area exists to prevent; an extra field the caller can see in the preview is the
// lesser risk.
var jsonFormBindingBraces = regexp.MustCompile(`\{\{\s*([^{}]+?)\s*\}\}`)

// collectJSONFormFields walks rows[].cols[], appending every input field and recursing into
// layout containers.
//
// Container nesting follows the engine's own traversal rather than a structural guess. A container
// keeps its children under extraSettings, in one of three shapes:
//
//	extraSettings.layoutDefinition.rows[].cols[]  — panel, subform
//	extraSettings.sections[]                      — tabs, accordion, wizard, masterDetail, columns,
//	                                                flexLayout; each entry is itself COL-shaped
//	                                                (a panel), NOT a row
//	extraSettings.expandablePanel                 — a single nested col
//
// Guessing structurally instead — scanning a col's direct values for anything rows-shaped — finds
// nothing, because nothing is stored that way: every field inside a container is dropped, the form
// yields zero fields and no error, and the workflow is then started with nothing filled in.
func collectJSONFormFields(rows []rowDefinitionWire, out *[]WorkflowFormField) {
	for _, row := range rows {
		collectJSONFormCols(row.Cols, out)
	}
}

func collectJSONFormCols(cols []map[string]any, out *[]WorkflowFormField) {
	for _, col := range cols {
		if col == nil || boolFromMap(col, "ignore") {
			continue
		}
		fieldType := stringFromMap(col, "type")
		// By declared type OR by shape. Matching on the type name alone fails open: a container
		// this client has not heard of is treated as a plain field, and its entire subtree is
		// dropped without a word. Anything carrying container-shaped extraSettings is therefore
		// walked as a container regardless of what it calls itself.
		if jsonFormContainerTypes[fieldType] || hasNestedFormContent(col) {
			collectNestedJSONFormRows(col, out)
			continue
		}
		// Gate on there being a real binding, not on the element id existing: the engine derives a
		// variable only from `value`, so a col without one is decoration.
		if key := jsonFormSubmissionKey(col); key != "" {
			*out = append(*out, toJSONFormField(col, key, fieldType))
		}
	}
}

// collectNestedJSONFormRows recurses into a container col's children — see collectJSONFormFields
// for the three shapes a container can use.
func collectNestedJSONFormRows(col map[string]any, out *[]WorkflowFormField) {
	extraSettings, ok := col["extraSettings"].(map[string]any)
	if !ok {
		return
	}
	before := len(*out)
	if layout, ok := extraSettings["layoutDefinition"].(map[string]any); ok {
		collectJSONFormFields(asRowDefinitions(layout["rows"]), out)
	}
	if sections, ok := extraSettings["sections"].([]any); ok {
		collectJSONFormCols(asCols(sections), out)
	}
	if panel, ok := extraSettings["expandablePanel"].(map[string]any); ok {
		// Through collectJSONFormCols, not straight into the recursion: the panel is itself a col
		// and may be marked ignore, and skipping that check here made the two routes to the same
		// panel disagree — one honoured ignore, the other harvested its fields anyway.
		collectJSONFormCols([]map[string]any{panel}, out)
	}

	// A sub-form keeps its fields in a SEPARATE form, referenced by extraSettings.formRef; the
	// designer palette stores no layout for it. Depending on the deployment the referenced layout
	// may or may not be inlined by the time this client sees it — when it is not, staying silent
	// would present a partial form as a complete one and start the workflow with the sub-form's
	// variables unset. Say so instead.
	if len(*out) == before {
		if ref := firstStringFromMap(extraSettings, "formRef", "formKey"); ref != "" {
			*out = append(*out, WorkflowFormField{
				ID:          stringFromMap(col, "id"),
				Name:        jsonFormFieldLabel(col, stringFromMap(col, "id")),
				Type:        stringFromMap(col, "type"),
				Unsupported: unsupportedSubform,
			})
		}
	}
}

// hasNestedFormContent reports a col that carries children, whatever its type claims to be.
func hasNestedFormContent(col map[string]any) bool {
	extraSettings, ok := col["extraSettings"].(map[string]any)
	if !ok {
		return false
	}
	for _, key := range []string{"layoutDefinition", "sections", "expandablePanel"} {
		if _, present := extraSettings[key]; present {
			return true
		}
	}
	return false
}

// asRowDefinitions converts a rows list, SKIPPING any entry that does not conform rather than
// discarding the whole list — one malformed sibling must not delete every other field in the
// subtree.
func asRowDefinitions(v any) []rowDefinitionWire {
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	rows := make([]rowDefinitionWire, 0, len(list))
	for _, entry := range list {
		entryMap, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		rawCols, ok := entryMap["cols"].([]any)
		if !ok {
			continue
		}
		rows = append(rows, rowDefinitionWire{Cols: asCols(rawCols)})
	}
	return rows
}

// asCols converts a list of col-shaped values, skipping non-conforming entries.
func asCols(list []any) []map[string]any {
	cols := make([]map[string]any, 0, len(list))
	for _, c := range list {
		if colMap, ok := c.(map[string]any); ok {
			cols = append(cols, colMap)
		}
	}
	return cols
}

// toJSONFormField maps one col to a WorkflowFormField. Key names are those of the enterprise form
// model's common attributes: `id`, `type`, `label` (NOT `name`), `isRequired` (NOT `required`).
// Note ID is the BOUND VARIABLE, not the col's id — see jsonFormFieldBinding.
func toJSONFormField(col map[string]any, key, fieldType string) WorkflowFormField {
	stencil := jsonFormStencilID(col)
	field := WorkflowFormField{
		ID:             key,
		Name:           jsonFormFieldLabel(col, key),
		Type:           fieldType,
		Required:       boolFromMap(col, "isRequired"),
		VisibleWhen:    jsonFormVisibilityCondition(col),
		ReadOnly:       jsonFormIsReadOnly(col),
		DefaultValue:   jsonFormDefaultValue(col),
		MultiValue:     jsonFormIsMultiValue(col),
		ResourcePicker: strings.HasPrefix(stencil, jsonFormCollibraStencilPrefix),
		ResolveWith:    jsonFormResolveWith(stencil),
		Unsupported:    jsonFormUnsupported(stencil),
	}
	if field.Unsupported == "" {
		if mode := jsonFormStorageMode(col); mode != "" && !strings.EqualFold(mode, "Id") {
			field.Unsupported = unsupportedFullStorage
		}
	}
	field.Options = jsonFormFieldOptions(col)
	field.OptionsExhaustive = len(field.Options) > 0
	return field
}

// VisibleWhenNever is the VisibleWhen value for a field the form hides unconditionally. It is a
// constant rather than a literal because callers BRANCH on it — a field that is never shown must
// not have its default submitted — and an edit to the wording alone would silently turn that
// branch off, with every test still green.
const VisibleWhenNever = "never (the form hides this field)"

// jsonFormVisibilityCondition returns the raw condition when `visible` is anything other than a
// plain boolean true — see WorkflowFormField.VisibleWhen for why the field stays required.
func jsonFormVisibilityCondition(col map[string]any) string {
	v, present := col["visible"]
	if !present {
		return ""
	}
	if b, ok := v.(bool); ok {
		if b {
			return ""
		}
		return VisibleWhenNever
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func jsonFormIsReadOnly(col map[string]any) bool {
	v, present := col["enabled"]
	if !present {
		return false
	}
	b, ok := v.(bool)
	return ok && !b
}

// jsonFormIsMultiValue reports a field that accepts several values at once — a multi-select or a
// checkbox group. The flag lives in extraSettings, not on the col.
func jsonFormIsMultiValue(col map[string]any) bool {
	// Two sources, because the designer only writes the `multi` flag for the select components.
	// A checkbox group or a tags input takes several values with no flag at all, and treating one
	// as single-valued rejects every list the caller could offer, with nothing in the response
	// hinting a list is even legal.
	switch strings.ToLower(stringFromMap(col, "type")) {
	case "checkboxgroup", "tags", "multiselect":
		return true
	}
	extraSettings, ok := col["extraSettings"].(map[string]any)
	if !ok {
		return false
	}
	// TWO spellings, because the palette is two palettes. The generic cloud components write
	// "multi"; the collibra-* resource pickers write "multiValue". Reading only the first reported
	// every asset/user/group picker as single-valued — measured on an OOTB form whose two pickers
	// both declare multiValue:true — and a field reported single-valued is submitted as a bare
	// string, which the process then iterates one CHARACTER at a time.
	return boolFromMap(extraSettings, "multi") || boolFromMap(extraSettings, "multiValue")
}

// jsonFormDefaultValue renders the form's pre-filled value. It is NOT always a string — the
// palette declares a checkbox default as a real boolean, and a picker's as an array — and reading
// it with a string-only accessor silently dropped those, which then reached the process unset.
// An empty collection is treated as no default, since there is nothing to submit.
func jsonFormDefaultValue(col map[string]any) string {
	switch v := col["defaultValue"].(type) {
	case string:
		return v
	case bool:
		return strconv.FormatBool(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case []any:
		// A picker or multi-select defaults to a list. An empty one carries no value to submit;
		// a populated one is rendered as the comma-separated form a multi-value field expects.
		parts := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ",")
	}
	return ""
}

// jsonFormResolveWith reuses the legacy routes for the JSON model: a collibra- palette stencil is
// named after the legacy form type it stands in for ("collibra-user", "collibra-assetType"), so one
// lookup answers both models and they cannot drift apart. A stencil the map does not know yields no
// hint rather than a guessed one, which leaves the caller with the generic "this needs a real
// resource" — no worse off than before any route was named.
func jsonFormResolveWith(stencil string) string {
	if !strings.HasPrefix(stencil, jsonFormCollibraStencilPrefix) {
		return ""
	}
	return legacyResourcePickerFormTypes[strings.TrimPrefix(stencil, jsonFormCollibraStencilPrefix)]
}

// jsonFormUnsupported mirrors the legacy path's honesty for the JSON model: a file upload cannot
// be produced through this API at all, and a role-in-community needs a paired structure rather
// than a resource id. Without this the caller is told to "resolve it via search" and loops.
func jsonFormUnsupported(stencil string) string {
	switch stencil {
	case jsonFormFileUploadStencil:
		return unsupportedFileUpload
	case jsonFormRoleInCommunityStencil:
		return unsupportedRoleInCommunityJSON
	}
	return ""
}

// jsonFormSubmissionKey returns the process variable this col writes to, or "" when it writes to
// none (a display-only component). A single {{identifier}} is the overwhelmingly common shape and
// is taken verbatim; anything else containing braces still binds a variable in the engine, so the
// braces are stripped rather than falling back to the element id — falling back would submit a
// name the process never reads, and with field validation disabled that fails silently.
func jsonFormSubmissionKey(col map[string]any) string {
	raw := stringFromMap(col, "value")
	if m := jsonFormFieldBinding.FindStringSubmatch(raw); m != nil {
		return m[1]
	}
	if m := jsonFormBindingBraces.FindStringSubmatch(raw); m != nil {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// jsonFormFieldLabel prefers the authored label, but a label can itself be a Flowable expression
// (one in the OOTB corpus picks between two wordings depending on another field). Showing that raw
// is worse than showing the field's id.
func jsonFormFieldLabel(col map[string]any, id string) string {
	label := stringFromMap(col, "label")
	if label == "" || strings.Contains(label, "{{") {
		return id
	}
	return label
}

// jsonFormStencilID reads designInfo.stencilId — the Workflow Designer palette component this
// field was authored from (e.g. "collibra-user", "cloud-text").
func jsonFormStencilID(col map[string]any) string {
	designInfo, ok := col["designInfo"].(map[string]any)
	if !ok {
		return ""
	}
	return stringFromMap(designInfo, "stencilId")
}

// jsonFormFieldOptions extracts a fixed choice list from extraSettings.items, which is where a
// statically-configured select/radio keeps its options. Options sourced dynamically instead
// (extraSettings.dataSource / queryUrl / lookupUrl) are deliberately NOT resolved: returning no
// options leaves the field as free text, so a wrong value fails loudly at start time rather than
// this client inventing a key. Entry shapes vary, so only unambiguous ones are accepted.
func jsonFormFieldOptions(col map[string]any) []WorkflowFormFieldOption {
	extraSettings, ok := col["extraSettings"].(map[string]any)
	if !ok {
		return nil
	}
	items, ok := extraSettings["items"].([]any)
	if !ok || len(items) == 0 {
		return nil
	}
	options := make([]WorkflowFormFieldOption, 0, len(items))
	for _, item := range items {
		switch v := item.(type) {
		case string:
			options = append(options, WorkflowFormFieldOption{Key: v, Label: v})
		case map[string]any:
			key := firstStringFromMap(v, "value", "id", "key")
			if key == "" {
				return nil // unrecognised shape — emit nothing rather than a guessed key
			}
			label := firstStringFromMap(v, "label", "name", "text")
			if label == "" {
				label = key
			}
			options = append(options, WorkflowFormFieldOption{Key: key, Label: label})
		default:
			return nil
		}
	}
	return options
}

func stringFromMap(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

func firstStringFromMap(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := m[k].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

func boolFromMap(m map[string]any, key string) bool {
	b, _ := m[key].(bool)
	return b
}

// --- Starting an instance ---

// StartWorkflowInstanceRequest is the body for POST /rest/2.0/workflowInstances.
// BusinessItemIDs/BusinessItemType are omitted for a GLOBAL
// workflow, which has no associated resource.
type StartWorkflowInstanceRequest struct {
	WorkflowDefinitionID string            `json:"workflowDefinitionId"`
	BusinessItemIDs      []string          `json:"businessItemIds,omitempty"`
	BusinessItemType     string            `json:"businessItemType,omitempty"`
	FormProperties       map[string]string `json:"formProperties,omitempty"`
}

// WorkflowInstance is the subset of a started instance this client needs.
type WorkflowInstance struct {
	ID string `json:"id"`
}

// StartWorkflowInstance starts a LEGACY-form (or no-form) workflow instance. POST
// /rest/2.0/workflowInstances returns 201 with a JSON array of started instances (one per
// business item); this client only ever starts one instance per call, so the first entry is
// returned. Returns the HTTP status code so callers can distinguish 403 (no permission) / 404
// (definition gone) from other failures.
func StartWorkflowInstance(ctx context.Context, client *http.Client, reqBody StartWorkflowInstanceRequest) (*WorkflowInstance, int, error) {
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/rest/2.0/workflowInstances", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	body, code, err := executeCollibraRequestWithStatus(client, req)
	if err != nil {
		return nil, code, err
	}
	var instances []WorkflowInstance
	if jsonErr := json.Unmarshal(body, &instances); jsonErr != nil {
		return nil, code, fmt.Errorf("failed to parse workflow instances: %w", jsonErr)
	}
	if len(instances) == 0 {
		return nil, code, fmt.Errorf("workflow start returned no instances")
	}
	return &instances[0], code, nil
}

// StartWorkflowInstanceWithFormRequest is the body for
// POST /rest/2.0/internal/workflow/startWithForm — see the package comment for why this endpoint
// is used instead of the public one.
type StartWorkflowInstanceWithFormRequest struct {
	WorkflowDefinitionID string                 `json:"workflowDefinitionId"`
	BusinessItemIDs      []string               `json:"businessItemIds,omitempty"`
	BusinessItemType     string                 `json:"businessItemType,omitempty"`
	FormProperties       map[string]interface{} `json:"formProperties,omitempty"`
}

// StartWorkflowInstanceWithForm starts a JSON-model workflow instance. Same response shape and
// single-instance convention as StartWorkflowInstance.
func StartWorkflowInstanceWithForm(ctx context.Context, client *http.Client, reqBody StartWorkflowInstanceWithFormRequest) (*WorkflowInstance, int, error) {
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/rest/2.0/internal/workflow/startWithForm", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	body, code, err := executeCollibraRequestWithStatus(client, req)
	if err != nil {
		return nil, code, err
	}
	var instances []WorkflowInstance
	if jsonErr := json.Unmarshal(body, &instances); jsonErr != nil {
		return nil, code, fmt.Errorf("failed to parse workflow instances: %w", jsonErr)
	}
	if len(instances) == 0 {
		return nil, code, fmt.Errorf("workflow start returned no instances")
	}
	return &instances[0], code, nil
}
