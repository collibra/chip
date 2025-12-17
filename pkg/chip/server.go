package chip

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"slices"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ToolMiddlewareHandler func(ctx context.Context, input any) (any, error)

type ToolMiddleware interface {
	Handle(next ToolMiddlewareHandler) ToolMiddlewareHandler
}

type ToolMiddlewareFunc func(next ToolMiddlewareHandler) ToolMiddlewareHandler

func (f ToolMiddlewareFunc) Handle(next ToolMiddlewareHandler) ToolMiddlewareHandler {
	return f(next)
}

type Server struct {
	toolMiddlewares []ToolMiddleware
	mcp.Server
}

func NewServer(opts ...ServerOption) *Server {
	s := &Server{
		toolMiddlewares: []ToolMiddleware{},
		Server: *mcp.NewServer(&mcp.Implementation{
			Name:    "Collibra MCP server",
			Title:   "Collibra Data Intelligence Platform MCP Server",
			Version: Version,
		}, nil),
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

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

type ServerOption func(*Server)

func WithToolMiddleware(middleware ToolMiddleware) ServerOption {
	return func(s *Server) {
		s.toolMiddlewares = append(s.toolMiddlewares, middleware)
	}
}

type ToolHandler[In, Out any] func(ctx context.Context, client *http.Client, input In) (Out, error)

type Tool[In, Out any] struct {
	Tool        *mcp.Tool
	ToolHandler ToolHandler[In, Out]
}

func RegisterTool[In, Out any](server *Server, tool *Tool[In, Out], client *http.Client, toolConfig *ToolConfig) {
	slog.Info(fmt.Sprintf("Registering tool: %s", tool.Tool.Name))
	mcp.AddTool(
		&server.Server,
		tool.Tool,
		toolHandlerFor(server, tool.ToolHandler, client, toolConfig),
	)
}

func toolHandlerFor[In, Out any](server *Server, logic ToolHandler[In, Out], client *http.Client, toolConfig *ToolConfig) mcp.ToolHandlerFor[In, Out] {
	var root ToolMiddlewareHandler = func(ctx context.Context, input any) (any, error) {
		typedInput, ok := input.(In)
		if !ok {
			var zero In
			return nil, fmt.Errorf("middleware chain input mismatch: expected %T, got %T", zero, input)
		}
		out, err := logic(ctx, client, typedInput)
		if err != nil {
			slog.ErrorContext(ctx, "error while calling tool logic", "error", err)
		}
		return out, err
	}

	chain := root
	for i := len(server.toolMiddlewares) - 1; i >= 0; i-- {
		chain = server.toolMiddlewares[i].Handle(chain)
	}

	return func(ctx context.Context, toolRequest *mcp.CallToolRequest, input In) (*mcp.CallToolResult, Out, error) {
		ctx = SetCallToolRequest(ctx, toolRequest)
		ctx = SetToolConfig(ctx, toolConfig)

		output, err := chain(ctx, input)
		if err != nil {
			var zero Out
			return nil, zero, err
		}

		typedOutput, ok := output.(Out)
		if !ok {
			var zero Out
			return nil, zero, fmt.Errorf("middleware chain output mismatch: expected %T, got %T", zero, output)
		}

		return nil, typedOutput, nil
	}
}
