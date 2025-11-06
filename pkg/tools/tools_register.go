package tools

import (
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func RegisterAll(server *mcp.Server, client *http.Client, toolConfig *chip.ToolConfig) {
	chip.RegisterMcpTool(server, NewAskDadTool(), client, toolConfig)
	chip.RegisterMcpTool(server, NewAskGlossaryTool(), client, toolConfig)
	chip.RegisterMcpTool(server, NewAssetDetailsTool(), client, toolConfig)
	chip.RegisterMcpTool(server, NewKeywordSearchTool(), client, toolConfig)
	chip.RegisterMcpTool(server, NewFindDataClassesTool(), client, toolConfig)
	chip.RegisterMcpTool(server, NewListAssetTypesTool(), client, toolConfig)
	chip.RegisterMcpTool(server, NewListDataContractsTool(), client, toolConfig)
	chip.RegisterMcpTool(server, NewPullDataContractManifestTool(), client, toolConfig)
	chip.RegisterMcpTool(server, NewAddClassificationMatchTool(), client, toolConfig)
	chip.RegisterMcpTool(server, NewFindClassificationMatchesTool(), client, toolConfig)
	chip.RegisterMcpTool(server, NewRemoveClassificationMatchTool(), client, toolConfig)
	chip.RegisterMcpTool(server, NewPushDataContractManifestTool(), client, toolConfig)
}
