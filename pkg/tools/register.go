package tools

import (
	"fmt"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/skills"
	"github.com/collibra/chip/pkg/tools/add_data_classification_match"
	"github.com/collibra/chip/pkg/tools/catalog_etl_add_schedule"
	"github.com/collibra/chip/pkg/tools/catalog_etl_cancel_job"
	"github.com/collibra/chip/pkg/tools/catalog_etl_delete_generic_config"
	"github.com/collibra/chip/pkg/tools/catalog_etl_delete_schedule"
	"github.com/collibra/chip/pkg/tools/catalog_etl_get_all_schedules"
	"github.com/collibra/chip/pkg/tools/catalog_etl_get_config"
	"github.com/collibra/chip/pkg/tools/catalog_etl_get_schedule"
	"github.com/collibra/chip/pkg/tools/catalog_etl_get_schema"
	"github.com/collibra/chip/pkg/tools/catalog_etl_save_generic_config"
	"github.com/collibra/chip/pkg/tools/catalog_etl_start_job"
	"github.com/collibra/chip/pkg/tools/catalog_etl_update_schedule"
	"github.com/collibra/chip/pkg/tools/configure_database_schemas"
	"github.com/collibra/chip/pkg/tools/create_asset"
	"github.com/collibra/chip/pkg/tools/create_community"
	"github.com/collibra/chip/pkg/tools/create_domain"
	"github.com/collibra/chip/pkg/tools/discover_business_glossary"
	"github.com/collibra/chip/pkg/tools/discover_data_assets"
	"github.com/collibra/chip/pkg/tools/edge_cancel_job"
	"github.com/collibra/chip/pkg/tools/edge_create_capability"
	"github.com/collibra/chip/pkg/tools/edge_create_connection"
	"github.com/collibra/chip/pkg/tools/edge_delete_capability"
	"github.com/collibra/chip/pkg/tools/edge_delete_connection"
	"github.com/collibra/chip/pkg/tools/edge_find_capabilities"
	"github.com/collibra/chip/pkg/tools/edge_find_connections"
	"github.com/collibra/chip/pkg/tools/edge_get_capability"
	"github.com/collibra/chip/pkg/tools/edge_get_connection"
	"github.com/collibra/chip/pkg/tools/edge_get_job_status"
	"github.com/collibra/chip/pkg/tools/edge_get_job_status_history"
	"github.com/collibra/chip/pkg/tools/edge_list_capabilities"
	"github.com/collibra/chip/pkg/tools/edge_list_capability_types"
	"github.com/collibra/chip/pkg/tools/edge_list_connections"
	"github.com/collibra/chip/pkg/tools/edge_list_sites"
	"github.com/collibra/chip/pkg/tools/edge_run_capability"
	"github.com/collibra/chip/pkg/tools/edit_asset"
	"github.com/collibra/chip/pkg/tools/find_domain_types"
	"github.com/collibra/chip/pkg/tools/find_users"
	"github.com/collibra/chip/pkg/tools/get_asset_details"
	"github.com/collibra/chip/pkg/tools/get_business_term_data"
	"github.com/collibra/chip/pkg/tools/get_column_semantics"
	"github.com/collibra/chip/pkg/tools/get_context_specification"
	"github.com/collibra/chip/pkg/tools/get_data_source_setup_guide"
	"github.com/collibra/chip/pkg/tools/get_debug_mcp_init_request"
	"github.com/collibra/chip/pkg/tools/get_job_status"
	"github.com/collibra/chip/pkg/tools/get_lineage_downstream"
	"github.com/collibra/chip/pkg/tools/get_lineage_entity"
	"github.com/collibra/chip/pkg/tools/get_lineage_transformation"
	"github.com/collibra/chip/pkg/tools/get_lineage_upstream"
	"github.com/collibra/chip/pkg/tools/get_measure_data"
	"github.com/collibra/chip/pkg/tools/get_registered_database"
	"github.com/collibra/chip/pkg/tools/get_table_semantics"
	"github.com/collibra/chip/pkg/tools/init_data_contract"
	"github.com/collibra/chip/pkg/tools/jobs_find"
	"github.com/collibra/chip/pkg/tools/list_asset_types"
	"github.com/collibra/chip/pkg/tools/list_context_specifications"
	"github.com/collibra/chip/pkg/tools/list_data_contracts"
	"github.com/collibra/chip/pkg/tools/list_integrations"
	"github.com/collibra/chip/pkg/tools/prepare_create_asset"
	"github.com/collibra/chip/pkg/tools/pull_data_contract_manifest"
	"github.com/collibra/chip/pkg/tools/push_data_contract_manifest"
	"github.com/collibra/chip/pkg/tools/register_database"
	"github.com/collibra/chip/pkg/tools/remove_data_classification_match"
	"github.com/collibra/chip/pkg/tools/search_asset_keyword"
	"github.com/collibra/chip/pkg/tools/search_data_classes"
	"github.com/collibra/chip/pkg/tools/search_data_classification_matches"
	"github.com/collibra/chip/pkg/tools/search_lineage_entities"
	"github.com/collibra/chip/pkg/tools/search_lineage_transformations"
	"github.com/collibra/chip/pkg/tools/start_ingestion"
	"github.com/collibra/chip/pkg/tools/start_technical_lineage"
	"github.com/collibra/chip/pkg/tools/test_connection"
	"github.com/collibra/chip/pkg/tools/upload_file"
)

// ContextSpecificationsFeature is the experimental-feature identifier used to
// gate the context specification tools.
const ContextSpecificationsFeature = "context-specifications"

// CopilotToolNames lists tool names that are routed to the copilot service.
// Used by chip-service to direct these requests to the copilot backend
// instead of the standard DGC API.
var CopilotToolNames = []string{
	"discover_data_assets",
	"discover_business_glossary",
}

func RegisterAll(server *chip.Server, client *http.Client, toolConfig *chip.ServerToolConfig) error {
	toolRegister(server, toolConfig, discover_data_assets.NewTool(client))
	toolRegister(server, toolConfig, discover_business_glossary.NewTool(client))
	toolRegister(server, toolConfig, get_asset_details.NewTool(client, toolConfig.IsExperimentalEnabled(ContextSpecificationsFeature)))
	toolRegister(server, toolConfig, search_asset_keyword.NewTool(client))
	toolRegister(server, toolConfig, search_data_classes.NewTool(client))
	toolRegister(server, toolConfig, list_asset_types.NewTool(client))
	toolRegister(server, toolConfig, add_data_classification_match.NewTool(client))
	toolRegister(server, toolConfig, search_data_classification_matches.NewTool(client))
	toolRegister(server, toolConfig, remove_data_classification_match.NewTool(client))
	toolRegister(server, toolConfig, list_data_contracts.NewTool(client))
	toolRegister(server, toolConfig, init_data_contract.NewTool(client))
	toolRegister(server, toolConfig, push_data_contract_manifest.NewTool(client))
	toolRegister(server, toolConfig, pull_data_contract_manifest.NewTool(client))
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
	toolRegister(server, toolConfig, create_asset.NewTool(client))
	toolRegister(server, toolConfig, edit_asset.NewTool(client))
	if toolConfig.IsExperimentalEnabled(ContextSpecificationsFeature) {
		toolRegister(server, toolConfig, list_context_specifications.NewTool(client))
		toolRegister(server, toolConfig, get_context_specification.NewTool(client))
	}
	toolRegister(server, toolConfig, find_users.NewTool(client))
	toolRegister(server, toolConfig, find_domain_types.NewTool(client))
	toolRegister(server, toolConfig, edge_find_connections.NewTool(client))
	toolRegister(server, toolConfig, edge_list_sites.NewTool(client))
	toolRegister(server, toolConfig, edge_list_capability_types.NewTool(client))
	toolRegister(server, toolConfig, create_community.NewTool(client))
	toolRegister(server, toolConfig, create_domain.NewTool(client))
	if toolConfig.AllowLocalFileUpload {
		toolRegister(server, toolConfig, upload_file.NewToolWithFilePath(client))
	} else {
		toolRegister(server, toolConfig, upload_file.NewTool(client))
	}
	toolRegister(server, toolConfig, get_data_source_setup_guide.NewTool(client))
	toolRegister(server, toolConfig, edge_create_connection.NewTool(client))
	toolRegister(server, toolConfig, edge_create_capability.NewTool(client))
	toolRegister(server, toolConfig, test_connection.NewTool(client))
	toolRegister(server, toolConfig, edge_get_job_status.NewTool(client))
	toolRegister(server, toolConfig, get_job_status.NewTool(client))
	toolRegister(server, toolConfig, register_database.NewTool(client))
	toolRegister(server, toolConfig, configure_database_schemas.NewTool(client))
	toolRegister(server, toolConfig, start_ingestion.NewTool(client))
	toolRegister(server, toolConfig, get_registered_database.NewTool(client))
	toolRegister(server, toolConfig, start_technical_lineage.NewTool(client))

	// Edge Management tools — read/delete/run operations with no counterpart among the
	// create/find/test/get-status lifecycle tools above (edge_create_connection and
	// edge_create_capability already upsert, so no edge update tools are needed here).
	toolRegister(server, toolConfig, edge_list_capabilities.NewTool(client))
	toolRegister(server, toolConfig, edge_find_capabilities.NewTool(client))
	toolRegister(server, toolConfig, edge_get_capability.NewTool(client))
	toolRegister(server, toolConfig, edge_delete_capability.NewTool(client))
	toolRegister(server, toolConfig, edge_run_capability.NewTool(client))
	toolRegister(server, toolConfig, edge_list_connections.NewTool(client))
	toolRegister(server, toolConfig, edge_get_connection.NewTool(client))
	toolRegister(server, toolConfig, edge_delete_connection.NewTool(client))
	toolRegister(server, toolConfig, edge_cancel_job.NewTool(client))
	toolRegister(server, toolConfig, edge_get_job_status_history.NewTool(client))
	// Catalog Generic Integration tools
	toolRegister(server, toolConfig, catalog_etl_get_config.NewTool(client))
	toolRegister(server, toolConfig, catalog_etl_save_generic_config.NewTool(client))
	toolRegister(server, toolConfig, catalog_etl_delete_generic_config.NewTool(client))
	toolRegister(server, toolConfig, catalog_etl_get_schema.NewTool(client))
	toolRegister(server, toolConfig, catalog_etl_get_schedule.NewTool(client))
	toolRegister(server, toolConfig, catalog_etl_add_schedule.NewTool(client))
	toolRegister(server, toolConfig, catalog_etl_update_schedule.NewTool(client))
	toolRegister(server, toolConfig, catalog_etl_delete_schedule.NewTool(client))
	toolRegister(server, toolConfig, catalog_etl_get_all_schedules.NewTool(client))
	toolRegister(server, toolConfig, catalog_etl_cancel_job.NewTool(client))
	toolRegister(server, toolConfig, catalog_etl_start_job.NewTool(client))
	// Jobs tools
	toolRegister(server, toolConfig, jobs_find.NewTool(client))
	// Integration lifecycle tools
	toolRegister(server, toolConfig, list_integrations.NewTool(client))

	if toolConfig.EnableDebugTools {
		toolRegister(server, toolConfig, get_debug_mcp_init_request.NewTool(client))
	}

	if skills.Enabled(toolConfig) {
		if err := skills.RegisterAll(server, toolConfig.SkillsDir); err != nil {
			return fmt.Errorf("register skills: %w", err)
		}
	}
	return nil
}

func toolRegister[In, Out any](server *chip.Server, toolConfig *chip.ServerToolConfig, tool *chip.Tool[In, Out]) {
	if toolConfig.IsToolEnabled(tool.Name) {
		chip.RegisterTool(server, tool)
	}
}
