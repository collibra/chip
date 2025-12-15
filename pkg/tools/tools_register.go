package tools

import (
	"net/http"

	"github.com/collibra/chip/pkg/chip"
)

func RegisterAll(server *chip.Server, client *http.Client, toolConfig *chip.ToolConfig) {
	for _, register := range toolRegistry {
		register(server, client, toolConfig)
	}
}

var toolRegistry = []toolRegistrar{
	toolRegister(NewAskDadTool),
	toolRegister(NewAskGlossaryTool),
	toolRegister(NewAssetDetailsTool),
	toolRegister(NewSearchKeywordTool),
	toolRegister(NewSearchDataClassesTool),
	toolRegister(NewListAssetTypesTool),
	toolRegister(NewAddDataClassificationMatchTool),
	toolRegister(NewSearchClassificationMatchesTool),
	toolRegister(NewRemoveDataClassificationMatchTool),
	toolRegister(NewListDataContractsTool),
	toolRegister(NewPushDataContractManifestTool),
	toolRegister(NewPullDataContractManifestTool),
}

type toolRegistrar func(*chip.Server, *http.Client, *chip.ToolConfig)

func toolRegister[In, Out any](toolFunc func() *chip.CollibraTool[In, Out]) toolRegistrar {
	return func(server *chip.Server, client *http.Client, toolConfig *chip.ToolConfig) {
		toolInstance := toolFunc()
		if toolConfig.IsToolEnabled(toolInstance.Tool.Name) {
			chip.RegisterMcpTool(server, toolInstance, client, toolConfig)
		}
	}
}
