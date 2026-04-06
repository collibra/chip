package tools

import (
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/tools/add_business_term"
	"github.com/collibra/chip/pkg/tools/add_data_classification_match"
	"github.com/collibra/chip/pkg/tools/create_asset"
	"github.com/collibra/chip/pkg/tools/discover_business_glossary"
	"github.com/collibra/chip/pkg/tools/discover_data_assets"
	"github.com/collibra/chip/pkg/tools/get_asset_details"
	"github.com/collibra/chip/pkg/tools/get_business_term_data"
	"github.com/collibra/chip/pkg/tools/get_column_semantics"
	"github.com/collibra/chip/pkg/tools/get_lineage_downstream"
	"github.com/collibra/chip/pkg/tools/get_lineage_entity"
	"github.com/collibra/chip/pkg/tools/get_lineage_transformation"
	"github.com/collibra/chip/pkg/tools/get_lineage_upstream"
	"github.com/collibra/chip/pkg/tools/get_measure_data"
	"github.com/collibra/chip/pkg/tools/get_table_semantics"
	"github.com/collibra/chip/pkg/tools/list_asset_types"
	"github.com/collibra/chip/pkg/tools/list_data_contracts"
	"github.com/collibra/chip/pkg/tools/prepare_add_business_term"
	"github.com/collibra/chip/pkg/tools/pull_data_contract_manifest"
	"github.com/collibra/chip/pkg/tools/push_data_contract_manifest"
	"github.com/collibra/chip/pkg/tools/remove_data_classification_match"
	"github.com/collibra/chip/pkg/tools/search_asset_keyword"
	"github.com/collibra/chip/pkg/tools/search_data_classification_matches"
	"github.com/collibra/chip/pkg/tools/search_data_classes"
	"github.com/collibra/chip/pkg/tools/search_lineage_entities"
	"github.com/collibra/chip/pkg/tools/search_lineage_transformations"
)

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
	toolRegister(server, toolConfig, add_business_term.NewTool(client))
	toolRegister(server, toolConfig, create_asset.NewTool(client))
}

func toolRegister[In, Out any](server *chip.Server, toolConfig *chip.ServerToolConfig, tool *chip.Tool[In, Out]) {
	if toolConfig.IsToolEnabled(tool.Name) {
		chip.RegisterTool(server, tool)
	}
}
