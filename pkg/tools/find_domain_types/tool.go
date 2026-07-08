// Package find_domain_types implements the find_domain_types MCP tool: resolves a
// domain type's name (e.g. "Physical Data Dictionary") to its UUID, for
// create_domain's typeId — there is no other discovery path for this, since domain
// types are not returned by edge_list_capability_types or any Edge API.
package find_domain_types

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct {
	Name   string `json:"name,omitempty" jsonschema:"Optional. Matches case-insensitively as a substring against the domain type name (e.g. 'Physical Data Dictionary', 'Technology Asset'). Omit to list all domain types."`
	Limit  int    `json:"limit,omitempty" jsonschema:"Optional. Maximum number of results to return. Default: 50."`
	Offset int    `json:"offset,omitempty" jsonschema:"Optional. Index of first result (pagination offset). Default: 0."`
}

type Output struct {
	Total       int64        `json:"total" jsonschema:"The total number of domain types matching the search criteria."`
	Offset      int64        `json:"offset" jsonschema:"The offset for the results."`
	Limit       int64        `json:"limit" jsonschema:"The maximum number of results returned."`
	DomainTypes []DomainType `json:"domainTypes" jsonschema:"The matching domain types."`
}

type DomainType struct {
	ID          string `json:"id" jsonschema:"The domain type's UUID — use this for create_domain's typeId."`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	PublicID    string `json:"publicId,omitempty"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "find_domain_types",
		Title:       "Find Domain Types",
		Description: "Finds DGC domain types by name (substring, case-insensitive), returning their UUIDs. Use this to resolve a domain type name (e.g. 'Physical Data Dictionary', needed for jdbc-ingestion databases) to a typeId for create_domain, instead of needing to already know the UUID.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		limit := input.Limit
		if limit == 0 {
			limit = 50
		}

		response, err := clients.FindDomainTypes(ctx, collibraClient, clients.FindDomainTypesQueryParams{
			Name:   input.Name,
			Limit:  limit,
			Offset: input.Offset,
		})
		if err != nil {
			return Output{}, err
		}

		domainTypes := make([]DomainType, len(response.Results))
		for i, d := range response.Results {
			domainTypes[i] = DomainType{ID: d.ID, Name: d.Name, Description: d.Description, PublicID: d.PublicID}
		}

		return Output{Total: response.Total, Offset: response.Offset, Limit: response.Limit, DomainTypes: domainTypes}, nil
	}
}
