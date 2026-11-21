// Package search_roles implements the search_roles MCP tool, a thin interface to DGC's GET
// /rest/2.0/roles: DGC's parameter names and defaults, except a default limit of 20.
package search_roles

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/validation"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct {
	Name                string `json:"name,omitempty" jsonschema:"Optional. Text to find in the role name; with description, a role matching either is returned."`
	NameMatchMode       string `json:"nameMatchMode,omitempty" jsonschema:"Optional. START, END, ANYWHERE or EXACT, always case-insensitive. Default: ANYWHERE."`
	Description         string `json:"description,omitempty" jsonschema:"Optional. Text to find in the role description."`
	Type                string `json:"type,omitempty" jsonschema:"Optional. ALL, GLOBAL or RESOURCE. Default: ALL. Resource roles are the ones assigned on a community, domain or asset, such as Owner or Steward."`
	OwnerID             string `json:"ownerId,omitempty" jsonschema:"Optional. UUID of a user or user group, from search_users or search_user_groups, to return the roles it holds."`
	RoleInheritanceMode string `json:"roleInheritanceMode,omitempty" jsonschema:"Optional, with ownerId only. DIRECTLY, VIA_GROUPS or DIRECTLY_AND_VIA_GROUPS. Default: DIRECTLY_AND_VIA_GROUPS."`
	IncludeDisabled     *bool  `json:"includeDisabled,omitempty" jsonschema:"Optional. Also return disabled roles. Default: false."`
	Offset              *int   `json:"offset,omitempty" jsonschema:"Optional. Rows to skip. Default: 0."`
	Limit               *int   `json:"limit,omitempty" jsonschema:"Optional. Rows to return, at most 1000; 0 means 1000. Default: 20."`
	CountLimit          *int   `json:"countLimit,omitempty" jsonschema:"Optional. -1 counts every match in total, 0 skips the count (total is 0), n caps total at n. Default: -1."`
}

type Output struct {
	Total   int64                   `json:"total" jsonschema:"Matching roles, as counted per countLimit."`
	Offset  int                     `json:"offset"`
	Limit   int                     `json:"limit"`
	Results []clients.DirectoryRole `json:"results"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "search_roles",
		Title: "Search Roles",
		Description: "Find Collibra roles by name or description, by type, or the roles a user or user group holds (ownerId). " +
			"A role is a named set of permissions. GLOBAL roles apply to the whole Collibra instance (for example Sysadmin). RESOURCE roles are given on a community, domain or asset (for example Owner, Steward or Product Owner). " +
			"Returns each role's id, name, description and whether it is global, disabled or a system role, plus total, offset and limit for paging. " +
			"Roles are a small, bounded set, so the call may be unfiltered; it still pages. Use a role's id as roleIds in find_responsibilities to see who holds it. " +
			"This finds roles, not who holds them. " +
			"Example user requests: \"Which resource roles exist?\"; \"Find the Product Owner role\"; \"What roles does Jane Smith hold?\".",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: chip.Ptr(false), IdempotentHint: true, OpenWorldHint: chip.Ptr(false)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		// DGC rejects an unknown enum value, or a parameter combination it does not take, with a 400.
		if err := validation.DashedUUIDOptional("ownerId", input.OwnerID); err != nil {
			return Output{}, err
		}
		page, err := clients.SearchRoles(ctx, collibraClient, clients.SearchRolesRequest{
			Name:                input.Name,
			NameMatchMode:       input.NameMatchMode,
			Description:         input.Description,
			Type:                input.Type,
			OwnerID:             input.OwnerID,
			RoleInheritanceMode: input.RoleInheritanceMode,
			IncludeDisabled:     input.IncludeDisabled,
			DirectoryPaging:     clients.DirectoryPaging{Offset: input.Offset, Limit: input.Limit, CountLimit: input.CountLimit},
		})
		if err != nil {
			return Output{}, err
		}
		return Output{Total: page.Total, Offset: page.Offset, Limit: page.Limit, Results: page.Results}, nil
	}
}
