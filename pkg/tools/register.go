package tools

import (
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/tools/add_business_term"
	"github.com/collibra/chip/pkg/tools/add_data_classification_match"
	"github.com/collibra/chip/pkg/tools/create_asset"
	"github.com/collibra/chip/pkg/tools/create_control"
	"github.com/collibra/chip/pkg/tools/discover_business_glossary"
	"github.com/collibra/chip/pkg/tools/discover_data_assets"
	"github.com/collibra/chip/pkg/tools/dry_run_control_query"
	"github.com/collibra/chip/pkg/tools/enable_control"
	"github.com/collibra/chip/pkg/tools/execute_control"
	"github.com/collibra/chip/pkg/tools/find_asset_type"
	"github.com/collibra/chip/pkg/tools/find_attribute_type"
	"github.com/collibra/chip/pkg/tools/find_relation_type"
	"github.com/collibra/chip/pkg/tools/find_status"
	"github.com/collibra/chip/pkg/tools/get_asset_details"
	"github.com/collibra/chip/pkg/tools/get_business_term_data"
	"github.com/collibra/chip/pkg/tools/get_column_semantics"
	"github.com/collibra/chip/pkg/tools/get_control"
	"github.com/collibra/chip/pkg/tools/get_control_execution_report"
	"github.com/collibra/chip/pkg/tools/get_lineage_downstream"
	"github.com/collibra/chip/pkg/tools/get_lineage_entity"
	"github.com/collibra/chip/pkg/tools/get_lineage_transformation"
	"github.com/collibra/chip/pkg/tools/get_lineage_upstream"
	"github.com/collibra/chip/pkg/tools/get_measure_data"
	"github.com/collibra/chip/pkg/tools/get_table_semantics"
	"github.com/collibra/chip/pkg/tools/list_asset_types"
	"github.com/collibra/chip/pkg/tools/list_attribute_types"
	"github.com/collibra/chip/pkg/tools/list_controls"
	"github.com/collibra/chip/pkg/tools/list_data_contracts"
	"github.com/collibra/chip/pkg/tools/list_managed_control_attributes"
	"github.com/collibra/chip/pkg/tools/list_relation_types"
	"github.com/collibra/chip/pkg/tools/list_statuses"
	"github.com/collibra/chip/pkg/tools/prepare_create_asset"
	"github.com/collibra/chip/pkg/tools/prepare_add_business_term"
	"github.com/collibra/chip/pkg/tools/pull_data_contract_manifest"
	"github.com/collibra/chip/pkg/tools/push_data_contract_manifest"
	"github.com/collibra/chip/pkg/tools/remove_data_classification_match"
	"github.com/collibra/chip/pkg/tools/resolve_domain"
	"github.com/collibra/chip/pkg/tools/search_asset_keyword"
	"github.com/collibra/chip/pkg/tools/search_data_classification_matches"
	"github.com/collibra/chip/pkg/tools/search_data_classes"
	"github.com/collibra/chip/pkg/tools/search_lineage_entities"
	"github.com/collibra/chip/pkg/tools/search_lineage_transformations"
)

// CopilotToolNames lists tool names that are routed to the copilot service.
// Used by chip-service to direct these requests to the copilot backend
// instead of the standard DGC API.
var CopilotToolNames = []string{
	"discover_data_assets",
	"discover_business_glossary",
}

func RegisterAll(server *chip.Server, client *http.Client, toolConfig *chip.ServerToolConfig) {
	toolRegister(server, toolConfig, discover_data_assets.NewTool(client))
	toolRegister(server, toolConfig, discover_business_glossary.NewTool(client))
	toolRegister(server, toolConfig, get_asset_details.NewTool(client))
	toolRegister(server, toolConfig, search_asset_keyword.NewTool(client))
	toolRegister(server, toolConfig, search_data_classes.NewTool(client))
	toolRegister(server, toolConfig, list_asset_types.NewTool(client))
	toolRegister(server, toolConfig, add_data_classification_match.NewTool(client))
	toolRegister(server, toolConfig, search_data_classification_matches.NewTool(client))
	toolRegister(server, toolConfig, remove_data_classification_match.NewTool(client))
	toolRegister(server, toolConfig, list_data_contracts.NewTool(client))
	toolRegister(server, toolConfig, push_data_contract_manifest.NewTool(client))
	toolRegister(server, toolConfig, pull_data_contract_manifest.NewTool(client))
	toolRegister(server, toolConfig, prepare_add_business_term.NewTool(client))
	toolRegister(server, toolConfig, get_business_term_data.NewTool(client))
	toolRegister(server, toolConfig, get_column_semantics.NewTool(client))
	toolRegister(server, toolConfig, get_lineage_downstream.NewTool(client))
	toolRegister(server, toolConfig, get_lineage_entity.NewTool(client))
	toolRegister(server, toolConfig, get_lineage_transformation.NewTool(client))
	toolRegister(server, toolConfig, get_lineage_upstream.NewTool(client))
	toolRegister(server, toolConfig, get_measure_data.NewTool(client))
	toolRegister(server, toolConfig, get_table_semantics.NewTool(client))
	toolRegister(server, toolConfig, search_lineage_entities.NewTool(client))
	toolRegister(server, toolConfig, search_lineage_transformations.NewTool(client))
	toolRegister(server, toolConfig, prepare_create_asset.NewTool(client))
	toolRegister(server, toolConfig, add_business_term.NewTool(client))
	toolRegister(server, toolConfig, create_asset.NewTool(client))

	// Control Tower (create-control flow): DGC discovery + management +
	// execution endpoints.
	toolRegister(server, toolConfig, resolve_domain.NewTool(client))
	toolRegister(server, toolConfig, list_managed_control_attributes.NewTool(client))
	toolRegister(server, toolConfig, dry_run_control_query.NewTool(client))
	toolRegister(server, toolConfig, create_control.NewTool(client))
	toolRegister(server, toolConfig, enable_control.NewTool(client))
	toolRegister(server, toolConfig, execute_control.NewTool(client))
	toolRegister(server, toolConfig, get_control.NewTool(client))
	toolRegister(server, toolConfig, list_controls.NewTool(client))
	toolRegister(server, toolConfig, get_control_execution_report.NewTool(client))

	// Catalog browsing + lookup (one-hour in-process cache shared between
	// list_* and find_* — see clients/catalog_cache.go).
	toolRegister(server, toolConfig, list_relation_types.NewTool(client))
	toolRegister(server, toolConfig, list_attribute_types.NewTool(client))
	toolRegister(server, toolConfig, list_statuses.NewTool(client))
	toolRegister(server, toolConfig, find_asset_type.NewTool(client))
	toolRegister(server, toolConfig, find_relation_type.NewTool(client))
	toolRegister(server, toolConfig, find_attribute_type.NewTool(client))
	toolRegister(server, toolConfig, find_status.NewTool(client))
}

func toolRegister[In, Out any](server *chip.Server, toolConfig *chip.ServerToolConfig, tool *chip.Tool[In, Out]) {
	if toolConfig.IsToolEnabled(tool.Name) {
		chip.RegisterTool(server, tool)
	}
}
