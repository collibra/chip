package chip

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"slices"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ToolHandlerFunc[In, Out any] func(ctx context.Context, input In) (Out, error)

type CallToolFunc func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error)

type ToolMiddleware interface {
	ToolHandle(ctx context.Context, toolRequest *mcp.CallToolRequest, next CallToolFunc) (*mcp.CallToolResult, error)
}

type ToolMiddlewareFunc func(ctx context.Context, toolRequest *mcp.CallToolRequest, next CallToolFunc) (*mcp.CallToolResult, error)

func (f ToolMiddlewareFunc) ToolHandle(ctx context.Context, toolRequest *mcp.CallToolRequest, next CallToolFunc) (*mcp.CallToolResult, error) {
	return f(ctx, toolRequest, next)
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

type Tool[In, Out any] struct {
	Name        string
	Description string
	Handler     ToolHandlerFunc[In, Out]
}

func RegisterTool[In, Out any](s *Server, tool *Tool[In, Out]) {
	slog.Info(fmt.Sprintf("Registering tool: %s", tool.Name))
	handler := func(ctx context.Context, toolRequest *mcp.CallToolRequest, input In) (*mcp.CallToolResult, Out, error) {
		var capturedOutput Out

		middlewareChain := func(ctx context.Context, r *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			out, err := tool.Handler(ctx, input)
			if err != nil {
				slog.ErrorContext(ctx, "error while calling tool function", "error", err)
			}
			capturedOutput = out
			return nil, err
		}

		for i := len(s.toolMiddlewares) - 1; i >= 0; i-- {
			mw := s.toolMiddlewares[i]
			next := middlewareChain
			middlewareChain = func(ctx context.Context, r *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return mw.ToolHandle(ctx, r, next)
			}
		}

		ctx = SetCallToolRequest(ctx, toolRequest)
		res, err := middlewareChain(ctx, toolRequest)

		return res, capturedOutput, err
	}

	mcp.AddTool(&s.Server, &mcp.Tool{
		Name:         tool.Name,
		Description:  tool.Description,
		InputSchema:  buildSchema[In](),
		OutputSchema: buildSchema[Out](),
	}, handler)
}

func buildSchema[Schema any]() *jsonschema.Schema {
	inputSchema, err := jsonschema.For[Schema](nil)
	if err != nil {
		log.Fatal(err)
	}
	inputSchema.AdditionalProperties = nil
	return inputSchema
}
