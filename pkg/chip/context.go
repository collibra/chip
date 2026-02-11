package chip

import (
	"context"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type contextKey int

const (
	callToolRequestKey contextKey = iota
	collibraHostKey
	toolPermissionsKey
)

func SetCallToolRequest(ctx context.Context, toolRequest *mcp.CallToolRequest) context.Context {
	return context.WithValue(ctx, callToolRequestKey, toolRequest)
}

func GetCallToolRequest(ctx context.Context) (*mcp.CallToolRequest, bool) {
	toolRequest, ok := ctx.Value(callToolRequestKey).(*mcp.CallToolRequest)
	return toolRequest, ok
}

func SetCollibraHost(ctx context.Context, collibraHost string) context.Context {
	return context.WithValue(ctx, collibraHostKey, collibraHost)
}

func GetCollibraHost(ctx context.Context) (string, bool) {
	collibraHost, ok := ctx.Value(collibraHostKey).(string)
	return collibraHost, ok
}

func SetToolPermissions(ctx context.Context, permissions []string) context.Context {
	return context.WithValue(ctx, toolPermissionsKey, permissions)
}

func GetToolPermissions(ctx context.Context) ([]string, bool) {
	permissions, ok := ctx.Value(toolPermissionsKey).([]string)
	return permissions, ok
}

func GetSessionId(ctx context.Context) string {
	toolRequest, ok := GetCallToolRequest(ctx)
	if ok {
		return toolRequest.GetSession().ID()
	}
	return uuid.New().String()
}
