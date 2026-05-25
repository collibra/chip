// Package create_asset implements the create_asset MCP tool: a single
// smart write tool for creating any Collibra asset. The agent supplies
// human-friendly identifiers (UUIDs, publicIds, or display names) for
// asset type, domain, status, and attributes; the server resolves them
// against Collibra's scoped assignment, gates a duplicate-name check
// (default-on), converts Markdown to HTML for RICH_TEXT attribute values,
// and writes the asset and its attributes.
//
// create_asset replaces the four-tool flow (prepare_add_business_term,
// add_business_term, prepare_create_asset, create_asset) with one
// edit_asset-style entry point. Calling prepare_create_asset
// first is optional — useful only for discovery — because every
// resolution and validation step lives here too.
package create_asset

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/attrwrite"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// maxOptionsInError caps the count of option names appended to a
// validation error message — keeps payloads small for LLM context.
const maxOptionsInError = 30

// OutputStatus is the overall outcome of a create_asset call.
type OutputStatus string

const (
	// StatusSuccess means the asset was created. attributeResults reports
	// per-attribute outcomes, which may include individual errors that
	// did not block asset creation itself.
	StatusSuccess OutputStatus = "success"
	// StatusDuplicateFound means an existing asset with the same name in
	// the resolved (assetType, domain) was found and allowDuplicate was
	// false — no write occurred. The agent can re-call with
	// allowDuplicate=true after confirming with the user.
	StatusDuplicateFound OutputStatus = "duplicate_found"
	// StatusValidationError means resolution or validation failed before
	// any write — invalid asset type/domain, type not allowed in domain,
	// unknown attribute, etc. The message includes suggestions.
	StatusValidationError OutputStatus = "validation_error"
	// StatusError means the asset itself could not be created due to a
	// downstream Collibra error.
	StatusError OutputStatus = "error"
)

// Input is the tool's typed input.
type Input struct {
	Name                        string           `json:"name" jsonschema:"Required. Name of the new asset."`
	AssetType                   string           `json:"assetType" jsonschema:"Required. Identifier for the asset type — accepts a UUID, the type's publicId (e.g. 'BusinessTerm'), or its display name (e.g. 'Business Term'). Resolved server-side."`
	Domain                      string           `json:"domain" jsonschema:"Required. Identifier for the target domain — accepts a UUID or the domain's display name (case-insensitive)."`
	DisplayName                 string           `json:"displayName,omitempty" jsonschema:"Optional. Separate display name. Defaults to name when omitted."`
	Status                      string           `json:"status,omitempty" jsonschema:"Optional. Initial status — accepts a UUID or a status display name (e.g. 'Candidate', 'Accepted'). Omit to use the asset type's default status."`
	ExcludeFromAutoHyperlinking bool             `json:"excludeFromAutoHyperlinking,omitempty" jsonschema:"Optional. When true, Collibra will not auto-create hyperlinks from other assets to this one. Defaults to false."`
	Attributes                  []InputAttribute `json:"attributes,omitempty" jsonschema:"Optional. Attribute values to set on the new asset. Each entry references an attribute type by name (e.g. 'Definition') or by UUID, with the value to assign."`
	AllowDuplicate              bool             `json:"allowDuplicate,omitempty" jsonschema:"Optional. When false (the default) and an asset with the same name already exists in the resolved (assetType, domain), the call returns status=duplicate_found without writing. Set true to bypass the check and create anyway."`
}

// InputAttribute is one attribute slot the agent wants to set.
type InputAttribute struct {
	Name   string `json:"name,omitempty" jsonschema:"Attribute type display name (e.g. 'Definition'). Server resolves this to the attribute type UUID via the asset type's scoped assignment. Pass either name or typeId."`
	TypeID string `json:"typeId,omitempty" jsonschema:"Attribute type UUID. Pass either name or typeId. typeId wins when both are supplied."`
	Value  string `json:"value" jsonschema:"Required. The attribute value. RICH_TEXT attributes (e.g. 'Definition') accept Markdown — the server converts to HTML before writing so it renders correctly in the Collibra UI."`
}

// Output is the typed response.
type Output struct {
	Status           OutputStatus      `json:"status" jsonschema:"success when the asset was created; duplicate_found when a same-named asset exists and allowDuplicate is false; validation_error for unresolved inputs; error for downstream Collibra failures."`
	Message          string            `json:"message" jsonschema:"Human-readable summary, including suggestions when validation fails."`
	Asset            *AssetSummary     `json:"asset,omitempty" jsonschema:"The newly created asset, on success."`
	Duplicates       []DuplicateInfo   `json:"duplicates,omitempty" jsonschema:"Existing assets that would conflict, on duplicate_found."`
	AttributeResults []AttributeResult `json:"attributeResults,omitempty" jsonschema:"Per-attribute outcomes, in the same order as input.attributes."`
}

// AssetSummary is the post-create snapshot of the asset.
type AssetSummary struct {
	ID          string `json:"id" jsonschema:"UUID of the newly created asset."`
	Name        string `json:"name"`
	DisplayName string `json:"displayName,omitempty"`
	Type        string `json:"type"`
	Domain      string `json:"domain"`
	Status      string `json:"status,omitempty"`
}

// DuplicateInfo is one existing-asset reference returned in a
// duplicate_found response.
type DuplicateInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// AttributeResult is the outcome of one attribute write.
type AttributeResult struct {
	Name            string `json:"name,omitempty" jsonschema:"Resolved attribute type display name."`
	TypeID          string `json:"typeId" jsonschema:"Resolved attribute type UUID."`
	Status          string `json:"status" jsonschema:"'success' or 'error'."`
	Error           string `json:"error,omitempty" jsonschema:"Error message when status is 'error'."`
	WrittenValue    string `json:"writtenValue,omitempty" jsonschema:"The value actually submitted to Collibra. For RICH_TEXT attributes this is the HTML form (post Markdown→HTML conversion); for plain attributes it equals the input value."`
	ConvertedFromMd bool   `json:"convertedFromMd,omitempty" jsonschema:"True when the input value was converted from Markdown to HTML before submission."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name: "create_asset",
		Description: "Create a new Collibra asset of any type. " +
			"Inputs accept human-friendly identifiers: assetType resolves from UUID, publicId, or display name; domain from UUID or display name; status from UUID or status name; attributes by name or typeId. " +
			"Markdown in RICH_TEXT attribute values (e.g. 'Definition') is converted to HTML server-side so it renders correctly in Collibra. " +
			"When allowDuplicate is false (the default), an existing asset with the same name in the same (assetType, domain) returns status=duplicate_found without writing. " +
			"Validation errors return suggestion-rich messages so the agent can self-correct. " +
			"Calling prepare_create_asset first is optional — only needed when the agent wants to enumerate options or inspect a type's full attribute schema.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{DestructiveHint: chip.Ptr(true)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if strings.TrimSpace(input.Name) == "" {
			return Output{Status: StatusValidationError, Message: "name is required."}, nil
		}
		if strings.TrimSpace(input.AssetType) == "" {
			return Output{Status: StatusValidationError, Message: "assetType is required."}, nil
		}
		if strings.TrimSpace(input.Domain) == "" {
			return Output{Status: StatusValidationError, Message: "domain is required."}, nil
		}

		ec, out := buildExecutionContext(ctx, collibraClient, input)
		if out != nil {
			return *out, nil
		}

		// Duplicate gate runs before attribute resolution so the agent
		// doesn't pay for a schema lookup when it's about to be told to
		// stop and confirm.
		if !input.AllowDuplicate {
			if dup, err := findDuplicate(ctx, collibraClient, input.Name, ec.assetType.ID, ec.domain.ID); err == nil && dup != nil {
				return Output{
					Status:     StatusDuplicateFound,
					Message:    fmt.Sprintf("An asset named %q already exists in domain %q (id %s). Re-call with allowDuplicate=true to create anyway.", dup.Name, ec.domain.Name, dup.ID),
					Duplicates: []DuplicateInfo{{ID: dup.ID, Name: dup.Name}},
				}, nil
			}
		}

		writer := attrwrite.New(collibraClient)
		resolvedAttrs, attrOut := resolveAttributes(ctx, writer, input.Attributes, ec.assignment)
		if attrOut != nil {
			return *attrOut, nil
		}

		// Status resolution happens last in the pre-flight so the agent
		// gets validation errors for cheap inputs (asset type, domain,
		// attributes) without paying for /statuses on those failures.
		statusID, statusOut := resolveStatus(ctx, collibraClient, input.Status)
		if statusOut != nil {
			return *statusOut, nil
		}

		// Write phase.
		assetResp, err := clients.CreateAsset(ctx, collibraClient, clients.CreateAssetRequest{
			Name:                        input.Name,
			TypeID:                      ec.assetType.ID,
			DomainID:                    ec.domain.ID,
			DisplayName:                 input.DisplayName,
			StatusID:                    statusID,
			ExcludeFromAutoHyperlinking: input.ExcludeFromAutoHyperlinking,
		})
		if err != nil {
			return Output{Status: StatusError, Message: fmt.Sprintf("Could not create asset: %v", err)}, nil
		}

		attrResults := writeAttributes(ctx, collibraClient, assetResp.ID, resolvedAttrs)

		return Output{
			Status:           StatusSuccess,
			Message:          fmt.Sprintf("Created asset %q (id %s) in domain %q.", assetResp.Name, assetResp.ID, ec.domain.Name),
			Asset:            summariseAsset(assetResp),
			AttributeResults: attrResults,
		}, nil
	}
}

// executionContext carries the resolved (assetType, domain, scoped
// assignment) trio that every later step consults. Building it once
// up-front keeps the rest of the handler linear.
type executionContext struct {
	assetType  *clients.PrepareCreateAssetType
	domain     clients.PrepareCreateDomain
	assignment *clients.PrepareCreateScopedAssignment
}

func buildExecutionContext(ctx context.Context, client *http.Client, input Input) (*executionContext, *Output) {
	assetType, err := resolveAssetType(ctx, client, input.AssetType)
	if err != nil {
		out := assetTypeNotResolved(ctx, client, input.AssetType, err)
		return nil, &out
	}

	domain, err := resolveDomain(ctx, client, input.Domain)
	if err != nil {
		out := domainNotResolved(ctx, client, input.Domain, err, assetType)
		return nil, &out
	}
	if domain.Type == nil {
		full, err := clients.GetDomainByID(ctx, client, domain.ID)
		if err != nil {
			return nil, &Output{Status: StatusValidationError, Message: fmt.Sprintf("Could not resolve domain type for %q: %v", domain.Name, err)}
		}
		domain = *full
	}
	if domain.Type == nil {
		return nil, &Output{Status: StatusValidationError, Message: fmt.Sprintf("Domain %q has no domain type — cannot determine scoped assignment.", domain.Name)}
	}

	assignment, err := clients.GetScopedAssignment(ctx, client, assetType.ID, domain.Type.ID)
	if err != nil {
		// Disambiguate "wrong domain type for this asset type" from "asset
		// type has no domain types at all" — the second case happens with
		// subtypes that inherit assignments from a parent and the standard
		// "pick a different domain" hint is misleading.
		if allowed, allowedErr := clients.ListAllowedDomainTypesForAssetType(ctx, client, assetType.ID); allowedErr == nil && len(allowed) == 0 {
			return nil, &Output{
				Status:  StatusValidationError,
				Message: fmt.Sprintf("No compatible domains found for asset type %q on this instance.", assetType.Name),
			}
		}
		return nil, &Output{
			Status: StatusValidationError,
			Message: fmt.Sprintf("Asset type %q is not allowed in domain %q (domain type %q). Pick a different domain or a different asset type.",
				assetType.Name, domain.Name, domain.Type.Name),
		}
	}

	return &executionContext{assetType: assetType, domain: domain, assignment: assignment}, nil
}

// --- resolution helpers ---

// resolveAssetType tries UUID → publicId → exact case-insensitive display
// name match against /assetTypes?name=… . The first strategy that
// returns a result wins.
func resolveAssetType(ctx context.Context, client *http.Client, value string) (*clients.PrepareCreateAssetType, error) {
	v := strings.TrimSpace(value)
	if isUUID(v) {
		if at, err := clients.GetAssetTypeByID(ctx, client, v); err == nil {
			return at, nil
		}
	}
	if at, err := clients.GetAssetTypeByPublicID(ctx, client, v); err == nil {
		return at, nil
	}
	matches, _, err := clients.SearchAssetTypesByName(ctx, client, v, 50)
	if err != nil {
		return nil, fmt.Errorf("searching for asset type %q: %w", v, err)
	}
	exact := filterAssetTypesByExactName(matches, v)
	switch len(exact) {
	case 1:
		return &exact[0], nil
	case 0:
		return nil, fmt.Errorf("no asset type matches %q", v)
	default:
		return nil, fmt.Errorf("asset type %q is ambiguous: %d exact matches", v, len(exact))
	}
}

// resolveDomain tries UUID → exact case-insensitive name match.
func resolveDomain(ctx context.Context, client *http.Client, value string) (clients.PrepareCreateDomain, error) {
	v := strings.TrimSpace(value)
	if isUUID(v) {
		if d, err := clients.GetDomainByID(ctx, client, v); err == nil {
			return *d, nil
		}
	}
	matches, _, err := clients.SearchDomainsByName(ctx, client, v, 50)
	if err != nil {
		return clients.PrepareCreateDomain{}, fmt.Errorf("searching for domain %q: %w", v, err)
	}
	exact := filterDomainsByExactName(matches, v)
	switch len(exact) {
	case 1:
		return exact[0], nil
	case 0:
		return clients.PrepareCreateDomain{}, fmt.Errorf("no domain matches %q", v)
	default:
		return clients.PrepareCreateDomain{}, fmt.Errorf("domain %q is ambiguous: %d exact matches", v, len(exact))
	}
}

// resolveStatus is no-op when input is empty (Collibra applies the asset
// type's default). Otherwise tries UUID, then case-insensitive exact
// name against the full /statuses list.
func resolveStatus(ctx context.Context, client *http.Client, value string) (string, *Output) {
	v := strings.TrimSpace(value)
	if v == "" {
		return "", nil
	}
	if isUUID(v) {
		return v, nil
	}
	statuses, err := clients.ListStatusesAll(ctx, client)
	if err != nil {
		return "", &Output{Status: StatusValidationError, Message: fmt.Sprintf("Could not list statuses to resolve %q: %v", v, err)}
	}
	for _, s := range statuses {
		if strings.EqualFold(s.Name, v) {
			return s.ID, nil
		}
	}
	names := make([]string, 0, len(statuses))
	for _, s := range statuses {
		names = append(names, s.Name)
	}
	return "", &Output{
		Status:  StatusValidationError,
		Message: fmt.Sprintf("Status %q does not match any defined status. %s", v, suggestionSuffix("Statuses", names)),
	}
}

// resolveAttributes maps each input attribute to the matching slot in
// the scoped assignment, surfaces unknown attribute names as a single
// validation error, and runs each value through writer.PrepareValue so
// RICH_TEXT attributes get Markdown→HTML conversion. Returns the
// resolved list ready for writing.
func resolveAttributes(ctx context.Context, writer *attrwrite.Writer, in []InputAttribute, assignment *clients.PrepareCreateScopedAssignment) ([]resolvedAttribute, *Output) {
	if len(in) == 0 {
		return nil, nil
	}
	byID := indexAssignmentByID(assignment.Attributes)
	byName := indexAssignmentByName(assignment.Attributes)

	resolved := make([]resolvedAttribute, 0, len(in))
	for i, ra := range in {
		slot, err := matchAttributeSlot(ra, byID, byName)
		if err != nil {
			names := assignmentAttributeNames(assignment.Attributes)
			return nil, &Output{
				Status:  StatusValidationError,
				Message: fmt.Sprintf("attributes[%d]: %v. %s", i, err, suggestionSuffix("Attributes", names)),
			}
		}
		written, converted := writer.PrepareValue(ctx, slot.AttributeTypeID, slot.Kind, ra.Value)
		resolved = append(resolved, resolvedAttribute{
			Slot:                  slot,
			Value:                 written,
			ConvertedFromMarkdown: converted,
		})
	}
	return resolved, nil
}

// matchAttributeSlot picks the scoped attribute slot for an input
// attribute. Either `name` or `typeId` must be present; `typeId` wins
// when both are supplied so the agent can rely on a UUID round-trip.
func matchAttributeSlot(ra InputAttribute, byID map[string]clients.PrepareCreateScopedAttribute, byName map[string]clients.PrepareCreateScopedAttribute) (clients.PrepareCreateScopedAttribute, error) {
	if id := strings.TrimSpace(ra.TypeID); id != "" {
		if slot, ok := byID[id]; ok {
			return slot, nil
		}
		return clients.PrepareCreateScopedAttribute{}, fmt.Errorf("typeId %q is not a valid attribute for this asset type in this domain", id)
	}
	if name := strings.TrimSpace(ra.Name); name != "" {
		if slot, ok := byName[strings.ToLower(name)]; ok {
			return slot, nil
		}
		return clients.PrepareCreateScopedAttribute{}, fmt.Errorf("name %q is not a valid attribute for this asset type in this domain", name)
	}
	return clients.PrepareCreateScopedAttribute{}, fmt.Errorf("attribute requires either name or typeId")
}

// resolvedAttribute is the post-resolution representation of one input
// attribute: which slot it targets and what value to write.
type resolvedAttribute struct {
	Slot                  clients.PrepareCreateScopedAttribute
	Value                 string
	ConvertedFromMarkdown bool
}

// --- duplicate detection ---

// findDuplicate returns the first existing asset (if any) with the same
// name in the resolved (assetType, domain). Errors are swallowed because
// a transient duplicate-check failure shouldn't block creation; the
// caller proceeds and Collibra will reject a real duplicate at write
// time anyway.
func findDuplicate(ctx context.Context, client *http.Client, name, assetTypeID, domainID string) (*clients.PrepareCreateAssetResult, error) {
	dups, err := clients.SearchAssetsForDuplicate(ctx, client, name, assetTypeID, domainID)
	if err != nil {
		return nil, err
	}
	for i := range dups {
		if strings.EqualFold(dups[i].Name, name) {
			return &dups[i], nil
		}
	}
	return nil, nil
}

// --- write phase ---

// writeAttributes fires one POST /attributes per resolved attribute.
// Per-attribute errors are captured but do not abort the loop — partial
// success is the more useful UX given the asset is already created.
func writeAttributes(ctx context.Context, client *http.Client, assetID string, resolved []resolvedAttribute) []AttributeResult {
	if len(resolved) == 0 {
		return nil
	}
	results := make([]AttributeResult, len(resolved))
	for i, r := range resolved {
		results[i] = AttributeResult{
			Name:            r.Slot.AttributeTypeName,
			TypeID:          r.Slot.AttributeTypeID,
			WrittenValue:    r.Value,
			ConvertedFromMd: r.ConvertedFromMarkdown,
		}
		_, err := clients.CreateAttribute(ctx, client, clients.CreateAttributeRequest{
			AssetID: assetID,
			TypeID:  r.Slot.AttributeTypeID,
			Value:   r.Value,
		})
		if err != nil {
			results[i].Status = "error"
			results[i].Error = err.Error()
			continue
		}
		results[i].Status = "success"
	}
	return results
}

// --- not-resolved branches ---

// assetTypeNotResolved enriches the validation_error message with the
// available asset types and a license hint: when an asset type's
// backing module is not licensed, the type simply isn't present in
// /assetTypes, so the failure looks like a typo to the agent.
func assetTypeNotResolved(ctx context.Context, client *http.Client, raw string, err error) Output {
	types, _, listErr := clients.ListAssetTypesForPrepare(ctx, client, 200)
	msg := fmt.Sprintf("Asset type %q could not be resolved: %v.", raw, err)
	if listErr == nil {
		names := make([]string, 0, len(types))
		for _, at := range types {
			names = append(names, at.Name)
		}
		msg += " " + suggestionSuffix("Asset types", names)
	}
	msg += " If you expected this type, the relevant module may not be enabled on this instance."
	return Output{Status: StatusValidationError, Message: msg}
}

// domainNotResolved enriches the validation error with domain
// suggestions filtered to the asset type's allowed domain types — so
// the agent doesn't propose, say, a Process Register domain for a
// Business Term. Filter is best-effort: if the assignment fetch fails,
// we fall back to listing all domains rather than blocking the response.
func domainNotResolved(ctx context.Context, client *http.Client, raw string, err error, assetType *clients.PrepareCreateAssetType) Output {
	domains, _, listErr := clients.ListDomainsForPrepare(ctx, client, 500)
	msg := fmt.Sprintf("Domain %q could not be resolved: %v.", raw, err)
	if listErr != nil {
		return Output{Status: StatusValidationError, Message: msg}
	}

	suggestionLabel := "Domains"
	if assetType != nil {
		if allowed, allowedErr := clients.ListAllowedDomainTypesForAssetType(ctx, client, assetType.ID); allowedErr == nil {
			if len(allowed) == 0 {
				return Output{
					Status:  StatusValidationError,
					Message: fmt.Sprintf("No compatible domains found for asset type %q on this instance.", assetType.Name),
				}
			}
			domains = filterDomainsByAllowedTypes(domains, allowed)
			suggestionLabel = compatibleDomainsLabel(allowed)
		}
	}

	names := make([]string, 0, len(domains))
	for _, d := range domains {
		names = append(names, d.Name)
	}
	msg += " " + suggestionSuffix(suggestionLabel, names)
	return Output{Status: StatusValidationError, Message: msg}
}

// filterDomainsByAllowedTypes is the same predicate as in
// prepare_create_asset; duplicated here because the package boundary
// keeps each tool's helpers co-located. Cheap and stable.
func filterDomainsByAllowedTypes(domains []clients.PrepareCreateDomain, allowed []clients.PrepareCreateAllowedDomainType) []clients.PrepareCreateDomain {
	allowedIDs := make(map[string]struct{}, len(allowed))
	for _, a := range allowed {
		allowedIDs[a.ID] = struct{}{}
	}
	out := make([]clients.PrepareCreateDomain, 0, len(domains))
	for _, d := range domains {
		if d.Type == nil {
			continue
		}
		if _, ok := allowedIDs[d.Type.ID]; ok {
			out = append(out, d)
		}
	}
	return out
}

func compatibleDomainsLabel(allowed []clients.PrepareCreateAllowedDomainType) string {
	names := make([]string, 0, len(allowed))
	for _, a := range allowed {
		names = append(names, a.Name)
	}
	switch len(names) {
	case 0:
		return "Domains"
	case 1:
		return names[0] + " domains"
	case 2:
		return names[0] + " or " + names[1] + " domains"
	default:
		return strings.Join(names[:len(names)-1], ", ") + ", or " + names[len(names)-1] + " domains"
	}
}

// --- shape converters and small helpers ---

func summariseAsset(a *clients.CreateAssetResponse) *AssetSummary {
	if a == nil {
		return nil
	}
	s := &AssetSummary{
		ID:          a.ID,
		Name:        a.Name,
		DisplayName: a.DisplayName,
		Type:        a.Type.Name,
		Domain:      a.Domain.Name,
	}
	if a.Status != nil {
		s.Status = a.Status.Name
	}
	return s
}

func indexAssignmentByID(slots []clients.PrepareCreateScopedAttribute) map[string]clients.PrepareCreateScopedAttribute {
	m := make(map[string]clients.PrepareCreateScopedAttribute, len(slots))
	for _, s := range slots {
		m[s.AttributeTypeID] = s
	}
	return m
}

func indexAssignmentByName(slots []clients.PrepareCreateScopedAttribute) map[string]clients.PrepareCreateScopedAttribute {
	m := make(map[string]clients.PrepareCreateScopedAttribute, len(slots))
	for _, s := range slots {
		m[strings.ToLower(s.AttributeTypeName)] = s
	}
	return m
}

func assignmentAttributeNames(slots []clients.PrepareCreateScopedAttribute) []string {
	names := make([]string, 0, len(slots))
	for _, s := range slots {
		names = append(names, s.AttributeTypeName)
	}
	return names
}

func filterAssetTypesByExactName(matches []clients.PrepareCreateAssetType, name string) []clients.PrepareCreateAssetType {
	var out []clients.PrepareCreateAssetType
	for _, m := range matches {
		if strings.EqualFold(m.Name, name) {
			out = append(out, m)
		}
	}
	return out
}

func filterDomainsByExactName(matches []clients.PrepareCreateDomain, name string) []clients.PrepareCreateDomain {
	var out []clients.PrepareCreateDomain
	for _, m := range matches {
		if strings.EqualFold(m.Name, name) {
			out = append(out, m)
		}
	}
	return out
}

// suggestionSuffix renders a short list of valid names so the agent can
// self-correct in one round instead of round-tripping through prepare.
func suggestionSuffix(label string, names []string) string {
	if len(names) == 0 {
		return ""
	}
	sort.Strings(names)
	if len(names) <= maxOptionsInError {
		return fmt.Sprintf("%s available: %s.", label, strings.Join(names, ", "))
	}
	return fmt.Sprintf("%s available: %s (and %d more).", label, strings.Join(names[:maxOptionsInError], ", "), len(names)-maxOptionsInError)
}

func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, r := range s {
		switch i {
		case 8, 13, 18, 23:
			if r != '-' {
				return false
			}
		default:
			if !isHex(r) {
				return false
			}
		}
	}
	return true
}

func isHex(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
}
