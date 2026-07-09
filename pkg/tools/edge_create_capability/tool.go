// Package edge_create_capability implements the edge_create_capability MCP tool: creates or
// updates an Edge capability (e.g. jdbc-ingestion or a lineage capability) via the
// private Edge capability management API.
package edge_create_capability

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
	TypeID       string         `json:"typeId" jsonschema:"The id of the capability type (e.g. 'jdbc-ingestion'). Use the edge_list_capability_types tool to discover available types and their expected parameters."`
	EdgeSiteID   string         `json:"edgeSiteId" jsonschema:"UUID of the edge site where this capability will run. Use the edge_list_sites tool to discover available sites."`
	Parameters   map[string]any `json:"parameters" jsonschema:"Capability install parameters as defined by the capability type's manifest — read it via edge_list_capability_types (pass a query) and ask the user for user-choice values; never invent them. For jdbc-ingestion this includes 'connection' (the id of a connection created via edge_create_connection), 'data-source-type', 'message-mode', and an optional 'other-settings' list of {name, type, value}. For technical lineage capabilities (edgeharvester-*) the parameters are the capability's entire configuration — collect and confirm them with the user before calling this tool (see the collibra/techlin skill); custom properties like techlinHost/techlinKey go inside the 'customParameters' list parameter as {name, value, type: 'string', secret: false, encrypted: false, fromVault: false} objects (never top-level); harvest-query parameters are omitted to accept the defaults (the server fills required ones) or set to the literal string 'use-default' — never invented SQL."`
}

type Output struct {
	Capability *clients.EdgeCapability `json:"capability,omitempty" jsonschema:"The created or updated capability."`
	Success    bool                    `json:"success" jsonschema:"Whether the capability was saved successfully."`
	Error      string                  `json:"error,omitempty" jsonschema:"Error message if saving the capability failed."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "edge_create_capability",
		Title:       "Create or Update Edge Capability",
		Description: "Creates or updates an Edge capability (e.g. jdbc-ingestion or a technical lineage capability) via the private Edge capability management API. Parameters must come from the capability type's manifest (edge_list_capability_types) and, for user-choice values, from the user — confirm the full set before calling. The server materializes defaults for required manifest parameters at save (the response echoes them back), and rejects a second capability of the same type on one connection (400 'already used') — treat that as 'the capability already exists', not as a reason to switch connections. Does not run the capability — use start_ingestion to trigger a jdbc-ingestion run, or start_technical_lineage for a technical lineage harvest.",
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
