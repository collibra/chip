package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"

	chip "github.com/collibra/chip/app"
	"github.com/collibra/chip/tools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	config := chip.Init()

	log.Printf("Starting Collibra MCP server (version: %s)...\n", chip.Version)

	if config.Api.Url == "" {
		log.Fatal("Missing Api url")
	}

	if config.Api.Username != "" && config.Api.Password != "" {
		log.Println("Warning: using a single basic auth header for all requests is not recommended as it will result in all actions being attributed to the same account. Consider setting an appropriate basic auth header for each request.")
	}

	client := &http.Client{
		Transport: &collibraClient{
			config: config,
			next:   chip.NewCollibraClient(config),
		},
	}
	server := chip.NewMcpServer()
	chip.RegisterMcpTool(server, tools.NewAskDadTool(), client)
	chip.RegisterMcpTool(server, tools.NewAskGlossaryTool(), client)
	chip.RegisterMcpTool(server, tools.NewAssetDetailsTool(), client)
	chip.RegisterMcpTool(server, tools.NewKeywordSearchTool(), client)
	chip.RegisterMcpTool(server, tools.NewFindDataClassesTool(), client)
	chip.RegisterMcpTool(server, tools.NewListAssetTypesTool(), client)
	chip.RegisterMcpTool(server, tools.NewListDataContractsTool(), client)
	chip.RegisterMcpTool(server, tools.NewPullDataContractManifestTool(), client)
	chip.RegisterMcpTool(server, tools.NewAddClassificationMatchTool(), client)
	chip.RegisterMcpTool(server, tools.NewFindClassificationMatchesTool(), client)
	chip.RegisterMcpTool(server, tools.NewRemoveClassificationMatchTool(), client)

	if config.Mcp.Mode == "stdio" {
		runStdioServer(server)
	} else if strings.HasPrefix(config.Mcp.Mode, "http") {
		runHttpServer(config.Mcp.Mode, server, config.Mcp.Http.Port)
	} else {
		log.Fatalf("Invalid server mode: '%s'", config.Mcp.Mode)
	}
}

func runStdioServer(server *mcp.Server) {
	log.Println("Listening on stdio")
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}

func runHttpServer(mode string, server *mcp.Server, port int) {
	var handler http.Handler

	switch mode {
	case "http", "http-streamable":
		log.Println("Using streamable http handler")
		handler = mcp.NewStreamableHTTPHandler(func(req *http.Request) *mcp.Server {
			return server
		}, &mcp.StreamableHTTPOptions{})
	case "http-sse":
		log.Println("Using SSE http handler")
		handler = mcp.NewSSEHandler(func(req *http.Request) *mcp.Server {
			return server
		})
	default:
		log.Fatalf("Invalid HTTP mode: %s (must be 'http', 'http-sse' or 'http-streamable')", mode)
	}

	httpServer := &http.Server{
		Addr:    fmt.Sprintf("localhost:%d", port),
		Handler: handler,
	}

	log.Println("Warning: HTTP server is only listening on localhost for security reasons.")
	log.Printf("Listening on localhost:%d", port)
	log.Fatal(httpServer.ListenAndServe())
}
