// Package list_domains implements the list_domains MCP tool, a thin interface to DGC's GET
// /rest/2.0/domains: DGC's parameter names and defaults, except a default limit of 20.
package list_domains

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
	Name                  string `json:"name,omitempty" jsonschema:"Optional. Text to find in the domain name."`
	NameMatchMode         string `json:"nameMatchMode,omitempty" jsonschema:"Optional. START, END, ANYWHERE or EXACT, always case-insensitive. Default: ANYWHERE."`
	CommunityID           string `json:"communityId,omitempty" jsonschema:"Optional. UUID of a community, from list_communities, to return only its domains."`
	TypeID                string `json:"typeId,omitempty" jsonschema:"Optional. UUID of a domain type, to return only domains of that type."`
	IncludeSubCommunities *bool  `json:"includeSubCommunities,omitempty" jsonschema:"Optional, with communityId only. Also return the domains of its sub-communities. Default: false."`
	ExcludeMeta           *bool  `json:"excludeMeta,omitempty" jsonschema:"Optional. Leave out the domains Collibra uses for its own configuration. Default: true."`
	Offset                *int   `json:"offset,omitempty" jsonschema:"Optional. Rows to skip. Default: 0."`
	Limit                 *int   `json:"limit,omitempty" jsonschema:"Optional. Rows to return, at most 1000; 0 means 1000. Default: 20."`
	CountLimit            *int   `json:"countLimit,omitempty" jsonschema:"Optional. -1 counts every match in total, 0 skips the count (total is 0), n caps total at n. Default: -1."`
}

type Output struct {
	Total   int64                     `json:"total" jsonschema:"Matching domains, as counted per countLimit."`
	Offset  int                       `json:"offset"`
	Limit   int                       `json:"limit"`
	Results []clients.DirectoryDomain `json:"results"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "list_domains",
		Title: "List Domains",
		Description: "List Collibra domains. A domain is a container of assets (business terms, tables, policies and so on) inside a community; its domain type says what it holds, such as a Glossary or a Physical Data Dictionary. " +
			"Filter by name, by the community it belongs to (communityId, with includeSubCommunities to include nested ones) or by domain type (typeId). " +
			"Returns each domain's id, name, description, type and community, plus total, offset and limit for paging. The domains Collibra uses for its own configuration are left out unless excludeMeta is false. " +
			"At least one filter is required: an unfiltered call is refused. Get community ids from list_communities. " +
			"Use a domain id as resourceIds in find_responsibilities to see who holds roles on it. " +
			"Example user requests: \"What domains are in the Finance community?\"; \"Find the Customer Glossary domain\"; \"List all glossary domains\".",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: chip.Ptr(false), IdempotentHint: true, OpenWorldHint: chip.Ptr(false)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if input.Name == "" && input.CommunityID == "" && input.TypeID == "" {
			return Output{}, fmt.Errorf("give at least one filter: name, communityId or typeId. An unfiltered search of every domain is refused")
		}
		if err := validation.DashedUUIDOptional("communityId", input.CommunityID); err != nil {
			return Output{}, err
		}
		if err := validation.DashedUUIDOptional("typeId", input.TypeID); err != nil {
			return Output{}, err
		}
		page, err := clients.ListDomains(ctx, collibraClient, clients.ListDomainsRequest{
			Name:                  input.Name,
			NameMatchMode:         input.NameMatchMode,
			CommunityID:           input.CommunityID,
			TypeID:                input.TypeID,
			IncludeSubCommunities: input.IncludeSubCommunities,
			ExcludeMeta:           input.ExcludeMeta,
			DirectoryPaging:       clients.DirectoryPaging{Offset: input.Offset, Limit: input.Limit, CountLimit: input.CountLimit},
		})
		if err != nil {
			return Output{}, err
		}
		return Output{Total: page.Total, Offset: page.Offset, Limit: page.Limit, Results: page.Results}, nil
	}
}
