// Package edge_create_connection implements the edge_create_connection MCP tool: creates or
// updates an Edge connection (e.g. a JDBC connection backing the jdbc-ingestion
// capability) via the private Edge connection management API.
package edge_create_connection

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
	ConnectionID string         `json:"connectionId,omitempty" jsonschema:"Optional. UUID of the connection to create or update. If provided, the connection is created with this exact id (or updated if it already exists). If omitted, a new connection is created and the server assigns an id."`
	Name         string         `json:"name" jsonschema:"The connection name."`
	Description  string         `json:"description,omitempty" jsonschema:"Optional description of the connection."`
	TypeID       string         `json:"typeId" jsonschema:"The id of the connection type (e.g. 'Generic' for a generic JDBC connection, or a vendor-specific type such as a Snowflake connection type). Determines which parameters are expected — use edge_list_capability_types to inspect a type's manifest."`
	EdgeSiteID   string         `json:"edgeSiteId" jsonschema:"UUID of the edge site where this connection will be valid. Use the edge_list_sites tool to discover available sites."`
	VaultID      string         `json:"vaultId,omitempty" jsonschema:"Optional UUID of the vault to retrieve vault-backed parameters from."`
	Parameters   map[string]any `json:"parameters" jsonschema:"Fixed connection parameters as defined by the connection type's manifest (e.g. driver-class, connection-string for the 'Generic' JDBC connection type). Do NOT put open-ended/vendor-specific properties (e.g. a Snowflake connection's Role, Warehouse, User, Database, or a private key file) here — use additionalProperties instead. If you don't already know a data source's driver class, connection string format, required properties, or verified driver source, call get_data_source_setup_guide first instead of guessing. For 'driver-jar' (and any other FILE-type parameter), the value is an artifact URI from upload_file — see that tool's policy on where driver files may come from."`

	AdditionalProperties    []AdditionalProperty `json:"additionalProperties,omitempty" jsonschema:"Optional. Open-ended, connection-type-defined properties beyond the fixed manifest parameters — e.g. a Snowflake connection (via the 'Generic' JDBC connection type) uses this for Role, Warehouse, User, Database, and private_key_file. Confirmed shape: each entry is injected as {name, type, value, secret} into a single array-valued parameter (see additionalPropertiesKey). For a FILE-type property, first call upload_file and use its returned artifact URI as this entry's value with type='file'."`
	AdditionalPropertiesKey string               `json:"additionalPropertiesKey,omitempty" jsonschema:"Optional. The manifest parameter name that additionalProperties are grouped under. Defaults to 'connection-properties', which is correct for the 'Generic' JDBC connection type (confirmed live). Other connection types may use a different key (e.g. AWS/GCP connections use 'additional-parameters') — check the type's manifest via edge_list_capability_types if unsure."`
}

// AdditionalProperty is one entry of an open-ended, connection-type-defined property
// list (DGC's ConnectionManifest.ParameterType.USER_DEFINED). Confirmed against a real
// working Snowflake-via-Generic-JDBC connection: each entry is {name, type, value,
// secret}, and a FILE-type entry's value is the bare artifact URI (same shape as any
// other FILE parameter), not a wrapped object.
type AdditionalProperty struct {
	Name   string `json:"name" jsonschema:"The property name (e.g. 'Role', 'Warehouse', 'private_key_file')."`
	Type   string `json:"type,omitempty" jsonschema:"'string' (default) or 'file'. Use 'file' when value is an artifact URI returned by upload_file."`
	Value  string `json:"value" jsonschema:"The property value. For type='file', this is the artifact URI returned by upload_file, not raw file content."`
	Secret bool   `json:"secret,omitempty" jsonschema:"Optional. Whether this value should be treated as a secret (e.g. a private key file or password)."`
}

type Output struct {
	Connection *clients.EdgeConnection `json:"connection,omitempty" jsonschema:"The created or updated connection."`
	Success    bool                    `json:"success" jsonschema:"Whether the connection was saved successfully."`
	Error      string                  `json:"error,omitempty" jsonschema:"Error message if saving the connection failed."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "edge_create_connection",
		Title:       "Create or Update Edge Connection",
		Description: "Creates or updates an Edge connection (e.g. a JDBC connection for the jdbc-ingestion capability) via the private Edge connection management API. Does not download driver files itself — see upload_file for how driver jars must be sourced (Collibra Marketplace only, not arbitrary URLs).",
		Handler:     handler(collibraClient),
		Permissions: []string{"dgc.edge-integration-capability-manage"},
		Annotations: &mcp.ToolAnnotations{DestructiveHint: chip.Ptr(true)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if err := validation.UUIDOptional("connectionId", input.ConnectionID); err != nil {
			return Output{}, err
		}
		if err := validation.UUID("edgeSiteId", input.EdgeSiteID); err != nil {
			return Output{}, err
		}
		if err := validation.UUIDOptional("vaultId", input.VaultID); err != nil {
			return Output{}, err
		}

		parameters := input.Parameters
		if parameters == nil {
			parameters = map[string]any{}
		}

		if len(input.AdditionalProperties) > 0 {
			key := input.AdditionalPropertiesKey
			if key == "" {
				key = "connection-properties"
			}
			entries := make([]map[string]any, len(input.AdditionalProperties))
			for i, prop := range input.AdditionalProperties {
				if prop.Name == "" {
					return Output{}, fmt.Errorf("additionalProperties[%d]: name is required", i)
				}
				propType := prop.Type
				if propType == "" {
					propType = "string"
				}
				entries[i] = map[string]any{
					"name":   prop.Name,
					"type":   propType,
					"value":  prop.Value,
					"secret": prop.Secret,
				}
			}
			parameters[key] = entries
		}

		request := clients.ConnectionRequest{
			Name:        input.Name,
			Description: input.Description,
			TypeID:      input.TypeID,
			EdgeSiteID:  input.EdgeSiteID,
			VaultID:     input.VaultID,
			Parameters:  parameters,
		}

		var connection *clients.EdgeConnection
		var err error
		if input.ConnectionID != "" {
			connection, err = clients.CreateOrUpdateConnection(ctx, collibraClient, input.ConnectionID, request)
		} else {
			connection, err = clients.CreateConnection(ctx, collibraClient, request)
		}
		if err != nil {
			return Output{Success: false, Error: fmt.Sprintf("failed to save connection: %s", err.Error())}, nil
		}

		return Output{Connection: connection, Success: true}, nil
	}
}
