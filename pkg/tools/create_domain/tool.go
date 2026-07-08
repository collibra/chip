// Package create_domain implements the create_domain MCP tool: creates a DGC domain
// within a community, e.g. a "Physical Data Dictionary" domain required before
// registering a database via register_database.
package create_domain

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
	Name                         string `json:"name" jsonschema:"The domain name."`
	Description                  string `json:"description,omitempty" jsonschema:"Optional description of the domain."`
	CommunityID                  string `json:"communityId" jsonschema:"UUID of the community (created via create_community) this domain belongs to."`
	TypeID                       string `json:"typeId" jsonschema:"UUID of the domain type. Use find_domain_types to resolve a name (e.g. 'Physical Data Dictionary', used for jdbc-ingestion databases) to its UUID."`
	ExcludedFromAutoHyperlinking bool   `json:"excludedFromAutoHyperlinking,omitempty" jsonschema:"Optional. If true, assets in this domain are excluded from automatic hyperlinking."`
}

type Output struct {
	Domain  *clients.DomainDetails `json:"domain,omitempty" jsonschema:"The created domain."`
	Success bool                   `json:"success" jsonschema:"Whether the domain was created successfully."`
	Error   string                 `json:"error,omitempty" jsonschema:"Error message if creating the domain failed."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "create_domain",
		Title:       "Create Domain",
		Description: "Creates a DGC domain within a community, e.g. a Physical Data Dictionary domain required before registering a database via register_database.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{DestructiveHint: chip.Ptr(true)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if err := validation.UUID("communityId", input.CommunityID); err != nil {
			return Output{}, err
		}
		if err := validation.UUID("typeId", input.TypeID); err != nil {
			return Output{}, err
		}

		domain, err := clients.CreateDomain(ctx, collibraClient, clients.CreateDomainRequest{
			Name:                         input.Name,
			Description:                  input.Description,
			CommunityID:                  input.CommunityID,
			TypeID:                       input.TypeID,
			ExcludedFromAutoHyperlinking: input.ExcludedFromAutoHyperlinking,
		})
		if err != nil {
			return Output{Success: false, Error: fmt.Sprintf("failed to create domain: %s", err.Error())}, nil
		}
		return Output{Domain: domain, Success: true}, nil
	}
}
