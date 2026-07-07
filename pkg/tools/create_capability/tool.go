// Package create_capability implements the create_capability MCP tool: creates or
// updates an Edge capability (e.g. jdbc-ingestion or a lineage capability) via the
// private Edge capability management API.
package create_capability

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
	CapabilityID string         `json:"capabilityId,omitempty" jsonschema:"Optional. UUID of the capability to create or update. If provided, the capability is created with this exact id (or updated if it already exists). If omitted, a new capability is created and the server assigns an id."`
	Name         string         `json:"name" jsonschema:"The capability name."`
	Description  string         `json:"description,omitempty" jsonschema:"Optional description of the capability."`
	TypeID       string         `json:"typeId" jsonschema:"The id of the capability type (e.g. 'jdbc-ingestion'). Use the list_capability_types tool to discover available types and their expected parameters."`
	EdgeSiteID   string         `json:"edgeSiteId" jsonschema:"UUID of the edge site where this capability will run. Use the list_edge_sites tool to discover available sites."`
	Parameters   map[string]any `json:"parameters" jsonschema:"Capability install parameters as defined by the capability type's manifest. For jdbc-ingestion, this includes 'connection' (the id of a connection created via create_connection), 'data-source-type', 'message-mode', and an optional 'other-settings' list of {name, type, value}."`
}

type Output struct {
	Capability *clients.EdgeCapability `json:"capability,omitempty" jsonschema:"The created or updated capability."`
	Success    bool                `json:"success" jsonschema:"Whether the capability was saved successfully."`
	Error      string              `json:"error,omitempty" jsonschema:"Error message if saving the capability failed."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "create_capability",
		Title:       "Create or Update Edge Capability",
		Description: "Creates or updates an Edge capability (e.g. jdbc-ingestion) via the private Edge capability management API. Does not run the capability — use start_ingestion to trigger a jdbc-ingestion run.",
		Handler:     handler(collibraClient),
		Permissions: []string{"dgc.edge-integration-capability-manage"},
		Annotations: &mcp.ToolAnnotations{DestructiveHint: chip.Ptr(true)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if err := validation.UUIDOptional("capabilityId", input.CapabilityID); err != nil {
			return Output{}, err
		}
		if err := validation.UUID("edgeSiteId", input.EdgeSiteID); err != nil {
			return Output{}, err
		}

		parameters := input.Parameters
		if parameters == nil {
			parameters = map[string]any{}
		}

		request := clients.CapabilityRequest{
			TypeID:      input.TypeID,
			Name:        input.Name,
			Description: input.Description,
			EdgeSiteID:  input.EdgeSiteID,
			Parameters:  parameters,
		}

		var capability *clients.EdgeCapability
		var err error
		if input.CapabilityID != "" {
			capability, err = clients.CreateOrUpdateCapability(ctx, collibraClient, input.CapabilityID, request)
		} else {
			capability, err = clients.CreateCapability(ctx, collibraClient, request)
		}
		if err != nil {
			return Output{Success: false, Error: fmt.Sprintf("failed to save capability: %s", err.Error())}, nil
		}

		return Output{Capability: capability, Success: true}, nil
	}
}
