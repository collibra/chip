package tools

import (
	"fmt"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/skills"
	"github.com/collibra/chip/pkg/tools/add_data_classification_match"
	"github.com/collibra/chip/pkg/tools/cancel_dq_job_run"
	"github.com/collibra/chip/pkg/tools/create_assessment"
	"github.com/collibra/chip/pkg/tools/create_asset"
	"github.com/collibra/chip/pkg/tools/create_dq_job"
	"github.com/collibra/chip/pkg/tools/create_dq_rule"
	"github.com/collibra/chip/pkg/tools/delete_dq_job"
	"github.com/collibra/chip/pkg/tools/delete_dq_job_run"
	"github.com/collibra/chip/pkg/tools/deploy_dq_rule_template"
	"github.com/collibra/chip/pkg/tools/discover_business_glossary"
	"github.com/collibra/chip/pkg/tools/discover_data_assets"
	"github.com/collibra/chip/pkg/tools/edit_assessment"
	"github.com/collibra/chip/pkg/tools/edit_asset"
	"github.com/collibra/chip/pkg/tools/find_dq_rules"
	"github.com/collibra/chip/pkg/tools/generate_dq_rule_sql"
	"github.com/collibra/chip/pkg/tools/get_assessment"
	"github.com/collibra/chip/pkg/tools/get_asset_details"
	"github.com/collibra/chip/pkg/tools/get_business_term_data"
	"github.com/collibra/chip/pkg/tools/get_column_semantics"
	"github.com/collibra/chip/pkg/tools/get_context_specification"
	"github.com/collibra/chip/pkg/tools/get_debug_mcp_init_request"
	"github.com/collibra/chip/pkg/tools/get_dq_rule"
	"github.com/collibra/chip/pkg/tools/get_dq_rule_results"
	"github.com/collibra/chip/pkg/tools/get_dq_rule_template"
	"github.com/collibra/chip/pkg/tools/get_lineage_downstream"
	"github.com/collibra/chip/pkg/tools/get_lineage_entity"
	"github.com/collibra/chip/pkg/tools/get_lineage_transformation"
	"github.com/collibra/chip/pkg/tools/get_lineage_upstream"
	"github.com/collibra/chip/pkg/tools/get_measure_data"
	"github.com/collibra/chip/pkg/tools/get_table_semantics"
	"github.com/collibra/chip/pkg/tools/init_data_contract"
	"github.com/collibra/chip/pkg/tools/list_asset_types"
	"github.com/collibra/chip/pkg/tools/list_context_specifications"
	"github.com/collibra/chip/pkg/tools/list_data_contracts"
	"github.com/collibra/chip/pkg/tools/list_dq_rule_templates"
	"github.com/collibra/chip/pkg/tools/prepare_create_asset"
	"github.com/collibra/chip/pkg/tools/pull_data_contract_manifest"
	"github.com/collibra/chip/pkg/tools/push_data_contract_manifest"
	"github.com/collibra/chip/pkg/tools/remove_data_classification_match"
	"github.com/collibra/chip/pkg/tools/search_asset_keyword"
	"github.com/collibra/chip/pkg/tools/search_catalog_columns"
	"github.com/collibra/chip/pkg/tools/search_data_classes"
	"github.com/collibra/chip/pkg/tools/search_data_classification_matches"
	"github.com/collibra/chip/pkg/tools/search_lineage_entities"
	"github.com/collibra/chip/pkg/tools/search_lineage_transformations"
	"github.com/collibra/chip/pkg/tools/start_collibra_workflow"
	"github.com/collibra/chip/pkg/tools/update_dq_job"
	"github.com/collibra/chip/pkg/tools/validate_dq_rule"
)

// ContextSpecificationsFeature is the experimental-feature identifier used to
// gate the context specification tools.
const ContextSpecificationsFeature = "context-specifications"

// DataQualityFeatureName gates the data-quality rule tools (create/validate/read rules,
// rule templates, Text2SQL and catalog column search) behind --experimental. Some WRITE to
// Collibra (create rules, deploy templates), so they stay opt-in until they graduate. Off by
// default. Shared with the data-quality job-creation and job run tools.
const DataQualityFeatureName = "data-quality"

// WorkflowsFeatureName gates start_collibra_workflow behind --experimental. It WRITES to Collibra
// (starts a workflow instance), so it stays opt-in until it graduates. Off by default.
const WorkflowsFeatureName = "workflows"

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
	toolRegister(server, toolConfig, get_assessment.NewTool(client))
	toolRegister(server, toolConfig, create_assessment.NewTool(client))
	toolRegister(server, toolConfig, edit_assessment.NewTool(client))
	if toolConfig.IsExperimentalEnabled(DataQualityFeatureName) {
		toolRegister(server, toolConfig, create_dq_job.NewTool(client))
		toolRegister(server, toolConfig, create_dq_rule.NewTool(client))
		toolRegister(server, toolConfig, get_dq_rule.NewTool(client))
		toolRegister(server, toolConfig, get_dq_rule_results.NewTool(client))
		toolRegister(server, toolConfig, validate_dq_rule.NewTool(client))
		toolRegister(server, toolConfig, list_dq_rule_templates.NewTool(client))
		toolRegister(server, toolConfig, get_dq_rule_template.NewTool(client))
		toolRegister(server, toolConfig, deploy_dq_rule_template.NewTool(client))
		toolRegister(server, toolConfig, generate_dq_rule_sql.NewTool(client))
		toolRegister(server, toolConfig, find_dq_rules.NewTool(client))
		toolRegister(server, toolConfig, search_catalog_columns.NewTool(client))
		toolRegister(server, toolConfig, cancel_dq_job_run.NewTool(client))
		toolRegister(server, toolConfig, delete_dq_job_run.NewTool(client))
		toolRegister(server, toolConfig, delete_dq_job.NewTool(client))
		toolRegister(server, toolConfig, update_dq_job.NewTool(client))
	}
	if toolConfig.IsExperimentalEnabled(ContextSpecificationsFeature) {
		toolRegister(server, toolConfig, list_context_specifications.NewTool(client))
		toolRegister(server, toolConfig, get_context_specification.NewTool(client))
	}
	if toolConfig.IsExperimentalEnabled(WorkflowsFeatureName) {
		toolRegister(server, toolConfig, start_collibra_workflow.NewTool(client))
	}

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
