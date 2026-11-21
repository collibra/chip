// Package list_communities implements the list_communities MCP tool, a thin interface to DGC's GET
// /rest/2.0/communities: DGC's parameter names and defaults, except a default limit of 20.
package list_communities

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/validation"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct {
	Name          string `json:"name,omitempty" jsonschema:"Optional. Text to find in the community name."`
	NameMatchMode string `json:"nameMatchMode,omitempty" jsonschema:"Optional. START, END, ANYWHERE or EXACT, always case-insensitive. Default: ANYWHERE."`
	ParentID      string `json:"parentId,omitempty" jsonschema:"Optional. UUID of a community, to return only its direct sub-communities."`
	ExcludeMeta   *bool  `json:"excludeMeta,omitempty" jsonschema:"Optional. Leave out the communities Collibra uses for its own configuration. Default: true."`
	Offset        *int   `json:"offset,omitempty" jsonschema:"Optional. Rows to skip. Default: 0."`
	Limit         *int   `json:"limit,omitempty" jsonschema:"Optional. Rows to return, at most 1000; 0 means 1000. Default: 20."`
	CountLimit    *int   `json:"countLimit,omitempty" jsonschema:"Optional. -1 counts every match in total, 0 skips the count (total is 0), n caps total at n. Default: -1."`
}

type Output struct {
	Total   int64                        `json:"total" jsonschema:"Matching communities, as counted per countLimit."`
	Offset  int                          `json:"offset"`
	Limit   int                          `json:"limit"`
	Results []clients.DirectoryCommunity `json:"results"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "list_communities",
		Title: "List Communities",
		Description: "List Collibra communities. A community is the top-level container of the data governance hierarchy: it groups domains (containers of assets) and can contain sub-communities. " +
			"Filter by name, or give parentId to list the direct sub-communities of one community; with no filter the call returns the first page of all communities. " +
			"Returns each community's id, name, description and parent, plus total, offset and limit for paging. The communities Collibra uses for its own configuration are left out unless excludeMeta is false. " +
			"Use a community id as communityId in list_domains to see its domains, or as resourceIds in find_responsibilities to see who holds roles on it. " +
			"Example user requests: \"What communities do we have?\"; \"Find the Finance community\"; \"What sits under the Sales community?\".",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: chip.Ptr(false), IdempotentHint: true, OpenWorldHint: chip.Ptr(false)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if err := validation.DashedUUIDOptional("parentId", input.ParentID); err != nil {
			return Output{}, err
		}
		page, err := clients.ListCommunities(ctx, collibraClient, clients.ListCommunitiesRequest{
			Name:            input.Name,
			NameMatchMode:   input.NameMatchMode,
			ParentID:        input.ParentID,
			ExcludeMeta:     input.ExcludeMeta,
			DirectoryPaging: clients.DirectoryPaging{Offset: input.Offset, Limit: input.Limit, CountLimit: input.CountLimit},
		})
		if err != nil {
			return Output{}, err
		}
		return Output{Total: page.Total, Offset: page.Offset, Limit: page.Limit, Results: page.Results}, nil
	}
}
