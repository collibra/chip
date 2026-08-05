package tools_test

import (
	"context"
	"log"
	"net/http"
	"slices"
	"testing"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/tools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const debugToolName = "get_debug_mcp_init_request"

func TestRegisterAll_DebugToolHiddenByDefault(t *testing.T) {
	names := listToolNames(t, &chip.ServerToolConfig{})
	if slices.Contains(names, debugToolName) {
		t.Fatalf("expected %q to be absent when EnableDebugTools=false; got tools=%v", debugToolName, names)
	}
}

func TestRegisterAll_DebugToolVisibleWhenEnabled(t *testing.T) {
	names := listToolNames(t, &chip.ServerToolConfig{EnableDebugTools: true})
	if !slices.Contains(names, debugToolName) {
		t.Fatalf("expected %q to be present when EnableDebugTools=true; got tools=%v", debugToolName, names)
	}
}

var dataQualityToolNames = []string{
	"create_data_quality_job",
	"create_data_quality_rule",
	"get_data_quality_rule",
	"get_data_quality_rule_results",
	"validate_data_quality_rule",
	"list_data_quality_rule_templates",
	"get_data_quality_rule_template",
	"deploy_data_quality_rule_template",
	"generate_data_quality_rule_sql",
	"find_data_quality_rules",
	"search_catalog_columns",
}

func TestRegisterAll_DataQualityToolsHiddenByDefault(t *testing.T) {
	names := listToolNames(t, &chip.ServerToolConfig{})
	for _, name := range dataQualityToolNames {
		if slices.Contains(names, name) {
			t.Fatalf("expected %q to be absent without the %q experimental feature; got tools=%v", name, tools.DataQualityFeatureName, names)
		}
	}
}

func TestRegisterAll_DataQualityToolsVisibleWhenEnabled(t *testing.T) {
	names := listToolNames(t, &chip.ServerToolConfig{Experimental: []string{tools.DataQualityFeatureName}})
	for _, name := range dataQualityToolNames {
		if !slices.Contains(names, name) {
			t.Fatalf("expected %q to be present with the %q experimental feature; got tools=%v", name, tools.DataQualityFeatureName, names)
		}
	}
}

func TestRegisterAll_AllToolsHaveProperAnnotations(t *testing.T) {
	cfg := &chip.ServerToolConfig{
		EnableDebugTools: true,
		Experimental:     []string{tools.ContextSpecificationsFeature, "skills"},
	}
	for _, tool := range listTools(t, cfg) {
		if tool.Title == "" {
			t.Errorf("tool %q has no title", tool.Name)
		}
		if tool.Annotations == nil {
			t.Errorf("tool %q has no annotations", tool.Name)
			continue
		}
		if tool.Annotations.DestructiveHint == nil {
			t.Errorf("tool %q does not set DestructiveHint explicitly", tool.Name)
		}
		if tool.Annotations.OpenWorldHint == nil {
			t.Errorf("tool %q does not set OpenWorldHint explicitly", tool.Name)
		}
		if tool.Annotations.ReadOnlyHint && tool.Annotations.DestructiveHint != nil && *tool.Annotations.DestructiveHint {
			t.Errorf("tool %q is read-only but marked destructive", tool.Name)
		}
		if tool.Annotations.ReadOnlyHint && !tool.Annotations.IdempotentHint {
			t.Errorf("tool %q is read-only but not marked idempotent", tool.Name)
		}
	}
}

func listToolNames(t *testing.T, cfg *chip.ServerToolConfig) []string {
	t.Helper()
	var names []string
	for _, tool := range listTools(t, cfg) {
		names = append(names, tool.Name)
	}
	return names
}

func listTools(t *testing.T, cfg *chip.ServerToolConfig) []*mcp.Tool {
	t.Helper()
	server := chip.NewServer()
	if err := tools.RegisterAll(server, &http.Client{}, cfg); err != nil {
		t.Fatalf("RegisterAll failed: %v", err)
	}

	t1, t2 := mcp.NewInMemoryTransports()
	if _, err := server.Connect(t.Context(), t1, nil); err != nil {
		log.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "client", Version: "v0.0.1"}, nil)
	session, err := client.Connect(t.Context(), t2, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = session.Close() }()

	var result []*mcp.Tool
	for tool, err := range session.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatalf("listing tools: %v", err)
		}
		result = append(result, tool)
	}
	return result
}
