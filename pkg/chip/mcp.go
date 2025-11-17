package chip

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ToolConfig struct {
	CollibraUrl   string
	EnabledTools  []string
	DisabledTools []string
}

func (tc *ToolConfig) IsToolEnabled(toolName string) bool {
	if slices.Contains(tc.DisabledTools, toolName) {
		return false
	}

	if len(tc.EnabledTools) > 0 {
		return slices.Contains(tc.EnabledTools, toolName)
	}

	return true
}

func NewMcpServer() *mcp.Server {
	return mcp.NewServer(&mcp.Implementation{
		Name:    "Collibra MCP server",
		Title:   "Collibra Data Intelligence Platform MCP Server - Discover and interact with your governed assets. Collibra's operating model structures data governance through Communities (high-level organizational units), Domains (logical groupings of assets within communities), and Assets (core building blocks like data sets, business terms, and reports). Assets have Attributes (metadata) and Relations (connections between assets), and are organized by Asset Types (templates defining structure) and Domain Types (templates defining which asset types belong in a domain). This hierarchical model forms a knowledge graph that enables effective data governance, access control, and lineage tracking across your organization.",
		Version: Version,
	}, nil)
}

func RegisterMcpTool[In, Out any](server *mcp.Server, tool *CollibraTool[In, Out], client *http.Client, toolConfig *ToolConfig) {
	slog.Info(fmt.Sprintf("Registering tool: %s", tool.Tool.Name))
	mcp.AddTool(server, tool.Tool, mcpToolFunction(tool.ToolHandler, client, toolConfig))
}

type CollibraToolHandler[In, Out any] func(ctx context.Context, client *http.Client, input In) (Out, error)

type CollibraTool[In, Out any] struct {
	Tool        *mcp.Tool
	ToolHandler CollibraToolHandler[In, Out]
}

func GetHeaderValue(mcpRequest *mcp.CallToolRequest, header string) string {
	extra := mcpRequest.GetExtra()
	if extra == nil {
		return ""
	}

	headers := extra.Header
	if headers == nil {
		return ""
	}

	if values, exists := headers[header]; exists && len(values) > 0 {
		return values[0]
	}
	return ""
}

func GetSessionId(mcpRequest *mcp.CallToolRequest) string {
	sessionId := mcpRequest.GetSession().ID()
	if sessionId == "" {
		sessionId = uuid.New().String()
	}
	return sessionId
}

type contextKey string

const (
	CallToolRequestKey contextKey = "mcpRequest"
	ToolConfigKey      contextKey = "toolConfig"
)

func GetCallToolRequest(ctx context.Context) (*mcp.CallToolRequest, error) {
	mcpRequest, ok := ctx.Value(CallToolRequestKey).(*mcp.CallToolRequest)
	if !ok || mcpRequest == nil {
		return nil, errors.New("mcpRequest not found in ctx")
	}
	return mcpRequest, nil
}

func GetToolConfig(ctx context.Context) (*ToolConfig, error) {
	config, ok := ctx.Value(ToolConfigKey).(*ToolConfig)
	if !ok || config == nil {
		return nil, errors.New("toolConfig not found in ctx")
	}
	return config, nil
}

func GetCollibraUrl(ctx context.Context) (string, error) {
	toolRequest, err := GetCallToolRequest(ctx)
	if err != nil {
		return "", err
	}
	if toolRequest.GetExtra() != nil {
		return strings.TrimSuffix(toolRequest.Extra.Header.Get("collibraUrl"), "/"), nil
	}
	config, err := GetToolConfig(ctx)
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(config.CollibraUrl, "/"), nil
}

func CopyHeader(mcpRequest *mcp.CallToolRequest, httpRequest *http.Request, header string) {
	extra := mcpRequest.GetExtra()
	if extra == nil {
		// When running in stdio mode, extra is nil.
		return
	}

	headers := extra.Header
	if headers == nil {
		return
	}

	if values, exists := headers[header]; exists {
		for _, value := range values {
			httpRequest.Header.Add(header, value)
		}
	}
}

func mcpToolFunction[In, Out any](handler CollibraToolHandler[In, Out], client *http.Client, toolConfig *ToolConfig) func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, Out, error) {
	return func(ctx context.Context, mcpRequest *mcp.CallToolRequest, input In) (*mcp.CallToolResult, Out, error) {
		ctx = context.WithValue(ctx, CallToolRequestKey, mcpRequest)
		ctx = context.WithValue(ctx, ToolConfigKey, toolConfig)
		output, err := handler(ctx, client, input)
		return nil, output, err
	}
}
