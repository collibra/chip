package tools

import (
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/tools/add_data_classification_match"
	"github.com/collibra/chip/pkg/tools/ask_dad"
	"github.com/collibra/chip/pkg/tools/ask_glossary"
	"github.com/collibra/chip/pkg/tools/find_data_classification_matches"
	"github.com/collibra/chip/pkg/tools/get_asset_details"
	"github.com/collibra/chip/pkg/tools/get_business_term"
	"github.com/collibra/chip/pkg/tools/keyword_search"
	"github.com/collibra/chip/pkg/tools/list_asset_types"
	"github.com/collibra/chip/pkg/tools/list_data_contracts"
	"github.com/collibra/chip/pkg/tools/pull_data_contract_manifest"
	"github.com/collibra/chip/pkg/tools/push_data_contract_manifest"
	"github.com/collibra/chip/pkg/tools/remove_data_classification_match"
	"github.com/collibra/chip/pkg/tools/search_data_classes"
)

func RegisterAll(server *chip.Server, client *http.Client, toolConfig *chip.ServerToolConfig) {
	toolRegister(server, toolConfig, ask_dad.NewTool(client))
	toolRegister(server, toolConfig, ask_glossary.NewTool(client))
	toolRegister(server, toolConfig, get_asset_details.NewTool(client))
	toolRegister(server, toolConfig, keyword_search.NewTool(client))
	toolRegister(server, toolConfig, search_data_classes.NewTool(client))
	toolRegister(server, toolConfig, list_asset_types.NewTool(client))
	toolRegister(server, toolConfig, add_data_classification_match.NewTool(client))
	toolRegister(server, toolConfig, find_data_classification_matches.NewTool(client))
	toolRegister(server, toolConfig, remove_data_classification_match.NewTool(client))
	toolRegister(server, toolConfig, list_data_contracts.NewTool(client))
	toolRegister(server, toolConfig, push_data_contract_manifest.NewTool(client))
	toolRegister(server, toolConfig, pull_data_contract_manifest.NewTool(client))
	toolRegister(server, toolConfig, get_business_term.NewTool(client))
}

func toolRegister[In, Out any](server *chip.Server, toolConfig *chip.ServerToolConfig, tool *chip.Tool[In, Out]) {
	if toolConfig.IsToolEnabled(tool.Name) {
		chip.RegisterTool(server, tool)
	}
}
