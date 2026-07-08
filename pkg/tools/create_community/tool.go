// Package create_community implements the create_community MCP tool: creates a DGC
// community, the top-level catalog scaffolding required before creating domains and
// registering databases via register_database.
package create_community

import (
	"context"
	"fmt"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct {
	Name        string `json:"name" jsonschema:"The community name."`
	Description string `json:"description,omitempty" jsonschema:"Optional description of the community."`
}

type Output struct {
	Community *clients.CommunityDetails `json:"community,omitempty" jsonschema:"The created community."`
	Success   bool                      `json:"success" jsonschema:"Whether the community was created successfully."`
	Error     string                    `json:"error,omitempty" jsonschema:"Error message if creating the community failed."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "create_community",
		Title:       "Create Community",
		Description: "Creates a DGC community. A community is required before creating a domain (create_domain) and registering a database (register_database).",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{DestructiveHint: chip.Ptr(true)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		community, err := clients.CreateCommunity(ctx, collibraClient, clients.CreateCommunityRequest{
			Name:        input.Name,
			Description: input.Description,
		})
		if err != nil {
			return Output{Success: false, Error: fmt.Sprintf("failed to create community: %s", err.Error())}, nil
		}
		return Output{Community: community, Success: true}, nil
	}
}
