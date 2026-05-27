package get_debug_mcp_init_request_test

import (
	"context"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/tools/get_debug_mcp_init_request"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestHandler_ReturnsParamsFromContext(t *testing.T) {
	tool := get_debug_mcp_init_request.NewTool(nil)
	params := &mcp.InitializeParams{
		ProtocolVersion: "2024-11-05",
		ClientInfo:      &mcp.Implementation{Name: "test-client", Version: "v1.2.3"},
		Capabilities:    &mcp.ClientCapabilities{},
	}
	ctx := chip.SetInitParams(context.Background(), params)

	out, err := tool.Handler(ctx, get_debug_mcp_init_request.Input{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if out.ProtocolVersion != "2024-11-05" {
		t.Fatalf("expected ProtocolVersion=2024-11-05, got %q", out.ProtocolVersion)
	}
	if out.ClientInfo == nil {
		t.Fatal("expected ClientInfo on output")
	}
	if out.ClientInfo.Name != "test-client" {
		t.Fatalf("expected ClientInfo.Name=test-client, got %q", out.ClientInfo.Name)
	}
	if out.ClientInfo.Version != "v1.2.3" {
		t.Fatalf("expected ClientInfo.Version=v1.2.3, got %q", out.ClientInfo.Version)
	}
	if out.Capabilities == nil {
		t.Fatal("expected Capabilities on output")
	}
}

func TestHandler_ErrorsWhenContextMissingParams(t *testing.T) {
	tool := get_debug_mcp_init_request.NewTool(nil)

	_, err := tool.Handler(context.Background(), get_debug_mcp_init_request.Input{})
	if err == nil {
		t.Fatal("expected error when context has no init params")
	}
	if !strings.Contains(err.Error(), "no InitializeParams") {
		t.Fatalf("expected error mentioning missing InitializeParams, got %q", err.Error())
	}
}
