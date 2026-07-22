// Package edge_find_connections implements the edge_find_connections MCP tool: finds Edge
// connections by name. Primarily for picking up a connection the user created
// manually (e.g. via the DGC/Edge UI, when a driver file was too large to upload
// through upload_file's contentBase64) — see edge_create_connection's package doc for the
// guided-manual-creation flow this supports.
package edge_find_connections

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/validation"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct {
	EdgeSiteID    string `json:"edgeSiteId,omitempty" jsonschema:"Optional. UUID of the edge site to restrict the search to."`
	Name          string `json:"name,omitempty" jsonschema:"Optional. The connection name to search for."`
	NameMatchMode string `json:"nameMatchMode,omitempty" jsonschema:"Optional. 'ANYWHERE' (default, substring match) or 'EXACT'."`
}

type Output struct {
	Connections []clients.EdgeConnection `json:"connections" jsonschema:"The matching connections."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "edge_find_connections",
		Title:       "Find Edge Connections",
		Description: "Finds Edge connections by name (and optionally edge site). Use this to pick up a connection that already exists — e.g. one the user created manually via the DGC/Edge UI for a driver file too large to pass through upload_file — instead of creating a duplicate with edge_create_connection.",
		Handler:     handler(collibraClient),
		Permissions: []string{"dgc.edge-view-connections-and-capabilities"},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if err := validation.UUIDOptional("edgeSiteId", input.EdgeSiteID); err != nil {
			return Output{}, err
		}

		connections, err := clients.FindConnections(ctx, collibraClient, clients.ConnectionFindRequest{
			EdgeSiteID:    input.EdgeSiteID,
			Name:          input.Name,
			NameMatchMode: input.NameMatchMode,
		})
		if err != nil {
			return Output{}, err
		}

		return Output{Connections: connections}, nil
	}
}
