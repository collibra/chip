package tools

import (
	"net/http"

	"github.com/collibra/chip/pkg/chip"
)

func RegisterAll(server *chip.Server, client *http.Client, toolConfig *chip.ToolConfig) {
	toolRegister(server, toolConfig, NewAskDadTool(client))
	toolRegister(server, toolConfig, NewAskGlossaryTool(client))
	toolRegister(server, toolConfig, NewAssetDetailsTool(client))
	toolRegister(server, toolConfig, NewSearchKeywordTool(client))
	toolRegister(server, toolConfig, NewSearchDataClassesTool(client))
	toolRegister(server, toolConfig, NewListAssetTypesTool(client))
	toolRegister(server, toolConfig, NewAddDataClassificationMatchTool(client))
	toolRegister(server, toolConfig, NewSearchClassificationMatchesTool(client))
	toolRegister(server, toolConfig, NewRemoveDataClassificationMatchTool(client))
	toolRegister(server, toolConfig, NewListDataContractsTool(client))
	toolRegister(server, toolConfig, NewPushDataContractManifestTool(client))
	toolRegister(server, toolConfig, NewPullDataContractManifestTool(client))
}

func toolRegister[In, Out any](server *chip.Server, toolConfig *chip.ToolConfig, tool *chip.Tool[In, Out]) {
	if toolConfig.IsToolEnabled(tool.Name) {
		chip.RegisterTool(server, tool)
	}
}
