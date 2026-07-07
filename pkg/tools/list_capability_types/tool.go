// Package list_capability_types implements the list_capability_types MCP tool: a
// read-only discovery tool for finding valid typeId values (and their manifests) for
// create_capability and create_connection on a given edge site.
package list_capability_types

import (
	"context"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/validation"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct {
	EdgeSiteID string `json:"edgeSiteId" jsonschema:"UUID of the edge site to list available capability and connection types for. Use the list_edge_sites tool to discover available sites."`
	Query      string `json:"query,omitempty" jsonschema:"Optional. Case-insensitive substring filter on type id (e.g. 'jdbc', 'snowflake'). Without a query, all type ids are returned WITHOUT their manifest (an edge site can have 80+ types; manifests are large) — call again with a query matching the type you want to see its full manifest with expected parameters."`
}

type Output struct {
	CapabilityTypes []clients.CapabilityType `json:"capabilityTypes" jsonschema:"Capability types available on this edge site (e.g. 'jdbc-ingestion'). Manifest is only populated when query narrows the results — see query's description."`
	ConnectionTypes []clients.ConnectionType `json:"connectionTypes" jsonschema:"Connection types available on this edge site (e.g. 'Generic' or a vendor-specific type). Manifest is only populated when query narrows the results — see query's description."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "list_capability_types",
		Title:       "List Capability and Connection Types",
		Description: "Lists the capability and connection types available on an edge site. Without query, returns just the ids (an edge site can have 80+ types, each with a large manifest). Pass query to filter to matching types and get their full manifest describing expected parameters for create_capability/create_connection.",
		Handler:     handler(collibraClient),
		Permissions: []string{"dgc.edge-view-connections-and-capabilities"},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if err := validation.UUID("edgeSiteId", input.EdgeSiteID); err != nil {
			return Output{}, err
		}

		capabilityTypes, err := clients.GetCapabilityTypes(ctx, collibraClient, input.EdgeSiteID)
		if err != nil {
			return Output{}, err
		}

		connectionTypes, err := clients.GetConnectionTypes(ctx, collibraClient, input.EdgeSiteID)
		if err != nil {
			return Output{}, err
		}

		query := strings.ToLower(strings.TrimSpace(input.Query))
		if query == "" {
			return Output{
				CapabilityTypes: stripManifests(capabilityTypes),
				ConnectionTypes: stripManifestsConn(connectionTypes),
			}, nil
		}

		matchedCapabilities := []clients.CapabilityType{}
		for _, c := range capabilityTypes {
			if strings.Contains(strings.ToLower(c.ID), query) {
				matchedCapabilities = append(matchedCapabilities, c)
			}
		}
		matchedConnections := []clients.ConnectionType{}
		for _, c := range connectionTypes {
			if strings.Contains(strings.ToLower(c.ID), query) {
				matchedConnections = append(matchedConnections, c)
			}
		}

		return Output{CapabilityTypes: matchedCapabilities, ConnectionTypes: matchedConnections}, nil
	}
}

func stripManifests(types []clients.CapabilityType) []clients.CapabilityType {
	out := make([]clients.CapabilityType, len(types))
	for i, t := range types {
		out[i] = clients.CapabilityType{ID: t.ID}
	}
	return out
}

func stripManifestsConn(types []clients.ConnectionType) []clients.ConnectionType {
	out := make([]clients.ConnectionType, len(types))
	for i, t := range types {
		out[i] = clients.ConnectionType{ID: t.ID}
	}
	return out
}
