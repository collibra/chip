// Package create_asset_access_request creates a Collibra Data Access request for a catalog
// asset. The WHAT of the request is the Data Access role linked to that asset, not a data
// object. Whether an asset can be requested is never inferred from its type: the roles linked
// to it are read from Data Access. A Data Product with no requestable role of its own is
// expanded to its output ports, because that is where its role normally lives.
package create_asset_access_request

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/validation"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// aiDescriptionSuffix is appended to every description so the access request is clearly
// attributed to an AI agent.
const aiDescriptionSuffix = "This access request was created by AI."

// suggestedNameMaxLen caps the length of a name suggestion derived from the purpose.
const suggestedNameMaxLen = 80

// dateLayout is the date-only form accepted for expiresAt, in addition to RFC 3339.
const dateLayout = "2006-01-02"

// Status values returned in the Output.
const (
	statusNoRoleLinked          = "no_role_linked"
	statusNeedsPortSelection    = "needs_port_selection"
	statusNeedsNameConfirmation = "needs_name_confirmation"
	statusCreated               = "created"
)

type Input struct {
	AssetID      string   `json:"assetId" jsonschema:"Required. UUID of the Collibra asset access is requested on. Any asset that has a Data Access role linked to it can be used; a Data Product with no role of its own is expanded to its output ports. An asset without a requestable role comes back as status no_role_linked."`
	OutputPortID string   `json:"outputPortId,omitempty" jsonschema:"Optional. UUID of the output port to request access to. Only applies when assetId is a Data Product — supply it after the user picked one from the outputPorts returned with status needs_port_selection, or to request a specific port of a product that carries a role of its own."`
	Users        []string `json:"users,omitempty" jsonschema:"The Collibra users who need the access (part of the WHO). Each entry is an email address (preferred) or a Collibra username; both are mapped to Data Access users by email address. Supply users, groups, or both — at least one beneficiary is required, and the request is not created unless every entry maps."`
	Groups       []string `json:"groups,omitempty" jsonschema:"The Collibra groups who need the access (part of the WHO). Each entry is a group name (preferred) or a Collibra group UUID; both are mapped to Data Access groups by name. Supply users, groups, or both — at least one beneficiary is required, and the request is not created unless every entry maps."`
	Purpose      string   `json:"purpose" jsonschema:"Required. The user-supplied purpose / business justification for the access request. Used verbatim as the description. The tool always appends a note indicating the request was created by AI."`
	ExpiresAt    string   `json:"expiresAt" jsonschema:"Required. The date the access must expire, as a date (2026-12-31, interpreted as the end of that day UTC) or an RFC 3339 timestamp (2026-12-31T17:00:00Z). Must be in the future. Ask the user — never invent an expiration date."`
	Name         string   `json:"name,omitempty" jsonschema:"Optional. Display name of the access request. If omitted, the tool returns a suggested name derived from the purpose and asks the agent to confirm it with the user before retrying."`
}

type Output struct {
	Status           string                            `json:"status,omitempty" jsonschema:"Outcome of the call: no_role_linked (the asset has no Data Access role that can be requested — see linkedRoles), needs_port_selection (the Data Product has several output ports — ask the user which one and call again with outputPortId), needs_name_confirmation (confirm the suggestedName with the user and call again with name set), or created (the request was successfully created)."`
	Message          string                            `json:"message,omitempty" jsonschema:"Human-readable explanation of the status, including what the agent should do next."`
	LinkedRoles      []clients.CatalogAssetRole        `json:"linkedRoles,omitempty" jsonschema:"The access controls linked to the asset. Present when status is no_role_linked and the asset has some, none of which is an active Grant; empty means nothing is linked to it at all. Use it to tell the user why the asset cannot be requested."`
	OutputPorts      []clients.DataProductOutputPort   `json:"outputPorts,omitempty" jsonschema:"The output ports of the Data Product. Present when status is needs_port_selection — present these to the user and let them pick one."`
	SuggestedName    string                            `json:"suggestedName,omitempty" jsonschema:"Name suggestion derived from the purpose. Present only when status is needs_name_confirmation."`
	Asset            *clients.CatalogAssetRef          `json:"asset,omitempty" jsonschema:"The asset the request is raised on, once resolved — the output port when a Data Product was supplied."`
	Role             *clients.CatalogAssetRole         `json:"role,omitempty" jsonschema:"The Data Access role linked to the asset — the WHAT of the request."`
	Users            []*clients.DataAccessIdentity     `json:"users,omitempty" jsonschema:"The Data Access users the supplied Collibra users were mapped to — part of the WHO of the request."`
	Groups           []*clients.DataAccessGroup        `json:"groups,omitempty" jsonschema:"The Data Access groups the supplied Collibra groups were mapped to — part of the WHO of the request."`
	UnresolvedUsers  []clients.UnresolvedBeneficiary   `json:"unresolvedUsers,omitempty" jsonschema:"Supplied users that could not be mapped to a Data Access user. Nothing is created while this is non-empty — ask the user to correct or drop them."`
	UnresolvedGroups []clients.UnresolvedBeneficiary   `json:"unresolvedGroups,omitempty" jsonschema:"Supplied groups that could not be mapped to a Data Access group. Nothing is created while this is non-empty — ask the user to correct or drop them."`
	ExpiresAt        string                            `json:"expiresAt,omitempty" jsonschema:"The expiration date sent to Data Access, in RFC 3339."`
	Request          *clients.DataAccessRequestSummary `json:"request,omitempty" jsonschema:"The created access request, if successful."`
	Error            string                            `json:"error,omitempty" jsonschema:"Error message if the access request could not be created."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "create_asset_access_request",
		Title: "Create Asset Access Request",
		Description: "Create a Collibra Data Access request for a catalog asset. Supply `assetId`, the Collibra users and/or groups who need access, a user-supplied purpose, and a mandatory expiration date. " +
			"The WHAT of the request is the Data Access role linked to the asset, which the tool resolves, so never pass a role or data object yourself. " +
			"Any asset that has an active Grant linked to it can be requested; when none is linked the tool returns status no_role_linked with whatever is linked in `linkedRoles`, and nothing is created. " +
			"A Data Product with no requestable role of its own is expanded to its output ports, and with several ports returns them with status needs_port_selection — ask the user which one and call again with `outputPortId`. Naming a port that way always takes precedence over the product itself. " +
			"If no name is supplied, the tool returns a suggested name with status needs_name_confirmation.",
		Handler:     handle(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: new(false), OpenWorldHint: new(false)},
	}
}

func handle(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if err := validation.UUID("assetId", input.AssetID); err != nil {
			return Output{}, err
		}
		if err := validation.UUIDOptional("outputPortId", input.OutputPortID); err != nil {
			return Output{}, err
		}
		purpose := strings.TrimSpace(input.Purpose)
		if purpose == "" {
			return Output{Error: "purpose is required — ask the user for the business justification for this access request"}, nil
		}
		if len(input.Users) == 0 && len(input.Groups) == 0 {
			return Output{Error: "at least one beneficiary is required — supply the users (by email address or Collibra username) and/or the groups (by name) that need the access"}, nil
		}
		expiresAt, err := parseExpiresAt(input.ExpiresAt)
		if err != nil {
			return Output{Error: err.Error()}, nil
		}

		asset, role, statusOutput, err := resolveRequestAsset(ctx, collibraClient, input)
		if err != nil {
			return Output{Error: err.Error()}, nil
		}
		if statusOutput != nil {
			return *statusOutput, nil
		}

		users, unresolvedUsers, err := clients.ResolveDataAccessUsers(ctx, collibraClient, input.Users)
		if err != nil {
			return Output{Asset: asset, Role: role, Error: fmt.Sprintf("Failed to resolve the requested users: %s", err.Error())}, nil
		}
		groups, unresolvedGroups, err := clients.ResolveDataAccessGroups(ctx, collibraClient, input.Groups)
		if err != nil {
			return Output{Asset: asset, Role: role, Users: users, Error: fmt.Sprintf("Failed to resolve the requested groups: %s", err.Error())}, nil
		}
		if len(unresolvedUsers) > 0 || len(unresolvedGroups) > 0 {
			return Output{
				Asset:            asset,
				Role:             role,
				Users:            users,
				Groups:           groups,
				UnresolvedUsers:  unresolvedUsers,
				UnresolvedGroups: unresolvedGroups,
				Error:            "Some beneficiaries could not be mapped into Data Access. Nothing was created. Ask the user to correct or drop them, then call again.",
			}, nil
		}
		if len(users) == 0 && len(groups) == 0 {
			return Output{Asset: asset, Role: role, Error: "none of the supplied beneficiaries resolved into Data Access"}, nil
		}

		name := strings.TrimSpace(input.Name)
		if name == "" {
			suggested := suggestNameFromPurpose(purpose)
			return Output{
				Status:        statusNeedsNameConfirmation,
				Asset:         asset,
				Role:          role,
				Users:         users,
				Groups:        groups,
				SuggestedName: suggested,
				Message: fmt.Sprintf("No name was supplied. Suggested name based on the purpose: %q. Confirm this with the user (or ask for a different name), then call create_asset_access_request again with the confirmed name in the `name` field.",
					suggested),
			}, nil
		}

		return createRequest(ctx, collibraClient, createParams{
			name:      name,
			purpose:   purpose,
			asset:     asset,
			role:      role,
			users:     users,
			groups:    groups,
			expiresAt: expiresAt,
		}), nil
	}
}

// resolveRequestAsset determines which asset the request is raised on and which role it goes
// through. It returns either both, or an Output carrying a status the caller must act on: the
// asset has no requestable role, or a Data Product's output port still has to be chosen.
//
// Whether an asset can carry a role is never inferred from its type — the roles linked to it
// are read from Data Access. A Data Product with no requestable role of its own is expanded
// to its output ports, since that is where its role normally lives.
func resolveRequestAsset(ctx context.Context, collibraClient *http.Client, input Input) (*clients.CatalogAssetRef, *clients.CatalogAssetRole, *Output, error) {
	asset, err := clients.GetCatalogAsset(ctx, collibraClient, input.AssetID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to read asset %s: %s", input.AssetID, err.Error())
	}
	if asset == nil {
		return nil, nil, nil, fmt.Errorf("no asset found with id %s — check the id and try again", input.AssetID)
	}

	resolved, role, statusOutput, err := selectRequestAsset(ctx, collibraClient, input, asset)
	if err != nil || statusOutput != nil {
		return nil, nil, statusOutput, err
	}
	if role != nil {
		return resolved, role, nil, nil
	}

	role, linked, err := requestableRole(ctx, collibraClient, resolved)
	if err != nil {
		return nil, nil, nil, err
	}
	if role == nil {
		return nil, nil, noRoleLinkedOutput(resolved, linked), nil
	}
	return resolved, role, nil, nil
}

// requestableRole returns the role an access request can be raised through — an active Grant
// linked to the asset — together with every role linked to it, so the caller can explain why
// none of them is usable.
func requestableRole(ctx context.Context, collibraClient *http.Client, asset *clients.CatalogAssetRef) (*clients.CatalogAssetRole, []clients.CatalogAssetRole, error) {
	linked, err := clients.ListAssetRoles(ctx, collibraClient, asset.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read the Data Access roles linked to %s %q: %s", asset.TypeName, asset.Name, err.Error())
	}
	for i := range linked {
		if strings.EqualFold(linked[i].Action, "Grant") && strings.EqualFold(linked[i].State, "Active") {
			return &linked[i], linked, nil
		}
	}
	return nil, linked, nil
}

// noRoleLinkedOutput reports an asset that cannot be requested as a status rather than an
// error, carrying whatever is linked to it so the caller can say why.
func noRoleLinkedOutput(asset *clients.CatalogAssetRef, linked []clients.CatalogAssetRole) *Output {
	message := fmt.Sprintf("%s %q has no Data Access role linked to it, so access cannot be requested on it. Ask the user for an asset that has one, or ask an administrator to link a role to this asset.",
		asset.TypeName, asset.Name)
	if len(linked) > 0 {
		message = fmt.Sprintf("%s %q has access controls linked to it, but none of them is an active Grant, so access cannot be requested on it: %s.",
			asset.TypeName, asset.Name, describeRoles(linked))
	}

	return &Output{
		Status:      statusNoRoleLinked,
		Asset:       asset,
		LinkedRoles: linked,
		Message:     message,
	}
}

// describeRoles renders the linked access controls with the action and state that make them
// unusable.
func describeRoles(linked []clients.CatalogAssetRole) string {
	labels := make([]string, 0, len(linked))
	for _, r := range linked {
		labels = append(labels, fmt.Sprintf("%q (%s, %s)", r.Name, r.Action, r.State))
	}
	return strings.Join(labels, ", ")
}

// selectRequestAsset chooses the asset the role is read from: the supplied one, or — for a
// Data Product with no requestable role of its own — one of its output ports. Naming a port
// explicitly always wins, so a caller can request one even where the product itself carries a
// role. The role is returned when selecting the asset already established it, so it is not
// looked up twice.
func selectRequestAsset(ctx context.Context, collibraClient *http.Client, input Input, asset *clients.CatalogAssetRef) (*clients.CatalogAssetRef, *clients.CatalogAssetRole, *Output, error) {
	if asset.TypeID != clients.DataProductAssetTypeID {
		if input.OutputPortID != "" && !strings.EqualFold(input.OutputPortID, asset.ID) {
			return nil, nil, nil, fmt.Errorf("outputPortId only applies when assetId is a Data Product, and asset %q is a %s — drop outputPortId, or pass the Data Product as assetId",
				asset.Name, asset.TypeName)
		}
		return asset, nil, nil, nil
	}

	if input.OutputPortID == "" {
		role, _, err := requestableRole(ctx, collibraClient, asset)
		if err != nil {
			return nil, nil, nil, err
		}
		if role != nil {
			return asset, role, nil, nil
		}
	}

	port, statusOutput, err := resolveOutputPortOfDataProduct(ctx, collibraClient, asset, input.OutputPortID)
	return port, nil, statusOutput, err
}

// resolveOutputPortOfDataProduct picks the output port of a Data Product: the one the user
// chose, the only one there is, or none — in which case the candidates are returned for the
// user to choose from.
func resolveOutputPortOfDataProduct(ctx context.Context, collibraClient *http.Client, dataProduct *clients.CatalogAssetRef, chosenPortID string) (*clients.CatalogAssetRef, *Output, error) {
	ports, err := clients.ListDataProductOutputPorts(ctx, collibraClient, dataProduct.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list the output ports of data product %q: %s", dataProduct.Name, err.Error())
	}
	if len(ports) == 0 {
		return nil, nil, fmt.Errorf("data product %q has no output ports, so there is nothing to request access to", dataProduct.Name)
	}

	if chosenPortID != "" {
		if !containsPort(ports, chosenPortID) {
			return nil, nil, fmt.Errorf("output port %s is not an output port of data product %q — pick one of its own ports",
				chosenPortID, dataProduct.Name)
		}
		return loadPort(ctx, collibraClient, chosenPortID)
	}

	if len(ports) == 1 {
		return loadPort(ctx, collibraClient, ports[0].ID)
	}

	return nil, &Output{
		Status:      statusNeedsPortSelection,
		OutputPorts: ports,
		Message: fmt.Sprintf("Data product %q has %d output ports. Access is requested per output port, so ask the user which one they need, then call create_asset_access_request again with that port's id in `outputPortId`.",
			dataProduct.Name, len(ports)),
	}, nil
}

// loadPort re-reads a port as a catalog asset, so its real asset type is used rather than an
// assumed one — the type is part of the key Data Access resolves the role by.
func loadPort(ctx context.Context, collibraClient *http.Client, portID string) (*clients.CatalogAssetRef, *Output, error) {
	port, err := clients.GetCatalogAsset(ctx, collibraClient, portID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read output port %s: %s", portID, err.Error())
	}
	if port == nil {
		return nil, nil, fmt.Errorf("no asset found with id %s", portID)
	}
	return port, nil, nil
}

func containsPort(ports []clients.DataProductOutputPort, portID string) bool {
	for _, p := range ports {
		if strings.EqualFold(p.ID, portID) {
			return true
		}
	}
	return false
}

type createParams struct {
	name      string
	purpose   string
	asset     *clients.CatalogAssetRef
	role      *clients.CatalogAssetRole
	users     []*clients.DataAccessIdentity
	groups    []*clients.DataAccessGroup
	expiresAt time.Time
}

func createRequest(ctx context.Context, collibraClient *http.Client, params createParams) Output {
	userIDs := make([]string, 0, len(params.users))
	for _, u := range params.users {
		userIDs = append(userIDs, u.ID)
	}
	groupIDs := make([]string, 0, len(params.groups))
	for _, g := range params.groups {
		groupIDs = append(groupIDs, g.ID)
	}

	request, err := clients.CreateAssetAccessRequest(ctx, collibraClient, clients.CreateAssetAccessRequestInput{
		Name:                    &params.name,
		Description:             buildDescription(params.purpose),
		UserIDs:                 userIDs,
		GroupIDs:                groupIDs,
		RoleID:                  params.role.ID,
		CatalogAsset:            *params.asset,
		ImplementationExpiresAt: params.expiresAt,
	})
	if err != nil {
		return Output{
			Asset:  params.asset,
			Role:   params.role,
			Users:  params.users,
			Groups: params.groups,
			Error:  fmt.Sprintf("Failed to create access request: %s", err.Error()),
		}
	}

	return Output{
		Status:    statusCreated,
		Asset:     params.asset,
		Role:      params.role,
		Users:     params.users,
		Groups:    params.groups,
		ExpiresAt: params.expiresAt.Format(time.RFC3339),
		Request:   request,
		Message: fmt.Sprintf("Access to %s %q was requested through role %q, expiring on %s.",
			params.asset.TypeName, params.asset.Name, params.role.Name, params.expiresAt.Format(time.RFC3339)),
	}
}

// parseExpiresAt accepts an RFC 3339 timestamp or a plain date. A plain date is taken as the
// end of that day in UTC, so "expires 2026-12-31" keeps access for the whole of 31 December.
func parseExpiresAt(value string) (time.Time, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return time.Time{}, fmt.Errorf("expiresAt is required — ask the user when the access must expire (a data product access request cannot be open-ended)")
	}

	expiresAt, err := time.Parse(time.RFC3339, trimmed)
	if err != nil {
		day, dayErr := time.Parse(dateLayout, trimmed)
		if dayErr != nil {
			return time.Time{}, fmt.Errorf("expiresAt %q is not a valid date — use 2026-12-31 or an RFC 3339 timestamp such as 2026-12-31T17:00:00Z", trimmed)
		}
		expiresAt = time.Date(day.Year(), day.Month(), day.Day(), 23, 59, 59, 0, time.UTC)
	}

	if !expiresAt.After(time.Now()) {
		return time.Time{}, fmt.Errorf("expiresAt %s is in the past — ask the user for a future expiration date", expiresAt.Format(time.RFC3339))
	}
	return expiresAt.UTC(), nil
}

func buildDescription(purpose string) string {
	if strings.Contains(purpose, aiDescriptionSuffix) {
		return purpose
	}
	if !strings.HasSuffix(purpose, ".") {
		purpose = purpose + "."
	}
	return purpose + " " + aiDescriptionSuffix
}

// suggestNameFromPurpose derives a short, human-readable name from the purpose text: the
// first sentence, trimmed to a word boundary, behind an "Access request:" prefix.
func suggestNameFromPurpose(purpose string) string {
	summary := strings.ReplaceAll(purpose, aiDescriptionSuffix, "")
	summary = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}
		return r
	}, summary)
	if idx := strings.IndexAny(summary, ".!?"); idx >= 0 {
		summary = summary[:idx]
	}
	summary = strings.Join(strings.Fields(summary), " ")
	if summary == "" {
		return "Access request"
	}
	if len(summary) > suggestedNameMaxLen {
		truncated := summary[:suggestedNameMaxLen]
		if sp := strings.LastIndex(truncated, " "); sp > suggestedNameMaxLen/2 {
			truncated = truncated[:sp]
		}
		summary = strings.TrimRight(truncated, " ,;:-")
	}
	return "Access request: " + summary
}
