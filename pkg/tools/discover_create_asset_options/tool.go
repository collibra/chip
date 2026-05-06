// Package discover_create_asset_options implements the
// discover_create_asset_options MCP tool — a read-only companion to
// create_asset. It lists available asset types and domains, resolves a
// name/publicId/UUID for either, and hydrates the scoped attribute and
// relation schema for a chosen (assetType, domain) pair so the agent
// knows what fields and statuses are available before composing the
// create. It does NOT perform duplicate detection or pre-flight
// validation — those live in create_asset itself, which is fully
// self-sufficient. Use this tool only when the agent needs to enumerate
// options or inspect a type's full schema.
package discover_create_asset_options

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// maxOptions caps the number of asset type / domain options returned in
// any one response — defensive against pathologically large instances.
// Set generously so the agent has enough context to recover from a typo
// or wrong selection without another round-trip; the happy-path
// enumeration helpers (enumerateDomainsFor, enumerateAssetTypesForDomain)
// also filter by compatibility before this cap applies, so the truncation
// bound is rarely binding in practice.
const maxOptions = 200

// Status is the discovery outcome.
type Status string

const (
	// StatusReady means an assetType + domain were both resolved and the
	// scoped assignment was hydrated successfully — the agent has
	// everything it needs to call create_asset.
	StatusReady Status = "ready"
	// StatusIncomplete means at least one of assetType / domain was
	// missing; the response includes pre-fetched options.
	StatusIncomplete Status = "incomplete"
	// StatusNeedsClarification means resolution failed (typo, multiple
	// matches, asset type not allowed in domain). Options for recovery
	// are included where useful.
	StatusNeedsClarification Status = "needs_clarification"
)

// Input is the tool's typed input. Either assetType or domain may be
// omitted to enumerate options.
type Input struct {
	AssetType         string `json:"assetType,omitempty" jsonschema:"Optional. Asset type identifier — accepts a UUID, the type's publicId (e.g. 'BusinessTerm'), or its display name (e.g. 'Business Term'). Resolution tries those strategies in order. Omit to enumerate available asset types."`
	Domain            string `json:"domain,omitempty" jsonschema:"Optional. Domain identifier — accepts a UUID or the domain's display name (case-insensitive). Omit to enumerate available domains."`
	IncludeStringType bool   `json:"includeStringType,omitempty" jsonschema:"Optional. When true, each attribute in attributeSchema is hydrated with its stringType (e.g. 'RICH_TEXT', 'PLAIN_TEXT') and description. Adds one /attributeTypes/{id} call per attribute, so omit unless the agent needs the extra detail."`
}

// Output is the structured discovery response.
type Output struct {
	Status            Status                 `json:"status" jsonschema:"ready when both inputs resolved and the scoped assignment was hydrated; incomplete when an input was missing; needs_clarification when an input could not be resolved or the type is not allowed in the domain."`
	Message           string                 `json:"message" jsonschema:"Human-readable summary of the outcome."`
	Resolved          *Resolved              `json:"resolved,omitempty" jsonschema:"Resolved IDs when both assetType and domain were provided and recognised. Pass these to create_asset, or use them as-is in the agent's context."`
	AssetTypeOptions  []AssetTypeOption      `json:"assetTypeOptions,omitempty" jsonschema:"Asset types available on this instance. Returned when assetType was omitted or could not be resolved. Truncated to a sensible cap; check optionsTruncated."`
	DomainOptions     []DomainOption         `json:"domainOptions,omitempty" jsonschema:"Domains available on this instance. Returned when domain was omitted or could not be resolved."`
	OptionsTruncated  bool                   `json:"optionsTruncated" jsonschema:"Whether assetTypeOptions or domainOptions was truncated below the instance's true total."`
	AttributeSchema   []AttributeSchemaEntry `json:"attributeSchema,omitempty" jsonschema:"Attribute slots in the scoped assignment for the resolved (assetType, domain) pair. Each entry tells the agent which attributes are required, what kind of value to supply, and (with includeStringType) whether the value is rich text."`
	RelationTypes     []RelationSchemaEntry  `json:"relationTypes,omitempty" jsonschema:"Relation slots in the scoped assignment — the relation roles available for assets of this type in this domain."`
	AvailableStatuses []StatusOption         `json:"availableStatuses,omitempty" jsonschema:"All statuses defined on this instance. Returned alongside ready/needs_clarification responses so the agent can pick a non-default initial status when calling create_asset."`
}

// Resolved holds the fully-resolved IDs for an assetType + domain pair.
type Resolved struct {
	AssetTypeID       string `json:"assetTypeId" jsonschema:"UUID of the resolved asset type."`
	AssetTypeName     string `json:"assetTypeName" jsonschema:"Display name of the resolved asset type."`
	AssetTypePublicID string `json:"assetTypePublicId,omitempty" jsonschema:"PublicId of the resolved asset type, when known."`
	DomainID          string `json:"domainId" jsonschema:"UUID of the resolved domain."`
	DomainName        string `json:"domainName" jsonschema:"Display name of the resolved domain."`
	DomainTypeID      string `json:"domainTypeId,omitempty" jsonschema:"UUID of the resolved domain's domain type — the key used to find the effective scoped assignment."`
	DomainTypeName    string `json:"domainTypeName,omitempty" jsonschema:"Display name of the resolved domain's domain type."`
}

// AssetTypeOption is one entry in assetTypeOptions.
type AssetTypeOption struct {
	ID       string `json:"id" jsonschema:"UUID of the asset type."`
	PublicID string `json:"publicId" jsonschema:"PublicId of the asset type."`
	Name     string `json:"name" jsonschema:"Display name of the asset type."`
}

// DomainOption is one entry in domainOptions.
type DomainOption struct {
	ID       string `json:"id" jsonschema:"UUID of the domain."`
	Name     string `json:"name" jsonschema:"Display name of the domain."`
	TypeName string `json:"typeName,omitempty" jsonschema:"Display name of the domain's domain type, when included by the API."`
}

// AttributeSchemaEntry is one attribute slot in the scoped assignment.
type AttributeSchemaEntry struct {
	AttributeTypeID       string   `json:"attributeTypeId" jsonschema:"UUID of the attribute type — pass this in attributes[].typeId to create_asset."`
	Name                  string   `json:"name" jsonschema:"Display name of the attribute (e.g. 'Definition', 'Note'). Also accepted by create_asset."`
	PublicID              string   `json:"publicId,omitempty" jsonschema:"PublicId of the attribute type."`
	Kind                  string   `json:"kind" jsonschema:"Attribute-type discriminator (e.g. 'StringAttributeType', 'NumericAttributeType')."`
	Required              bool     `json:"required" jsonschema:"True when the assignment's minimumOccurrences > 0. Note: Collibra doesn't always enforce this at create time — it can be an attestation/workflow signal rather than a hard create-time requirement. Agents may try a minimal create to discover what's actually enforced."`
	Min                   int      `json:"min" jsonschema:"Minimum number of occurrences."`
	Max                   *int     `json:"max,omitempty" jsonschema:"Maximum number of occurrences. Absent when unbounded."`
	StringType            string   `json:"stringType,omitempty" jsonschema:"For string-kind attributes: 'RICH_TEXT' means create_asset will run the value through Markdown→HTML conversion. Only populated when input.includeStringType is true."`
	Description           string   `json:"description,omitempty" jsonschema:"Server-defined description of the attribute. Only populated when input.includeStringType is true."`
	AllowedValues         []string `json:"allowedValues,omitempty" jsonschema:"Permitted values for list-type attributes. Only populated when input.includeStringType is true."`
}

// RelationSchemaEntry is one relation slot in the scoped assignment.
type RelationSchemaEntry struct {
	RelationTypeID string `json:"relationTypeId" jsonschema:"UUID of the relation type."`
	Role           string `json:"role" jsonschema:"Forward role name of the relation (e.g. 'is synonym of')."`
	CoRole         string `json:"coRole,omitempty" jsonschema:"Reverse role name."`
	Direction      string `json:"direction,omitempty" jsonschema:"Direction of the relation as defined in the assignment."`
	TargetTypeID   string `json:"targetTypeId,omitempty" jsonschema:"UUID of the asset type on the target side of the relation."`
	TargetTypeName string `json:"targetTypeName,omitempty" jsonschema:"Display name of the asset type on the target side of the relation."`
}

// StatusOption is one entry in availableStatuses.
type StatusOption struct {
	ID   string `json:"id" jsonschema:"UUID of the status."`
	Name string `json:"name" jsonschema:"Display name of the status."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name: "discover_create_asset_options",
		Description: "Read-only companion to create_asset. Enumerates available asset types and domains, resolves a UUID/publicId/displayName for either, " +
			"and hydrates the scoped attribute and relation schema for a given (assetType, domain) pair so the agent knows what attributes and relations are available. " +
			"Optional: pass includeStringType=true to also populate each attribute's stringType (e.g. 'RICH_TEXT') and description. " +
			"Calling this before create_asset is optional — create_asset performs its own resolution, validation, and duplicate-check. " +
			"Use this tool when the agent needs to browse what's creatable on this instance or inspect an asset type's full schema before composing a create.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		// Step 1: handle the missing-input cases up-front. The agent gets
		// pre-fetched lists so it can re-prompt the user without another
		// round-trip. When the agent gives us a domain but no asset type,
		// resolve the domain first and narrow the asset-type list to
		// those actually allowed in it — otherwise we'd return the global
		// alphabetical list, which makes "what can I create in this
		// domain?" return a meaningless answer.
		if strings.TrimSpace(input.AssetType) == "" {
			if strings.TrimSpace(input.Domain) != "" {
				domain, err := resolveDomain(ctx, collibraClient, input.Domain)
				if err != nil {
					return domainNotResolved(ctx, collibraClient, input.Domain, err)
				}
				return enumerateAssetTypesForDomain(ctx, collibraClient, domain)
			}
			return enumerateAssetTypes(ctx, collibraClient)
		}

		// Step 2: resolve the asset type.
		assetType, atErr := resolveAssetType(ctx, collibraClient, input.AssetType)
		if atErr != nil {
			return assetTypeNotResolved(ctx, collibraClient, input.AssetType, atErr)
		}

		if strings.TrimSpace(input.Domain) == "" {
			return enumerateDomainsFor(ctx, collibraClient, assetType)
		}

		// Step 3: resolve the domain (carries the domain type, needed for
		// the scoped-assignment lookup).
		domain, domErr := resolveDomain(ctx, collibraClient, input.Domain)
		if domErr != nil {
			return domainNotResolved(ctx, collibraClient, input.Domain, domErr)
		}
		if domain.Type == nil {
			full, err := getDomainDetailsByID(ctx, collibraClient, domain.ID)
			if err != nil {
				return Output{Status: StatusNeedsClarification, Message: fmt.Sprintf("Could not fetch full details for domain %q: %v", domain.Name, err)}, nil
			}
			domain = *full
		}

		// Step 4: hydrate the scoped assignment.
		assignment, err := clients.GetScopedAssignment(ctx, collibraClient, assetType.ID, domain.Type.ID)
		if err != nil {
			return Output{
				Status: StatusNeedsClarification,
				Message: fmt.Sprintf("Asset type %q is not allowed in domain %q (domain type %q). Pick a different domain or a different asset type.",
					assetType.Name, domain.Name, domain.Type.Name),
			}, nil
		}

		// Step 5: assemble the response.
		out := Output{
			Status: StatusReady,
			Message: fmt.Sprintf("Resolved asset type %q in domain %q. %d attribute slot(s), %d relation slot(s) in scope.",
				assetType.Name, domain.Name, len(assignment.Attributes), len(assignment.Relations)),
			Resolved: &Resolved{
				AssetTypeID:       assetType.ID,
				AssetTypeName:     assetType.Name,
				AssetTypePublicID: assetType.PublicID,
				DomainID:          domain.ID,
				DomainName:        domain.Name,
				DomainTypeID:      domain.Type.ID,
				DomainTypeName:    domain.Type.Name,
			},
			AttributeSchema: schemaEntriesFromAssignment(assignment.Attributes),
			RelationTypes:   relationEntriesFromAssignment(assignment.Relations),
		}

		if input.IncludeStringType {
			if err := hydrateAttributeDetails(ctx, collibraClient, out.AttributeSchema); err != nil {
				out.Message += " (attribute details could not be fully hydrated: " + err.Error() + ")"
			}
		}

		statuses, _ := clients.ListStatusesAll(ctx, collibraClient)
		out.AvailableStatuses = toStatusOptions(statuses)

		return out, nil
	}
}

// --- enumeration helpers (used when an input is missing) ---

func enumerateAssetTypes(ctx context.Context, client *http.Client) (Output, error) {
	types, total, err := clients.ListAssetTypesForPrepare(ctx, client, maxOptions+1)
	if err != nil {
		return Output{}, err
	}
	truncated := total > maxOptions
	if len(types) > maxOptions {
		types = types[:maxOptions]
	}
	return Output{
		Status:           StatusIncomplete,
		Message:          "assetType is required. Pick one from assetTypeOptions and call again.",
		AssetTypeOptions: toAssetTypeOptions(types),
		OptionsTruncated: truncated,
	}, nil
}

// enumerateAssetTypesForDomain narrows the asset-type list to those
// permitted in the resolved domain — the symmetric companion of
// enumerateDomainsFor. /assignments/domain/{id}/assetTypes is
// authoritative for this; calling it once is cheaper than the agent
// guessing from the global list and getting "type not allowed in
// domain" on the next call.
func enumerateAssetTypesForDomain(ctx context.Context, client *http.Client, domain clients.PrepareCreateDomain) (Output, error) {
	allowed, err := clients.GetAvailableAssetTypesForDomain(ctx, client, domain.ID)
	if err != nil {
		return Output{}, fmt.Errorf("listing asset types for domain %q: %w", domain.Name, err)
	}
	truncated := len(allowed) > maxOptions
	if truncated {
		allowed = allowed[:maxOptions]
	}
	domainTypeName := ""
	if domain.Type != nil {
		domainTypeName = domain.Type.Name
	}
	msg := fmt.Sprintf("assetType is required. Asset types creatable in domain %q", domain.Name)
	if domainTypeName != "" {
		msg += fmt.Sprintf(" (a %q domain)", domainTypeName)
	}
	msg += " are listed in assetTypeOptions."
	return Output{
		Status:           StatusIncomplete,
		Message:          msg,
		AssetTypeOptions: toAssetTypeOptions(allowed),
		OptionsTruncated: truncated,
	}, nil
}

// enumerateDomainsFor returns the domain options the agent can pick
// from when the asset type is known. The list is filtered to domains
// whose domain type matches one of the asset type's allowed domain
// types (so e.g. asking for "where can I create a Business Term" only
// returns Glossary-type domains). One extra /assignments/assetType/{id}
// call buys this; the alternative — returning every domain on the
// instance — produced misleading suggestions and clipped the useful
// ones when the 20-item cap was hit.
func enumerateDomainsFor(ctx context.Context, client *http.Client, assetType *clients.PrepareCreateAssetType) (Output, error) {
	allowed, allowedErr := clients.ListAllowedDomainTypesForAssetType(ctx, client, assetType.ID)
	domains, _, err := clients.ListDomainsForPrepare(ctx, client, 500)
	if err != nil {
		return Output{}, err
	}

	// Three cases by allowed-types lookup:
	//   - lookup errored        : fall back to unfiltered ("don't know, here's everything")
	//   - lookup ok, non-empty  : filter to compatible domains
	//   - lookup ok, empty      : asset type has no direct domain type assignments
	//                             on this instance. Returning the global list would
	//                             be misleading; report the fact and let the agent
	//                             decide what to tell the user.
	switch {
	case allowedErr == nil && len(allowed) == 0:
		return Output{
			Status:  StatusNeedsClarification,
			Message: noCompatibleDomainsMessage(assetType.Name),
		}, nil
	}

	candidates := domains
	filtered := false
	if allowedErr == nil {
		candidates = filterDomainsByAllowedTypes(domains, allowed)
		filtered = true
	}
	truncated := len(candidates) > maxOptions
	if truncated {
		candidates = candidates[:maxOptions]
	}

	msg := fmt.Sprintf("domain is required for asset type %q. Pick one from domainOptions and call again.", assetType.Name)
	if filtered {
		msg += fmt.Sprintf(" Filtered to %s.", joinDomainTypeNames(allowed))
	}

	return Output{
		Status:           StatusIncomplete,
		Message:          msg,
		DomainOptions:    toDomainOptions(candidates),
		OptionsTruncated: truncated,
	}, nil
}

// noCompatibleDomainsMessage is shared by every code path that detects an
// asset type with zero direct domain type assignments. Plain factual text —
// no theorising about why (e.g. subtype inheritance), since chip can't
// actually know that. The agent has the surrounding context to interpret.
func noCompatibleDomainsMessage(assetTypeName string) string {
	return fmt.Sprintf("No compatible domains found for asset type %q on this instance.", assetTypeName)
}

// filterDomainsByAllowedTypes keeps only the domains whose Type.ID
// appears in allowed. Domains without a Type field are dropped — they
// can't be safely matched.
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

// joinDomainTypeNames produces "Glossary domains" / "Glossary or Code
// Repository domains" for the message suffix.
func joinDomainTypeNames(allowed []clients.PrepareCreateAllowedDomainType) string {
	names := make([]string, 0, len(allowed))
	for _, a := range allowed {
		names = append(names, a.Name)
	}
	switch len(names) {
	case 0:
		return "compatible domains"
	case 1:
		return names[0] + " domains"
	case 2:
		return names[0] + " or " + names[1] + " domains"
	default:
		return strings.Join(names[:len(names)-1], ", ") + ", or " + names[len(names)-1] + " domains"
	}
}

// --- resolution helpers ---

// resolveAssetType tries UUID, publicId, then case-insensitive exact name match
// against /assetTypes?name=…. Returns the first strategy that succeeds.
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
	exact := filterExactNameAssetTypes(matches, v)
	switch len(exact) {
	case 1:
		return &exact[0], nil
	case 0:
		return nil, fmt.Errorf("no asset type matches %q", v)
	default:
		return nil, fmt.Errorf("asset type %q is ambiguous: %d exact matches", v, len(exact))
	}
}

// resolveDomain tries UUID, then case-insensitive exact name match.
func resolveDomain(ctx context.Context, client *http.Client, value string) (clients.PrepareCreateDomain, error) {
	v := strings.TrimSpace(value)
	if isUUID(v) {
		if d, err := getDomainDetailsByID(ctx, client, v); err == nil {
			return *d, nil
		}
	}
	matches, _, err := clients.SearchDomainsByName(ctx, client, v, 50)
	if err != nil {
		return clients.PrepareCreateDomain{}, fmt.Errorf("searching for domain %q: %w", v, err)
	}
	exact := filterExactNameDomains(matches, v)
	switch len(exact) {
	case 1:
		return exact[0], nil
	case 0:
		return clients.PrepareCreateDomain{}, fmt.Errorf("no domain matches %q", v)
	default:
		return clients.PrepareCreateDomain{}, fmt.Errorf("domain %q is ambiguous: %d exact matches", v, len(exact))
	}
}

// getDomainDetailsByID wraps GetDomainByID with a fallback to populate
// the Type field. The /domains/{id} endpoint already returns it; if a
// future version drops the field we'd want a parallel GET, hence the
// indirection.
func getDomainDetailsByID(ctx context.Context, client *http.Client, id string) (*clients.PrepareCreateDomain, error) {
	d, err := clients.GetDomainByID(ctx, client, id)
	if err != nil {
		return nil, err
	}
	return d, nil
}

// --- not-resolved branches: surface options + a license hint when relevant ---

func assetTypeNotResolved(ctx context.Context, client *http.Client, raw string, resolveErr error) (Output, error) {
	types, total, err := clients.ListAssetTypesForPrepare(ctx, client, maxOptions+1)
	if err != nil {
		return Output{Status: StatusNeedsClarification, Message: resolveErr.Error()}, nil
	}
	truncated := total > maxOptions
	if len(types) > maxOptions {
		types = types[:maxOptions]
	}
	return Output{
		Status:           StatusNeedsClarification,
		Message:          notResolvedMessage("Asset type", raw, resolveErr) + " If you expected this type, the relevant module may not be enabled on this instance.",
		AssetTypeOptions: toAssetTypeOptions(types),
		OptionsTruncated: truncated,
	}, nil
}

func domainNotResolved(ctx context.Context, client *http.Client, raw string, resolveErr error) (Output, error) {
	domains, total, err := clients.ListDomainsForPrepare(ctx, client, maxOptions+1)
	if err != nil {
		return Output{Status: StatusNeedsClarification, Message: resolveErr.Error()}, nil
	}
	truncated := total > maxOptions
	if len(domains) > maxOptions {
		domains = domains[:maxOptions]
	}
	return Output{
		Status:           StatusNeedsClarification,
		Message:          notResolvedMessage("Domain", raw, resolveErr),
		DomainOptions:    toDomainOptions(domains),
		OptionsTruncated: truncated,
	}, nil
}

func notResolvedMessage(label, raw string, err error) string {
	return fmt.Sprintf("%s %q could not be resolved: %v.", label, raw, err)
}

// --- detail hydration (per-attribute fetch when requested) ---

// hydrateAttributeDetails fans out one /attributeTypes/{id} call per
// attribute slot to pull stringType, description, and allowedValues.
// Errors on individual fetches are tolerated so a single missing detail
// doesn't blank the whole schema.
func hydrateAttributeDetails(ctx context.Context, client *http.Client, schema []AttributeSchemaEntry) error {
	var firstErr error
	for i := range schema {
		details, err := clients.GetAttributeTypeFull(ctx, client, schema[i].AttributeTypeID)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		schema[i].StringType = details.StringType
		schema[i].Description = details.Description
		if len(details.AllowedValues) > 0 {
			schema[i].AllowedValues = details.AllowedValues
		}
		if schema[i].PublicID == "" {
			schema[i].PublicID = details.PublicID
		}
	}
	return firstErr
}

// --- shape converters ---

func toAssetTypeOptions(in []clients.PrepareCreateAssetType) []AssetTypeOption {
	out := make([]AssetTypeOption, len(in))
	for i, at := range in {
		out[i] = AssetTypeOption{ID: at.ID, PublicID: at.PublicID, Name: at.Name}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

func toDomainOptions(in []clients.PrepareCreateDomain) []DomainOption {
	out := make([]DomainOption, len(in))
	for i, d := range in {
		opt := DomainOption{ID: d.ID, Name: d.Name}
		if d.Type != nil {
			opt.TypeName = d.Type.Name
		}
		out[i] = opt
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

func toStatusOptions(in []clients.PrepareCreateStatus) []StatusOption {
	out := make([]StatusOption, len(in))
	for i, s := range in {
		out[i] = StatusOption{ID: s.ID, Name: s.Name}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

func schemaEntriesFromAssignment(in []clients.PrepareCreateScopedAttribute) []AttributeSchemaEntry {
	out := make([]AttributeSchemaEntry, len(in))
	for i, a := range in {
		out[i] = AttributeSchemaEntry{
			AttributeTypeID:   a.AttributeTypeID,
			Name:              a.AttributeTypeName,
			PublicID:          a.AttributeTypePublicID,
			Kind:              a.Kind,
			Required:          a.Required,
			Min:               a.Min,
			Max:               a.Max,
		}
	}
	return out
}

func relationEntriesFromAssignment(in []clients.PrepareCreateScopedRelation) []RelationSchemaEntry {
	out := make([]RelationSchemaEntry, len(in))
	for i, r := range in {
		entry := RelationSchemaEntry{
			RelationTypeID: r.RelationTypeID,
			Role:           r.Role,
			CoRole:         r.CoRole,
			Direction:      r.Direction,
		}
		if r.TargetType != nil {
			entry.TargetTypeID = r.TargetType.ID
			entry.TargetTypeName = r.TargetType.Name
		}
		out[i] = entry
	}
	return out
}

// --- small predicates ---

func filterExactNameAssetTypes(matches []clients.PrepareCreateAssetType, name string) []clients.PrepareCreateAssetType {
	var out []clients.PrepareCreateAssetType
	for _, m := range matches {
		if strings.EqualFold(m.Name, name) {
			out = append(out, m)
		}
	}
	return out
}

func filterExactNameDomains(matches []clients.PrepareCreateDomain, name string) []clients.PrepareCreateDomain {
	var out []clients.PrepareCreateDomain
	for _, m := range matches {
		if strings.EqualFold(m.Name, name) {
			out = append(out, m)
		}
	}
	return out
}

// isUUID reports whether s looks like a Collibra UUID (8-4-4-4-12 hex).
// Lenient on case but strict on shape so we don't fan out a name search
// for what's clearly an ID typo.
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
