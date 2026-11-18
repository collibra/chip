// Package find_responsibilities implements the find_responsibilities MCP tool, a thin interface to
// DGC's GET /rest/2.0/responsibilities: DGC's parameter names and defaults, except a default limit of 20.
package find_responsibilities

import (
	"context"
	"fmt"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/validation"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct {
	RoleIDs            []string `json:"roleIds,omitempty" jsonschema:"Optional. UUIDs of roles, from search_roles. Returns the assignments of those roles."`
	OwnerIDs           []string `json:"ownerIds,omitempty" jsonschema:"Optional. UUIDs of users or user groups, from search_users or search_user_groups. Returns what they are assigned."`
	ResourceIDs        []string `json:"resourceIds,omitempty" jsonschema:"Optional. UUIDs of communities, domains or assets, from list_communities, list_domains or an asset search. Returns who is assigned on them."`
	IncludeInherited   *bool    `json:"includeInherited,omitempty" jsonschema:"Optional, with resourceIds only. Also return assignments inherited from the parent community or domain. Default: true."`
	GlobalOnly         *bool    `json:"globalOnly,omitempty" jsonschema:"Optional. Only global role assignments, which apply to the whole instance. Default: false."`
	ExcludeEmptyGroups *bool    `json:"excludeEmptyGroups,omitempty" jsonschema:"Optional. Leave out assignments held by a user group with no members. Default: false."`
	Offset             *int     `json:"offset,omitempty" jsonschema:"Optional. Rows to skip. Default: 0."`
	Limit              *int     `json:"limit,omitempty" jsonschema:"Optional. Rows to return, at most 1000; 0 means 1000. Default: 20."`
	CountLimit         *int     `json:"countLimit,omitempty" jsonschema:"Optional. -1 counts every match in total, 0 skips the count (total is 0), n caps total at n. Default: -1."`
}

type Output struct {
	Total   int64                             `json:"total" jsonschema:"Matching assignments, as counted per countLimit."`
	Offset  int                               `json:"offset"`
	Limit   int                               `json:"limit"`
	Results []clients.DirectoryResponsibility `json:"results" jsonschema:"Each assignment: role, owner (a User or UserGroup) and resource, which is absent for a global role."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "find_responsibilities",
		Title: "Find Role Assignments",
		Description: "Find who holds which role on which resource. In Collibra a role (a named set of permissions) is given to a user or to a user group on a resource (a community, a domain or an asset); " +
			"one such assignment is called a responsibility. Filter by role (roleIds), by owner (ownerIds, users or user groups) or by resource (resourceIds); a filter on assignments of the whole instance is globalOnly. " +
			"Each returned row names the role, the owner (with its type, User or UserGroup) and the resource, which is absent for a global role; the response also has total, offset and limit for paging. " +
			"Get role ids from search_roles, owner ids from search_users or search_user_groups, and resource ids from list_communities, list_domains or an asset search. " +
			"At least one filter is required: an unfiltered call is refused. With resourceIds, assignments inherited from parent communities and domains are included unless includeInherited is false. " +
			"Example user requests: \"Who are all the Product Owners?\"; \"Who is the Steward of the Finance domain?\"; \"Which roles does the Data Stewards group hold?\".",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: chip.Ptr(false), IdempotentHint: true, OpenWorldHint: chip.Ptr(false)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if len(input.RoleIDs) == 0 && len(input.OwnerIDs) == 0 && len(input.ResourceIDs) == 0 && (input.GlobalOnly == nil || !*input.GlobalOnly) {
			return Output{}, fmt.Errorf("give at least one filter: roleIds, ownerIds, resourceIds or globalOnly. An unfiltered search of every role assignment is refused")
		}
		if err := validation.DashedUUIDs("roleIds", input.RoleIDs); err != nil {
			return Output{}, err
		}
		if err := validation.DashedUUIDs("ownerIds", input.OwnerIDs); err != nil {
			return Output{}, err
		}
		if err := validation.DashedUUIDs("resourceIds", input.ResourceIDs); err != nil {
			return Output{}, err
		}
		page, err := clients.SearchResponsibilities(ctx, collibraClient, clients.SearchResponsibilitiesRequest{
			ResourceIDs:        input.ResourceIDs,
			OwnerIDs:           input.OwnerIDs,
			RoleIDs:            input.RoleIDs,
			IncludeInherited:   input.IncludeInherited,
			GlobalOnly:         input.GlobalOnly,
			ExcludeEmptyGroups: input.ExcludeEmptyGroups,
			DirectoryPaging:    clients.DirectoryPaging{Offset: input.Offset, Limit: input.Limit, CountLimit: input.CountLimit},
		})
		if err != nil {
			return Output{}, err
		}
		return Output{Total: page.Total, Offset: page.Offset, Limit: page.Limit, Results: page.Results}, nil
	}
}
